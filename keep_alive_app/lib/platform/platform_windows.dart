import 'package:flutter/services.dart';

import '../core/constants.dart';
import '../models/battery_info.dart';
import 'platform_interface.dart';

class KeepAlivePlatformWindows extends KeepAlivePlatform {
  static const _channel = MethodChannel(AppConstants.platformChannelName);

  @override
  Future<String> getPlatformName() async {
    final result = await _channel.invokeMethod<String>('getPlatformName');
    return result ?? 'Windows';
  }

  @override
  Future<void> setAutoStart(bool enabled) async {
    await _channel.invokeMethod('setAutoStart', {'enabled': enabled});
  }

  @override
  Future<bool> isAutoStartEnabled() async {
    final result = await _channel.invokeMethod<bool>('isAutoStartEnabled');
    return result ?? false;
  }

  @override
  Future<void> setTrayIcon(String iconPath) async {
    await _channel.invokeMethod('setTrayIcon', {'iconPath': iconPath});
  }

  @override
  Future<void> setTrayTooltip(String tooltip) async {
    await _channel.invokeMethod('setTrayTooltip', {'tooltip': tooltip});
  }

  @override
  Future<int?> showContextMenu(List<String> items) async {
    final result = await _channel.invokeMethod<int>('showContextMenu', {
      'items': items,
    });
    return result;
  }

  @override
  Future<void> showPopover(double x, double y) async {
    // Not applicable on Windows — tray click handled by system_tray package.
  }

  @override
  Future<void> hidePopover() async {
    // Not applicable on Windows.
  }

  @override
  Future<BatteryInfo> getBatteryInfo() async {
    final result = await _channel.invokeMapMethod<String, dynamic>('getBatteryInfo');
    if (result == null) {
      return const BatteryInfo(percentage: 100.0, isPresent: false);
    }
    return BatteryInfo.fromJson(result);
  }

  @override
  Future<String> getAppSupportDir() async {
    final result = await _channel.invokeMethod<String>('getAppSupportDir');
    if (result == null || result.isEmpty) {
      throw PlatformException(
        code: 'APP_SUPPORT_DIR_ERROR',
        message: 'Failed to get application support directory',
      );
    }
    return result;
  }
}
