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

  Future<BatteryInfo> getBatteryInfo();

  Stream<String> get trayEventStream;

  Future<void> waitUntilNativeReady();
}
