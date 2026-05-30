import 'dart:async';

import 'package:flutter/services.dart';

import '../core/constants.dart';
import '../models/battery_info.dart';
import 'platform_interface.dart';

class KeepAlivePlatformMacOS extends KeepAlivePlatform {
  static const _channel = MethodChannel(AppConstants.platformChannelName);

  StreamController<String>? __trayEventController;
  StreamController<String> get _trayEventController {
    if (__trayEventController == null) {
      __trayEventController = StreamController<String>.broadcast();
      _channel.setMethodCallHandler(_handleMethodCall);
    }
    return __trayEventController!;
  }

  @override
  Stream<String> get trayEventStream => _trayEventController.stream;

  Future<dynamic> _handleMethodCall(MethodCall call) async {
    if (call.method == AppConstants.methodOnTrayEvent) {
      final event = call.arguments as String?;
      if (event != null) {
        _trayEventController.add(event);
      }
    } else if (call.method == 'systemShutdown') {
      _trayEventController.add('systemShutdown');
    }
  }

  @override
  Future<String> getPlatformName() async {
    final result =
        await _channel.invokeMethod<String>(AppConstants.methodGetPlatformName);
    return result ?? 'macOS';
  }

  @override
  Future<void> setAutoStart(bool enabled) async {
    await _channel.invokeMethod(AppConstants.methodSetAutoStart, {
      'enabled': enabled,
    });
  }

  @override
  Future<bool> isAutoStartEnabled() async {
    final result = await _channel
        .invokeMethod<bool>(AppConstants.methodIsAutoStartEnabled);
    return result ?? false;
  }

  @override
  Future<void> setTrayIcon(String iconPath) async {
    await _channel.invokeMethod(AppConstants.methodSetTrayIcon, {
      'iconPath': iconPath,
    });
  }

  @override
  Future<void> setTrayTooltip(String tooltip) async {
    await _channel.invokeMethod(AppConstants.methodSetTrayTooltip, {
      'tooltip': tooltip,
    });
  }

  @override
  Future<int?> showContextMenu(List<String> items) async {
    final result =
        await _channel.invokeMethod<int>(AppConstants.methodShowContextMenu, {
      'items': items,
    });
    return result;
  }

  @override
  Future<void> showPopover() async {
    await _channel.invokeMethod(AppConstants.methodShowPopover);
  }

  @override
  Future<void> hidePopover() async {
    await _channel.invokeMethod(AppConstants.methodHidePopover);
  }

  @override
  Future<BatteryInfo> getBatteryInfo() async {
    final result = await _channel
        .invokeMapMethod<String, dynamic>(AppConstants.methodGetBatteryInfo);
    if (result == null) {
      return const BatteryInfo(percentage: 100.0, isPresent: false);
    }
    return BatteryInfo.fromJson(result);
  }

  @override
  Future<String> getAppSupportDir() async {
    final result = await _channel
        .invokeMethod<String>(AppConstants.methodGetAppSupportDir);
    if (result == null || result.isEmpty) {
      throw PlatformException(
        code: 'APP_SUPPORT_DIR_ERROR',
        message: 'Failed to get application support directory',
      );
    }
    return result;
  }
}
