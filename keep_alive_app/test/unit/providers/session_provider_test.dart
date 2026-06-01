import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/exceptions.dart';
import 'package:keep_alive_app/models/cli_flags.dart';
import 'package:keep_alive_app/providers/process_provider.dart';
import 'package:keep_alive_app/providers/session_provider.dart';
import 'package:keep_alive_app/providers/settings_provider.dart';
import 'package:keep_alive_app/services/process_manager.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _RecordingProcessManager extends ProcessManager {
  final List<_Call> calls = [];
  String? _failOnCall;

  bool _running = false;

  void setFailure(String? callName) {
    _failOnCall = callName;
  }

  void setSuccess() {
    _failOnCall = null;
  }

  @override
  bool get isRunning => _running;

  @override
  int? get pid => _running ? 99999 : null;

  @override
  Stream<String> get stdoutStream => const Stream.empty();

  @override
  Stream<String> get stderrStream => const Stream.empty();

  @override
  Stream<CliProcessException> get unexpectedExitStream => const Stream.empty();

  @override
  Future<void> start(CliFlags flags) async {
    calls.add(_Call('start', flags));
    if (_failOnCall == 'start') {
      throw Exception('Start failed');
    }
    _running = true;
  }

  @override
  Future<void> stop() async {
    calls.add(const _Call('stop'));
    if (_failOnCall == 'stop') {
      throw Exception('Stop failed');
    }
    _running = false;
  }
}

class _Call {
  final String method;
  final CliFlags? flags;

  const _Call(this.method, [this.flags]);

  @override
  String toString() => '_Call($method, $flags)';
}

void main() {
  group('SessionOrchestrator', () {
    late _RecordingProcessManager processManager;
    late ProviderContainer container;

    setUp(() async {
      SharedPreferences.setMockInitialValues({});
      processManager = _RecordingProcessManager();

      container = ProviderContainer(
        overrides: [
          processManagerProvider.overrideWithValue(processManager),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    group('toggleKeepAwake', () {
      test('setting true starts session and updates settings', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();

        await orchestrator.toggleKeepAwake(true);

        expect(container.read(appSettingsProvider).keepAwake, isTrue);
        expect(processManager.calls.length, 1);
        expect(processManager.calls.first.method, 'start');
      });

      test('setting false stops session', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();

        await orchestrator.toggleKeepAwake(true);
        processManager.calls.clear();

        await orchestrator.toggleKeepAwake(false);

        expect(container.read(appSettingsProvider).keepAwake, isFalse);
        expect(processManager.calls.length, 1);
        expect(processManager.calls.first.method, 'stop');
      });

      test('start failure rolls back keepAwake setting', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setFailure('start');

        try {
          await orchestrator.toggleKeepAwake(true);
        } catch (_) {
          // Expected
        }

        expect(container.read(appSettingsProvider).keepAwake, isFalse);
      });

      test('stop failure does not rethrow', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();
        await orchestrator.toggleKeepAwake(true);

        processManager.calls.clear();
        processManager.setFailure('stop');

        await orchestrator.toggleKeepAwake(false);

        expect(container.read(appSettingsProvider).keepAwake, isFalse);
      });
    });

    group('updateFlags', () {
      test('does nothing when keepAwake is false', () async {
        final orchestrator = container.read(sessionProvider);

        await orchestrator.updateFlags(const CliFlags(durationMinutes: 30));

        expect(processManager.calls, isEmpty);
      });

      test('restarts CLI when running and flags change', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();

        await orchestrator.toggleKeepAwake(true);
        processManager.calls.clear();

        await orchestrator.updateFlags(const CliFlags(simulateActivity: true));

        expect(processManager.calls.length, 2);
        expect(processManager.calls[0].method, 'stop');
        expect(processManager.calls[1].method, 'start');
        expect(processManager.calls[1].flags?.simulateActivity, isTrue);
      });

      test('rolls back keepAwake on restart failure', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();

        await orchestrator.toggleKeepAwake(true);
        processManager.calls.clear();

        // restartSession calls stop then start, set start to fail
        processManager.setFailure('start');

        try {
          await orchestrator.updateFlags(const CliFlags(durationMinutes: 60));
        } catch (_) {
          // Expected
        }

        expect(container.read(appSettingsProvider).keepAwake, isFalse);
      });
    });

    group('applySettingsAndRestart', () {
      test('does nothing when keepAwake is false', () async {
        final orchestrator = container.read(sessionProvider);

        await orchestrator.applySettingsAndRestart();

        expect(processManager.calls, isEmpty);
      });

      test('restarts CLI when running', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();

        await container.read(appSettingsProvider.notifier).setKeepAwake(true);
        await orchestrator.toggleKeepAwake(true);
        processManager.calls.clear();

        await container.read(appSettingsProvider.notifier).setBatteryThreshold(30);
        await orchestrator.applySettingsAndRestart();

        expect(processManager.calls.length, 2);
        expect(processManager.calls[0].method, 'stop');
        expect(processManager.calls[1].method, 'start');
      });

      test('rolls back on start failure when not running', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setFailure('start');

        await container.read(appSettingsProvider.notifier).setKeepAwake(true);

        try {
          await orchestrator.applySettingsAndRestart();
        } catch (_) {
          // Expected
        }

        expect(container.read(appSettingsProvider).keepAwake, isFalse);
      });

      test('rapid calls coalesce into a single restart', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();
        await orchestrator.toggleKeepAwake(true);
        processManager.calls.clear();

        // Ten rapid calls in <100 ms — must produce exactly one stop+start
        // pair once the 350 ms debounce flushes.
        final futures = <Future<void>>[];
        for (var i = 0; i < 10; i++) {
          futures.add(orchestrator.applySettingsAndRestart());
        }
        await Future.wait(futures);

        final stops = processManager.calls.where((c) => c.method == 'stop').length;
        final starts = processManager.calls.where((c) => c.method == 'start').length;
        expect(stops, 1, reason: 'debounce should collapse to one stop');
        expect(starts, 1, reason: 'debounce should collapse to one start');
      });

      test('flushPendingRestart fires immediately', () async {
        final orchestrator = container.read(sessionProvider);
        processManager.setSuccess();
        await orchestrator.toggleKeepAwake(true);
        processManager.calls.clear();

        // Fire-and-forget so the debounce is pending.
        unawaited(orchestrator.applySettingsAndRestart());
        // Without flush, no call has happened yet (still inside debounce).
        expect(processManager.calls, isEmpty);

        await orchestrator.flushPendingRestart();
        expect(processManager.calls.length, 2);
        expect(processManager.calls[0].method, 'stop');
        expect(processManager.calls[1].method, 'start');
      });
    });
  });
}
