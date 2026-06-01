import 'dart:async';
import 'dart:io';

import '../core/constants.dart';
import '../core/logger.dart';
import 'process_manager.dart';

/// Reaps a CLI process left behind by a previous app session.
///
/// Each time the Flutter app spawns the CLI it writes the child's PID to a
/// file in app support. If the Flutter app exits cleanly the file is removed.
/// If it crashes hard (SIGKILL, force-quit, OS panic) the file is left over
/// and the child may still be running. On the next launch this sweeper reads
/// that file, verifies the process is still alive and is in fact `keepalive`
/// (not a recycled PID belonging to something else), then terminates it.
class StaleCliSweeper {
  StaleCliSweeper._();

  /// Inspects the PID file, kills any matching orphan CLI process, and
  /// removes the file. Safe to call before the CLI binary check, since it
  /// only reads pre-existing OS state.
  static Future<void> sweep() async {
    final path = await ProcessManager.resolvePidFilePath();
    final file = File(path);
    if (!file.existsSync()) return;

    int? pid;
    try {
      final raw = (await file.readAsString()).trim();
      pid = int.tryParse(raw);
    } catch (e) {
      AppLogger.warning('Could not read stale pid file: $e');
    }

    if (pid == null || pid <= 0) {
      AppLogger.info('Removing invalid stale pid file');
      await _safeDelete(file);
      return;
    }

    if (!await _isAlive(pid)) {
      AppLogger.info('Stale pid $pid is not alive, clearing pid file');
      await _safeDelete(file);
      return;
    }

    if (!await _looksLikeOurCli(pid)) {
      AppLogger.warning(
        'PID $pid is alive but does not look like keepalive; '
        'leaving it alone and clearing pid file',
      );
      await _safeDelete(file);
      return;
    }

    AppLogger.warning(
      'Found orphan CLI from previous session (pid $pid); terminating',
    );
    await _terminate(pid);
    await _safeDelete(file);
  }

  /// Cross-platform liveness probe. On POSIX uses `kill -0 <pid>` which sends
  /// no actual signal but succeeds iff the PID exists and is signalable by
  /// the current user. On Windows uses `tasklist` filtered to the PID.
  static Future<bool> _isAlive(int pid) async {
    try {
      if (Platform.isWindows) {
        final result = await Process.run(
          'tasklist',
          ['/FI', 'PID eq $pid', '/FO', 'CSV', '/NH'],
        );
        final out = (result.stdout as String).trim();
        return result.exitCode == 0 &&
            out.isNotEmpty &&
            !out.toLowerCase().contains('no tasks');
      }
      final result = await Process.run('kill', ['-0', pid.toString()]);
      return result.exitCode == 0;
    } catch (_) {
      return false;
    }
  }

  static Future<bool> _looksLikeOurCli(int pid) async {
    try {
      if (Platform.isWindows) {
        final result = await Process.run(
          'tasklist',
          ['/FI', 'PID eq $pid', '/FO', 'CSV', '/NH'],
        );
        final out = (result.stdout as String).toLowerCase();
        return out.contains(AppConstants.cliBinaryName.toLowerCase());
      }
      if (Platform.isLinux) {
        try {
          final link = await Link('/proc/$pid/exe').target();
          return link.split('/').last.toLowerCase() ==
              AppConstants.cliBinaryName.toLowerCase();
        } catch (_) {
          // /proc/<pid>/exe is often unreadable for processes owned by
          // other users; fall back to /proc/<pid>/comm.
          final comm = File('/proc/$pid/comm');
          if (comm.existsSync()) {
            final name = (await comm.readAsString()).trim().toLowerCase();
            return name == AppConstants.cliBinaryName.toLowerCase();
          }
          return false;
        }
      }
      // macOS: ps gives the process basename.
      final result = await Process.run(
        'ps',
        ['-o', 'comm=', '-p', pid.toString()],
      );
      if (result.exitCode != 0) return false;
      final name = (result.stdout as String).trim().split('/').last;
      return name.toLowerCase() == AppConstants.cliBinaryName.toLowerCase();
    } catch (e) {
      AppLogger.debug('Could not identify pid $pid: $e');
      return false;
    }
  }

  static Future<void> _terminate(int pid) async {
    if (Platform.isWindows) {
      try {
        await Process.run('taskkill', ['/PID', pid.toString()]);
      } catch (_) {}
      // Brief grace period before /F to mirror the in-session stop policy.
      await Future<void>.delayed(const Duration(seconds: 2));
      try {
        if (await _isAlive(pid)) {
          await Process.run('taskkill', ['/F', '/PID', pid.toString()]);
        }
      } catch (_) {}
      return;
    }
    try {
      Process.killPid(pid, ProcessSignal.sigterm);
    } catch (_) {}
    await Future<void>.delayed(const Duration(seconds: 2));
    if (await _isAlive(pid)) {
      try {
        Process.killPid(pid, ProcessSignal.sigkill);
      } catch (_) {}
    }
  }

  static Future<void> _safeDelete(File file) async {
    try {
      if (file.existsSync()) await file.delete();
    } catch (e) {
      AppLogger.debug('Failed to delete pid file: $e');
    }
  }
}
