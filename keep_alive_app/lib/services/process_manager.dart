import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:path_provider/path_provider.dart';
import 'package:synchronized/synchronized.dart';

import '../core/constants.dart';
import '../core/exceptions.dart';
import '../core/logger.dart';
import '../models/cli_flags.dart';
import '../platform/platform_interface.dart';
import 'cli_download_service.dart';
import 'github_api_service.dart';
import 'process_group_posix.dart';

class ProcessManager {
  final CliDownloadService _downloadService;
  final Lock _lock = Lock();
  Process? _currentProcess;
  bool _hasProcess = false;
  bool _starting = false;
  bool _stopRequested = false;

  final StreamController<String> _stdoutController =
      StreamController<String>.broadcast();
  final StreamController<String> _stderrController =
      StreamController<String>.broadcast();
  final StreamController<CliProcessException> _crashController =
      StreamController<CliProcessException>.broadcast();
  final StreamController<int> _exitCodeController =
      StreamController<int>.broadcast();

  final List<String> _stdoutBuffer = [];
  final List<String> _stderrBuffer = [];

  // ignore: cancel_subscriptions
  StreamSubscription<String>? _stdoutSub;
  // ignore: cancel_subscriptions
  StreamSubscription<String>? _stderrSub;

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

  Stream<int> get processExitStream => _exitCodeController.stream;

  List<String> get stdoutLines => List.unmodifiable(_stdoutBuffer);

  List<String> get stderrLines => List.unmodifiable(_stderrBuffer);

  Future<void> start(CliFlags flags) => _lock.synchronized(() => _startLocked(flags));

  Future<void> _startLocked(CliFlags flags) async {
    if (_hasProcess || _starting) {
      AppLogger.warning(
        'CLI process already running or starting (pid: $pid), skipping start',
      );
      return;
    }

    _starting = true;
    try {
      final binaryPath = await _downloadService.binaryPath;
      final binaryFile = File(binaryPath);

      if (!binaryFile.existsSync()) {
        throw CliProcessException('CLI binary not found at: $binaryPath');
      }

      final args = flags.toArgs();
      final cliVersion = await _resolveCliVersion(binaryPath);
      AppLogger.info(
        'Starting CLI: $binaryPath ${args.join(' ')} (version: ${cliVersion ?? "unknown"})',
      );

      final workingDir = await _resolveWorkingDir();
      final environment = _buildChildEnvironment();

      _stopRequested = false;
      _currentProcess = await Process.start(
        binaryPath,
        args,
        mode: ProcessStartMode.normal,
        workingDirectory: workingDir,
        environment: environment,
        includeParentEnvironment: true,
      );

      _hasProcess = true;
      final childPid = _currentProcess!.pid;
      AppLogger.info('CLI started with PID $childPid');
      await _writePidFile(childPid);
      await _detachFromParent(childPid);

      _stdoutSub = _currentProcess!.stdout
          .transform(systemEncoding.decoder)
          .transform(const LineSplitter())
          .listen(
            _onStdoutLine,
            onError: (Object e) => _onStderrError(e),
            onDone: _onStdoutDone,
          );

      _stderrSub = _currentProcess!.stderr
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
      if (e is CliProcessException) rethrow;
      throw CliProcessException(
        'Failed to start CLI process: $e',
        underlying: e,
      );
    } finally {
      _starting = false;
    }
  }

  Future<String?> _resolveCliVersion(String binaryPath) async {
    try {
      final result = await Process.run(
        binaryPath,
        const [AppConstants.cliVersionArg],
      ).timeout(const Duration(seconds: 5));
      if (result.exitCode != 0) return null;
      return (result.stdout as String).trim();
    } catch (_) {
      return null;
    }
  }

  Future<String?> _resolveWorkingDir() async {
    try {
      final dir = await getApplicationSupportDirectory();
      if (!dir.existsSync()) {
        await dir.create(recursive: true);
      }
      return dir.path;
    } catch (e) {
      AppLogger.debug('Could not resolve application support dir for cwd: $e');
      return null;
    }
  }

