import 'dart:ffi';
import 'dart:io';

import '../core/logger.dart';

typedef _SetpgidNative = Int32 Function(Int32 pid, Int32 pgid);
typedef _SetpgidDart = int Function(int pid, int pgid);

/// Moves the given child PID into its own process group via libc `setpgid`.
///
/// On POSIX systems a child inherits the parent's process group by default,
/// so a SIGKILL of the Flutter app can leave the CLI orphaned with PPID 1.
/// Putting the child in its own group lets the OS scope cleanup correctly
/// and lets the stale-process sweeper on next launch reap any survivor.
///
/// Best-effort: failures are logged and ignored — the sweeper handles the
/// rare case where this call fails (sandboxes, exotic init systems).
class ProcessGroupPosix {
  ProcessGroupPosix._();

  static _SetpgidDart? _cachedSetpgid;

  static _SetpgidDart? _resolve() {
    if (_cachedSetpgid != null) return _cachedSetpgid;
    if (!Platform.isMacOS && !Platform.isLinux) return null;
    try {
      final libc = DynamicLibrary.process();
      _cachedSetpgid =
          libc.lookupFunction<_SetpgidNative, _SetpgidDart>('setpgid');
      return _cachedSetpgid;
    } catch (e) {
      AppLogger.warning('Could not resolve libc setpgid: $e');
      return null;
    }
  }

  /// Promotes [childPid] to its own process group. Returns true on success.
  static bool detach(int childPid) {
    final setpgid = _resolve();
    if (setpgid == null) return false;
    try {
      final rc = setpgid(childPid, childPid);
      if (rc != 0) {
        AppLogger.debug('setpgid($childPid, $childPid) returned $rc');
        return false;
      }
      return true;
    } catch (e) {
      AppLogger.debug('setpgid threw: $e');
      return false;
    }
  }
}
