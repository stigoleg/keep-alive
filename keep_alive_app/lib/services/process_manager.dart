import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';

import '../core/constants.dart';
import '../core/exceptions.dart';
import '../core/logger.dart';
import '../models/cli_flags.dart';
import 'cli_download_service.dart';
import 'github_api_service.dart';

class ProcessManager {
  final CliDownloadService _downloadService;
  Process? _currentProcess;
  bool _hasProcess = false;
  bool _stopRequested = false;

  final StreamController<String> _stdoutController =
      StreamController<String>.broadcast();
  final StreamController<String> _stderrController =
      StreamController<String>.broadcast();
  final StreamController<CliProcessException> _crashController =
      StreamController<CliProcessException>.broadcast();

  final List<String> _stdoutBuffer = [];
  final List<String> _stderrBuffer = [];

  ProcessManager({CliDownloadService? downloadService})
      : _downloadService = downloadService ??
            CliDownloadService(
              apiService: GitHubApiService(dio: Dio()),
              dio: Dio(),
            );

  bool get isRunning => _hasProcess;

  int? get pid => _currentProcess?.pid;

  Stream<String> get stdoutStream => _stdoutController.stream;

  Stream<String> get stderrStream => _stderrController.stream;

  Stream<CliProcessException> get unexpectedExitStream =>
      _crashController.stream;

  List<String> get stdoutLines => List.unmodifiable(_stdoutBuffer);

  List<String> get stderrLines => List.unmodifiable(_stderrBuffer);

  Future<void> start(CliFlags flags) async {
    if (_hasProcess) {
      AppLogger.warning('CLI process already running (pid: $pid), skipping start');
      return;
    }

    final binaryPath = await _downloadService.binaryPath;
    final binaryFile = File(binaryPath);

    if (!binaryFile.existsSync()) {
      throw CliProcessException('CLI binary not found at: $binaryPath');
    }

    final args = flags.toArgs();
    AppLogger.info('Starting CLI: $binaryPath ${args.join(' ')}');

    try {
      _stopRequested = false;
      _currentProcess = await Process.start(
        binaryPath,
        args,
        mode: ProcessStartMode.normal,
      );

      _hasProcess = true;
      AppLogger.info('CLI started with PID ${_currentProcess!.pid}');

      _currentProcess!.stdout
          .transform(systemEncoding.decoder)
          .transform(const LineSplitter())
          .listen(
            _onStdoutLine,
            onError: (Object e) => _onStderrError(e),
            onDone: _onStdoutDone,
          );

      _currentProcess!.stderr
          .transform(systemEncoding.decoder)
          .transform(const LineSplitter())
          .listen(
            _onStderrLine,
            onError: (Object e) => _onStderrError(e),
            onDone: _onStderrDone,
          );

      unawaited(_currentProcess!.exitCode.then(_onProcessExit));
    } catch (e) {
      _hasProcess = false;
      _currentProcess = null;
      throw CliProcessException(
        'Failed to start CLI process: $e',
        underlying: e,
      );
    }
  }

  Future<void> stop() async {
    final process = _currentProcess;
    if (process == null || !_hasProcess) {
      AppLogger.debug('No CLI process to stop');
      return;
    }

    _stopRequested = true;
    AppLogger.info('Stopping CLI process (pid: ${process.pid})');

    try {
      if (Platform.isWindows) {
        await _windowsKill(process.pid);
      } else {
        await _unixKill(process);
      }
    } catch (e) {
      AppLogger.error('Error during process stop', e);
      rethrow;
    } finally {
      _hasProcess = false;
      _currentProcess = null;
    }
  }

  Future<void> _unixKill(Process process) async {
    process.kill(ProcessSignal.sigterm);

    try {
      const timeout = Duration(
        seconds: AppConstants.processGracefulTimeoutSeconds,
      );
      await process.exitCode.timeout(timeout);
      AppLogger.info('CLI process exited gracefully');
    } on TimeoutException {
      AppLogger.warning(
        'CLI process did not exit within ${AppConstants.processGracefulTimeoutSeconds}s, force killing',
      );
      process.kill(ProcessSignal.sigkill);
      await process.exitCode;
      AppLogger.info('CLI process force killed');
    }
  }

  Future<void> _windowsKill(int pid) async {
    try {
      await Process.run('taskkill', ['/PID', pid.toString()]);
    } catch (_) {}

    const timeout = Duration(
      seconds: AppConstants.processGracefulTimeoutSeconds,
    );
    final waited = await _waitForProcessExit(timeout);
    if (!waited) {
      AppLogger.warning(
        'CLI process did not exit within ${AppConstants.processGracefulTimeoutSeconds}s, force killing',
      );
      try {
        await Process.run('taskkill', ['/F', '/PID', pid.toString()]);
      } catch (_) {}
    }
  }

  Future<bool> _waitForProcessExit(Duration timeout) async {
    try {
      await _currentProcess?.exitCode.timeout(timeout);
      return true;
    } on TimeoutException {
      return false;
    }
  }

  Future<void> restart(CliFlags flags) async {
    AppLogger.info('Restarting CLI with new flags');
    await stop();
    await Future<void>.delayed(const Duration(milliseconds: 200));
    await start(flags);
  }

  void dispose() {
    AppLogger.info('Disposing ProcessManager');
    _stopRequested = true;
    if (_currentProcess != null) {
      try {
        _currentProcess!.kill(ProcessSignal.sigkill);
      } catch (_) {}
      _currentProcess = null;
      _hasProcess = false;
    }
    _stdoutController.close();
    _stderrController.close();
    _crashController.close();
    _stdoutBuffer.clear();
    _stderrBuffer.clear();
  }

  void _onStdoutLine(String line) {
    _stdoutBuffer.add(line);
    while (_stdoutBuffer.length > AppConstants.maxLogLines) {
      _stdoutBuffer.removeAt(0);
    }
    _safeAddToController(_stdoutController, line);
    AppLogger.debug('[CLI stdout] $line');
  }

  void _onStderrLine(String line) {
    _stderrBuffer.add(line);
    while (_stderrBuffer.length > AppConstants.maxLogLines) {
      _stderrBuffer.removeAt(0);
    }
    _safeAddToController(_stderrController, line);
    AppLogger.warning('[CLI stderr] $line');
  }

  void _onStderrError(Object error) {
    AppLogger.error('CLI stderr stream error', error);
  }

  void _onStdoutDone() {
    AppLogger.debug('CLI stdout stream closed');
  }

  void _onStderrDone() {
    AppLogger.debug('CLI stderr stream closed');
  }

  void _onProcessExit(int exitCode) {
    AppLogger.info('CLI process exited with code $exitCode');
    final wasRunning = _hasProcess;
    final wasRequested = _stopRequested;
    _hasProcess = false;
    _currentProcess = null;
    _safeAddToController(
      _stdoutController,
      '[Process exited with code $exitCode]',
    );
    if (wasRunning && !wasRequested && exitCode != 0) {
      final exception = CliProcessException(
        'CLI process exited unexpectedly with code $exitCode',
      );
      AppLogger.error(exception.message);
      if (!_crashController.isClosed) {
        _crashController.add(exception);
      }
    }
  }

  void _safeAddToController(StreamController<String> controller, String line) {
    if (!controller.isClosed) {
      controller.add(line);
    }
  }
}
