import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/logger.dart';
import '../models/cli_flags.dart';
import '../models/cli_process_state.dart';
import '../services/process_manager.dart';

final processManagerProvider = Provider<ProcessManager>((ref) {
  return ProcessManager();
});

final cliProcessProvider =
    NotifierProvider<CliProcessNotifier, CliProcessState>(
  CliProcessNotifier.new,
);

class CliProcessNotifier extends Notifier<CliProcessState> {
  late final ProcessManager _processManager;
  StreamSubscription<String>? _stdoutSub;
  StreamSubscription<String>? _stderrSub;

  @override
  CliProcessState build() {
    _processManager = ref.watch(processManagerProvider);

    _stdoutSub = _processManager.stdoutStream.listen((line) {
      AppLogger.debug('[stdout] $line');
    });

    _stderrSub = _processManager.stderrStream.listen((line) {
      AppLogger.warning('[stderr] $line');
    });

    ref.onDispose(() {
      _stdoutSub?.cancel();
      _stderrSub?.cancel();
      _processManager.dispose();
    });

    return const CliProcessState();
  }

  Future<void> startSession(CliFlags flags) async {
    if (state.isRunning) {
      AppLogger.warning('Session already running, stopping first');
      await stopSession();
    }

    state = state.copyWith(
      status: CliProcessStatus.starting,
    );

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

  Future<void> stopSession() async {
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

  Future<void> restartSession(CliFlags flags) async {
    if (state.isRunning) {
      await stopSession();
    }
    await startSession(flags);
  }
}
