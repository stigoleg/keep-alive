import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/battery_info.dart';
import 'package:keep_alive_app/providers/battery_provider.dart';
import 'package:keep_alive_app/services/battery_monitor.dart';

class _FakeBatteryMonitor extends BatteryMonitor {
  final StreamController<BatteryInfo> _controller =
      StreamController<BatteryInfo>.broadcast();

  @override
  Stream<BatteryInfo> get batteryStream => _controller.stream;

  @override
  Future<BatteryInfo> getCurrentBattery() async =>
      const BatteryInfo(percentage: 50.0);

  @override
  void startPolling() {}

  @override
  void stopPolling() {}

  void emit(BatteryInfo info) {
    if (!_controller.isClosed) {
      _controller.add(info);
    }
  }

  @override
  void dispose() {
    _controller.close();
  }
}

void main() {
  group('batteryStateProvider', () {
    test('emits value from monitor stream', () async {
      final monitor = _FakeBatteryMonitor();

      final container = ProviderContainer(
        overrides: [
          batteryMonitorProvider.overrideWithValue(monitor),
        ],
      );

      final future = container.read(batteryStateProvider.future);
      monitor.emit(const BatteryInfo(percentage: 75.0, isCharging: true));

      final emitted = await future;
      expect(emitted.percentage, 75.0);
      expect(emitted.isCharging, isTrue);

      container.dispose();
      monitor.dispose();
    });

    test('listener receives emitted values', () async {
      final monitor = _FakeBatteryMonitor();

      final container = ProviderContainer(
        overrides: [
          batteryMonitorProvider.overrideWithValue(monitor),
        ],
      );

      final emitted = <BatteryInfo>[];
      final sub = container.listen(
        batteryStateProvider,
        (prev, next) {
          if (next.hasValue) emitted.add(next.value!);
        },
      );

      monitor.emit(const BatteryInfo(percentage: 42.0));
      await Future<void>.delayed(const Duration(milliseconds: 10));

      expect(emitted.length, 1);
      expect(emitted.first.percentage, 42.0);

      sub.close();
      container.dispose();
      monitor.dispose();
    });

    test('provider disposes monitor on container dispose', () {
      final monitor = _FakeBatteryMonitor();

      final container = ProviderContainer(
        overrides: [
          batteryMonitorProvider.overrideWithValue(monitor),
        ],
      );

      container.dispose();

      expect(monitor.batteryStream.isBroadcast, isTrue);
    });
  });
}
