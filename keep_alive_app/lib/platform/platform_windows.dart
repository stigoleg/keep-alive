import 'dart:async';

import 'package:flutter/services.dart';

import '../core/constants.dart';
import '../models/battery_info.dart';
import 'platform_interface.dart';

class KeepAlivePlatformWindows extends KeepAlivePlatform {
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

  @override
  Future<void> waitUntilNativeReady() async {
    _trayEventController;

    for (var i = 0; i < 20; i++) {
      try {
        await _channel.invokeMethod<String>('getPlatformName');
        return;
      } on MissingPluginException {
        await Future<void>.delayed(const Duration(milliseconds: 100));
      }
    }
  }

  Future<dynamic> _handleMethodCall(MethodCall call) async {
    switch (call.method) {
      case AppConstants.methodOnTrayEvent:
        final event = call.arguments as String?;
        if (event != null) _trayEventController.add(event);
      case 'systemShutdown':
        _trayEventController.add('systemShutdown');
    }
  }

  @override
  Future<String> getPlatformName() async {
    final result = await _channel.invokeMethod<String>(
      AppConstants.methodGetPlatformName,
    );
    return result ?? 'Windows';
  }

  @override
  Future<void> setAutoStart(bool enabled) async {
    await _channel.invokeMethod(AppConstants.methodSetAutoStart, {
      'enabled': enabled,
    });
  }

  @override
  Future<bool> isAutoStartEnabled() async {
    final result = await _channel.invokeMethod<bool>(
      AppConstants.methodIsAutoStartEnabled,
    );
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
  Future<void> setStatusBarTitle(String title) async {
    await _channel.invokeMethod(AppConstants.methodSetStatusBarTitle, {
      'title': title,
    });
  }

  @override
  Future<int?> showContextMenu(List<String> items) async {
    final result = await _channel.invokeMethod<int>(
      AppConstants.methodShowContextMenu,
      {'items': items},
    );
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
    final result = await _channel.invokeMapMethod<String, dynamic>(
      AppConstants.methodGetBatteryInfo,
    );
    if (result == null) {
      return const BatteryInfo(percentage: 100.0, isPresent: false);
    }
    return BatteryInfo.fromJson(result);
  }

  @override
  Future<String?> getBundledCliPath() async => null;

  @override
  Future<bool> ensureActivitySimulationPermission() async => true;

  @override
  Future<void> assignProcessToJobObject(int pid) async {
    try {
      await _channel.invokeMethod(
        AppConstants.methodAssignProcessToJobObject,
        {'pid': pid},
      );
    } on MissingPluginException {
      // Older builds of the host without the Job Object channel — fall back
      // silently; the stale-process sweeper still catches orphans next launch.
    }
  }

  @override
  Future<void> activateExistingInstance() async {
    try {
      await _channel.invokeMethod(AppConstants.methodActivateExistingInstance);
    } on MissingPluginException {
      // Native side not present; nothing we can do from Dart.
    }
  }

  @override
  Future<String> getAppSupportDir() async {
    final result = await _channel.invokeMethod<String>(
      AppConstants.methodGetAppSupportDir,
    );
    if (result == null || result.isEmpty) {
      throw PlatformException(
        code: 'APP_SUPPORT_DIR_ERROR',
        message: 'Failed to get application support directory',
      );
    }
    return result;
  }
}
