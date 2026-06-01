import 'dart:async';
import 'dart:io';

import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

import '../core/constants.dart';
import '../core/logger.dart';

/// Best-effort single-instance guard for the Flutter app.
///
/// Writes our PID to `<appSupport>/keepalive_app.pid` and holds an open file
/// handle for the lifetime of the process. On startup, if the file already
/// exists, verifies the PID is alive and looks like our own executable; if
/// so the second launch backs out (returns null from [acquire]). Otherwise
/// the lockfile is treated as stale and overwritten.
///
/// This is not bullet-proof (real cross-platform file locks need fcntl
/// + LockFileEx) but is good enough to prevent the common case of two tray
/// instances racing for the same CLI subprocess.
class InstanceLock {
  final File _file;
  final RandomAccessFile _handle;

  InstanceLock._(this._file, this._handle);

  /// Attempts to acquire the lock. Returns an [InstanceLock] on success, or
  /// null if another live instance already owns it.
  static Future<InstanceLock?> acquire() async {
    final dir = await getApplicationSupportDirectory();
    if (!dir.existsSync()) {
      await dir.create(recursive: true);
    }
    final path = p.join(dir.path, AppConstants.appInstanceLockFile);
    final file = File(path);

    if (file.existsSync()) {
      final raw = await _safeReadString(file);
      final existing = int.tryParse(raw.trim());
      if (existing != null && existing > 0 && existing != pid) {
        if (await _isAlive(existing) && await _looksLikeUs(existing)) {
          AppLogger.warning(
            'Another instance is already running (pid $existing); exiting',
          );
          return null;
        }
        AppLogger.info(
          'Found stale instance lock for pid $existing; overwriting',
        );
      }
    }

    final handle = await file.open(mode: FileMode.write);
    await handle.writeString('$pid\n');
    await handle.flush();
    AppLogger.info(
      'Instance lock acquired at ${p.basename(path)} (pid $pid)',
    );
    return InstanceLock._(file, handle);
  }

  /// Releases the lock. Safe to call multiple times.
  Future<void> release() async {
    try {
      await _handle.close();
    } catch (_) {}
    try {
      if (_file.existsSync()) {
        await _file.delete();
      }
    } catch (e) {
      AppLogger.debug('Could not delete instance lock: $e');
    }
  }

  static Future<String> _safeReadString(File file) async {
    try {
      return await file.readAsString();
    } catch (_) {
      return '';
    }
  }

  static Future<bool> _isAlive(int targetPid) async {
    try {
      if (Platform.isWindows) {
        final result = await Process.run(
          'tasklist',
          ['/FI', 'PID eq $targetPid', '/FO', 'CSV', '/NH'],
        );
        final out = (result.stdout as String).trim().toLowerCase();
        return result.exitCode == 0 &&
            out.isNotEmpty &&
            !out.contains('no tasks');
      }
      final result = await Process.run('kill', ['-0', '$targetPid']);
      return result.exitCode == 0;
    } catch (_) {
      return false;
    }
  }

  static Future<bool> _looksLikeUs(int targetPid) async {
    try {
      final ourName = p.basename(Platform.resolvedExecutable).toLowerCase();
      if (Platform.isWindows) {
        final result = await Process.run(
          'tasklist',
          ['/FI', 'PID eq $targetPid', '/FO', 'CSV', '/NH'],
        );
        return (result.stdout as String).toLowerCase().contains(ourName);
      }
      if (Platform.isLinux) {
        try {
          final link = await Link('/proc/$targetPid/exe').target();
          return p.basename(link).toLowerCase() == ourName;
        } catch (_) {
          final comm = File('/proc/$targetPid/comm');
          if (comm.existsSync()) {
            final name = (await comm.readAsString()).trim().toLowerCase();
            // Linux /proc/<pid>/comm is truncated to 15 chars.
            return name == ourName || ourName.startsWith(name);
          }
          return false;
        }
      }
      final result = await Process.run(
        'ps',
        ['-o', 'comm=', '-p', '$targetPid'],
      );
      if (result.exitCode != 0) return false;
      final name = p.basename((result.stdout as String).trim());
      return name.toLowerCase() == ourName;
    } catch (e) {
      AppLogger.debug('Could not identify pid $targetPid: $e');
      return false;
    }
  }
}
