import 'dart:async';

import '../core/constants.dart';
import '../core/logger.dart';
import '../models/battery_info.dart';
import '../platform/platform_interface.dart';

class BatteryMonitor {
  final KeepAlivePlatform _platform;

  final StreamController<BatteryInfo> _batteryController =
      StreamController<BatteryInfo>.broadcast();

  Timer? _pollTimer;
  BatteryInfo? _lastEmitted;

  BatteryMonitor({KeepAlivePlatform? platform})
      : _platform = platform ?? KeepAlivePlatform.instance;

  Stream<BatteryInfo> get batteryStream => _batteryController.stream;

  Future<BatteryInfo> getCurrentBattery() async {
    try {
      return await _platform.getBatteryInfo();
    } on Exception catch (e) {
      AppLogger.warning('Failed to read battery info: $e');
      return const BatteryInfo(percentage: 100.0, isPresent: false);
    }
  }

  void startPolling() {
    if (_pollTimer?.isActive ?? false) return;

    _emitCurrentBattery();

    _pollTimer = Timer.periodic(AppConstants.batteryPollInterval, (_) {
      _emitCurrentBattery();
    });
  }

  void stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  Future<void> _emitCurrentBattery() async {
    final info = await getCurrentBattery();
    if (_lastEmitted != info) {
      _lastEmitted = info;
      if (!_batteryController.isClosed) {
        _batteryController.add(info);
      }
    }
  }

  void dispose() {
    stopPolling();
    _batteryController.close();
  }
}