  /// GUI-launched apps on macOS inherit a minimal PATH without /usr/local/bin
  /// or /opt/homebrew/bin, which breaks helpers like `caffeinate` and `pmset`
  /// that the CLI shells out to. Augment PATH so the child process can find
  /// the system tools it depends on regardless of how the GUI was launched.
  Map<String, String> _buildChildEnvironment() {
    if (!Platform.isMacOS && !Platform.isLinux) {
      return const <String, String>{};
    }
    const ensure = [
      '/usr/bin',
      '/bin',
      '/usr/sbin',
      '/sbin',
      '/usr/local/bin',
      '/usr/local/sbin',
      '/opt/homebrew/bin',
      '/opt/homebrew/sbin',
    ];
    final current = Platform.environment['PATH'] ?? '';
    final segments = <String>[
      for (final s in current.split(':')) if (s.isNotEmpty) s,
    ];
    for (final dir in ensure) {
      if (!segments.contains(dir)) segments.add(dir);
    }
    return <String, String>{'PATH': segments.join(':')};
  }

  Future<void> stop() => _lock.synchronized(_stopLocked);

  Future<void> _stopLocked() async {
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
      await _cancelStreamSubs();
      _hasProcess = false;
      _currentProcess = null;
      await _deletePidFile();
    }
  }

  Future<void> _cancelStreamSubs() async {
    final stdoutSub = _stdoutSub;
    final stderrSub = _stderrSub;
    _stdoutSub = null;
    _stderrSub = null;
    try {
      await stdoutSub?.cancel();
    } catch (_) {}
    try {
      await stderrSub?.cancel();
    } catch (_) {}
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

  Future<void> restart(CliFlags flags) => _lock.synchronized(() async {
        AppLogger.info('Restarting CLI with new flags');
        await _stopLocked();
        await Future<void>.delayed(const Duration(milliseconds: 200));
        await _startLocked(flags);
      });

  /// Best-effort graceful stop with bounded wait, then hard cleanup.
  /// Async so callers can await full process teardown before exiting.
  Future<void> dispose() async {
    AppLogger.info('Disposing ProcessManager');
    _stopRequested = true;
    if (_currentProcess != null) {
      try {
        await stop().timeout(
          const Duration(seconds: AppConstants.processGracefulTimeoutSeconds),
        );
      } catch (e) {
        AppLogger.warning('Graceful stop failed during dispose: $e');
        _forceKillCurrent();
      }
    }
    await _cancelStreamSubs();
    if (!_stdoutController.isClosed) await _stdoutController.close();
    if (!_stderrController.isClosed) await _stderrController.close();
    if (!_crashController.isClosed) await _crashController.close();
    if (!_exitCodeController.isClosed) await _exitCodeController.close();
    _stdoutBuffer.clear();
    _stderrBuffer.clear();
  }

  void _forceKillCurrent() {
    final process = _currentProcess;
    if (process == null) return;
    try {
      if (Platform.isWindows) {
        Process.run('taskkill', ['/F', '/PID', process.pid.toString()]);
      } else {
        process.kill(ProcessSignal.sigkill);
      }
    } catch (e) {
      AppLogger.warning('Force-kill failed: $e');
    }
    _currentProcess = null;
    _hasProcess = false;
  }

  void _onStdoutLine(String line) {
    _appendBounded(_stdoutBuffer, line);
    _safeAddToController(_stdoutController, line);
    AppLogger.debug('[CLI stdout] $line');
  }

  void _onStderrLine(String line) {
    _appendBounded(_stderrBuffer, line);
    _safeAddToController(_stderrController, line);
    AppLogger.warning('[CLI stderr] $line');
  }

  static void _appendBounded(List<String> buffer, String line) {
    buffer.add(line);
    if (buffer.length > AppConstants.maxLogLines) {
      buffer.removeRange(0, buffer.length - AppConstants.maxLogLines);
    }
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
    unawaited(_deletePidFile());
    _safeAddToController(
      _stdoutController,
      '[Process exited with code $exitCode]',
    );
    _safeAddToExitCodeController(exitCode);
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

  void _safeAddToExitCodeController(int code) {
    if (!_exitCodeController.isClosed) {
      _exitCodeController.add(code);
    }
  }

  void _safeAddToController(StreamController<String> controller, String line) {
    if (!controller.isClosed) {
      controller.add(line);
    }
  }

  /// Resolves the canonical PID file path used to track the running CLI
  /// across process boundaries (used by the stale-process sweeper and by
  /// force-kill on quit).
  static Future<String> resolvePidFilePath() async {
    final dir = await getApplicationSupportDirectory();
    if (!dir.existsSync()) {
      await dir.create(recursive: true);
    }
    return '${dir.path}${Platform.pathSeparator}${AppConstants.cliPidFile}';
  }

  /// Hooks the freshly-spawned child PID into an OS-managed lifetime group
  /// so it dies (or is reapable) when the Flutter app does. Best-effort.
  /// - POSIX: `setpgid(pid, pid)` via dart:ffi.
  /// - Windows: `AssignProcessToJobObject` via platform channel; the job is
  ///   created with KILL_ON_JOB_CLOSE so a hard parent crash kills the CLI.
  Future<void> _detachFromParent(int childPid) async {
    if (Platform.isMacOS || Platform.isLinux) {
      final ok = ProcessGroupPosix.detach(childPid);
      AppLogger.debug('setpgid for $childPid: ${ok ? "ok" : "noop"}');
      return;
    }
    if (Platform.isWindows) {
      try {
        await KeepAlivePlatform.instance.assignProcessToJobObject(childPid);
      } catch (e) {
        AppLogger.warning('assignProcessToJobObject failed: $e');
      }
    }
  }

  Future<void> _writePidFile(int pid) async {
    try {
      final path = await resolvePidFilePath();
      // Atomic write: write to temp then rename.
      final tmp = File('$path.tmp');
      await tmp.writeAsString('$pid\n', flush: true);
      await tmp.rename(path);
    } catch (e) {
      AppLogger.warning('Failed to write CLI pid file: $e');
    }
  }

  Future<void> _deletePidFile() async {
    try {
      final path = await resolvePidFilePath();
      final file = File(path);
      if (file.existsSync()) {
        await file.delete();
      }
    } catch (e) {
      AppLogger.debug('Failed to delete CLI pid file: $e');
    }
  }

  /// Reads the persisted CLI PID (if any) and issues an unconditional force
  /// kill via SIGKILL / taskkill /F. Safe to call from the quit path even if
  /// our in-memory [_currentProcess] is stale or absent. Returns true if a
  /// kill signal was actually sent.
  static Future<bool> forceKillFromPidFile() async {
    try {
      final path = await resolvePidFilePath();
      final file = File(path);
      if (!file.existsSync()) return false;
      final raw = (await file.readAsString()).trim();
      final pid = int.tryParse(raw);
      if (pid == null || pid <= 0) {
        AppLogger.warning('Invalid pid in $path: "$raw"');
        await file.delete();
        return false;
      }
      AppLogger.info('Force-killing CLI from pid file: $pid');
      if (Platform.isWindows) {
        try {
          await Process.run('taskkill', ['/F', '/PID', pid.toString()]);
        } catch (e) {
          AppLogger.warning('taskkill /F failed: $e');
        }
      } else {
        try {
          Process.killPid(pid, ProcessSignal.sigkill);
        } catch (e) {
          AppLogger.warning('SIGKILL failed: $e');
        }
      }
      try {
        await file.delete();
      } catch (_) {}
      return true;
    } catch (e) {
      AppLogger.warning('forceKillFromPidFile failed: $e');
      return false;
    }
  }
}
