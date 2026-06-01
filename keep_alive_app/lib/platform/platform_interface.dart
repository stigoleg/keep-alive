import 'dart:async';
import 'dart:io' show Platform;

import '../models/battery_info.dart';
import 'platform_linux.dart';
import 'platform_macos.dart';
import 'platform_windows.dart';

abstract class KeepAlivePlatform {
  static KeepAlivePlatform get instance => _instance;
  static final KeepAlivePlatform _instance = _createPlatformInstance();

  static KeepAlivePlatform _createPlatformInstance() {
    if (Platform.isMacOS) return KeepAlivePlatformMacOS();
    if (Platform.isWindows) return KeepAlivePlatformWindows();
    if (Platform.isLinux) return KeepAlivePlatformLinux();
    throw UnsupportedError('Unsupported platform: ${Platform.operatingSystem}');
  }

  Future<String> getPlatformName();

  Future<void> setAutoStart(bool enabled);

  Future<bool> isAutoStartEnabled();

  Future<void> setTrayIcon(String iconPath);

  Future<void> setTrayTooltip(String tooltip);

  Future<void> setStatusBarTitle(String title);

  Future<int?> showContextMenu(List<String> items);

  Future<void> showPopover();

  Future<void> hidePopover();

  Future<String> getAppSupportDir();

  /// Returns the absolute path to the keepalive CLI bundled inside the host
  /// application bundle, or null when the platform does not ship a bundled CLI
  /// or the binary is missing/non-executable.
  Future<String?> getBundledCliPath();

  Future<BatteryInfo> getBatteryInfo();

  /// Ensures the OS has granted whatever permission is required for the
  /// keep-alive activity simulator to actually move the cursor / post input
  /// events. On macOS this maps to Accessibility (TCC) and triggers the
  /// system prompt the first time it returns false. Platforms without a
  /// permission requirement should return true.
  Future<bool> ensureActivitySimulationPermission();

  Stream<String> get trayEventStream;

  Future<void> waitUntilNativeReady();

  /// Attaches the child process [pid] to a parent-bound lifetime container
  /// so the OS will terminate it if the Flutter app dies unexpectedly. On
  /// Windows this is a Job Object with KILL_ON_JOB_CLOSE; on macOS/Linux
  /// the same effect is achieved with `setpgid` (handled inline via FFI in
  /// ProcessManager), so the default implementation is a no-op.
  Future<void> assignProcessToJobObject(int pid) async {}

  /// Best-effort hook called when the single-instance guard rejects a
  /// duplicate launch. The duplicate process is about to exit; we cannot
  /// reach inside the original process from here, so a default no-op is
  /// correct. Platforms with a working dock-bounce / IPC mechanism (macOS
  /// Distributed Notifications, Win32 SendMessage) may override.
  Future<void> activateExistingInstance() async {}
}
