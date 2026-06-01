import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:synchronized/synchronized.dart';

import '../core/exceptions.dart';
import '../core/logger.dart';
import '../models/cli_flags.dart';
import '../models/cli_process_state.dart';
import '../services/process_manager.dart';
import 'cli_binary_provider.dart';

final processManagerProvider = Provider<ProcessManager>((ref) {
  return ProcessManager(downloadService: ref.watch(cliDownloadServiceProvider));
});

final cliProcessProvider =
    NotifierProvider<CliProcessNotifier, CliProcessState>(
  CliProcessNotifier.new,
);

class CliProcessNotifier extends Notifier<CliProcessState> {
  late final ProcessManager _processManager;
  final Lock _lock = Lock();
  StreamSubscription<String>? _stdoutSub;
  StreamSubscription<String>? _stderrSub;
  StreamSubscription<CliProcessException>? _crashSub;
  StreamSubscription<int>? _exitSub;

  @override
  CliProcessState build() {
    _processManager = ref.watch(processManagerProvider);

    _stdoutSub = _processManager.stdoutStream.listen((line) {
      AppLogger.debug('[stdout] $line');
    });

    _stderrSub = _processManager.stderrStream.listen((line) {
      AppLogger.warning('[stderr] $line');
    });

    _crashSub = _processManager.unexpectedExitStream.listen((exception) {
      AppLogger.error('CLI process crashed: ${exception.message}');
      if (state.isRunning) {
        state = CliProcessState(
          status: CliProcessStatus.error,
          pid: _processManager.pid,
          startTime: state.startTime,
          errorMessage: exception.message,
        );
      }
    });

    _exitSub = _processManager.processExitStream.listen((exitCode) {
      if (exitCode == 0 && state.isRunning) {
        AppLogger.info('CLI process completed normally');
        state = const CliProcessState(status: CliProcessStatus.idle);
      }
    });

    ref.onDispose(() async {
      await _stdoutSub?.cancel();
      await _stderrSub?.cancel();
      await _crashSub?.cancel();
      await _exitSub?.cancel();
      await _processManager.dispose();
    });

    return const CliProcessState();
  }

  Future<void> startSession(CliFlags flags) =>
      _lock.synchronized(() => _startSessionLocked(flags));

  Future<void> _startSessionLocked(CliFlags flags) async {
    if (state.isRunning) {
      AppLogger.warning('Session already running, stopping first');
      await _stopSessionLocked();
    }

    state = state.copyWith(status: CliProcessStatus.starting);

    try {
      await _processManager.start(flags);
      state = CliProcessState(
        status: CliProcessStatus.running,
        pid: _processManager.pid,
        startTime: DateTime.now(),
      );
      AppLogger.info('Session started (pid: ${_processManager.pid})');
    } on Exception catch (e) {
      AppLogger.error('Failed to start session', e);
      state = state.copyWith(
        status: CliProcessStatus.error,
        errorMessage: e.toString(),
      );
      rethrow;
    }
  }

  Future<void> stopSession() => _lock.synchronized(_stopSessionLocked);

  Future<void> _stopSessionLocked() async {
    final currentStatus = state.status;
    if (currentStatus == CliProcessStatus.idle ||
        currentStatus == CliProcessStatus.stopping) {
      return;
    }

    state = state.copyWith(status: CliProcessStatus.stopping);

    try {
      await _processManager.stop();
      state = const CliProcessState(
        status: CliProcessStatus.idle,
        exitCode: 0,
      );
      AppLogger.info('Session stopped');
    } on Exception catch (e) {
      AppLogger.error('Error stopping session', e);
      state = state.copyWith(
        status: CliProcessStatus.error,
        errorMessage: e.toString(),
      );
      rethrow;
    }
  }

  Future<void> restartSession(CliFlags flags) =>
      _lock.synchronized(() async {
        if (state.isRunning) {
          await _stopSessionLocked();
        }
        await _startSessionLocked(flags);
      });

  void clearError() {
    if (state.status == CliProcessStatus.error) {
      state = const CliProcessState(status: CliProcessStatus.idle);
    }
  }
}
