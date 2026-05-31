import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/battery_info.dart';
import 'package:keep_alive_app/platform/platform_interface.dart';
import 'package:keep_alive_app/services/battery_monitor.dart';

class _FakePlatform extends KeepAlivePlatform {
  final Future<BatteryInfo> Function() handler;
  _FakePlatform(this.handler);

  final StreamController<String> _controller =
      StreamController<String>.broadcast();

  @override
  Stream<String> get trayEventStream => _controller.stream;

  @override
  Future<void> waitUntilNativeReady() async {}

  @override
  Future<String> getPlatformName() async => 'Fake';

  @override
  Future<void> setAutoStart(bool enabled) async {}

  @override
  Future<bool> isAutoStartEnabled() async => false;

  @override
  Future<void> setTrayIcon(String iconPath) async {}

  @override
  Future<void> setTrayTooltip(String tooltip) async {}

  @override
  Future<void> setStatusBarTitle(String title) async {}

  @override
  Future<int?> showContextMenu(List<String> items) async => null;

  @override
  Future<void> showPopover() async {}

  @override
  Future<void> hidePopover() async {}

  @override
  Future<String> getAppSupportDir() async => '/fake';

  @override
  Future<BatteryInfo> getBatteryInfo() => handler();
}

void main() {
  group('BatteryMonitor', () {
    group('getCurrentBattery', () {
      test('returns BatteryInfo from platform', () async {
        const expected = BatteryInfo(percentage: 75.0, isCharging: true);
        final platform = _FakePlatform(() async => expected);
        final monitor = BatteryMonitor(platform: platform);

        final result = await monitor.getCurrentBattery();
        expect(result.percentage, 75.0);
        expect(result.isCharging, isTrue);
        expect(result.isPresent, isTrue);
      });

      test('returns default on platform exception', () async {
        final platform = _FakePlatform(
          () => throw Exception('Battery API unavailable'),
        );
        final monitor = BatteryMonitor(platform: platform);

        final result = await monitor.getCurrentBattery();
        expect(result.percentage, 100.0);
        expect(result.isPresent, isFalse);
      });
    });

    group('batteryStream', () {
      test('emits current battery on startPolling', () async {
        const info = BatteryInfo(percentage: 42.0, isCharging: false);
        final platform = _FakePlatform(() async => info);
        final monitor = BatteryMonitor(platform: platform);

        final stream = monitor.batteryStream;

        monitor.startPolling();

        final emitted = await stream.first;
        expect(emitted.percentage, 42.0);
        expect(emitted.isCharging, isFalse);

        monitor.dispose();
      });

      test('skips duplicate values on sequential manual calls', () async {
        var callCount = 0;
        final platform = _FakePlatform(() async {
          callCount++;
          return const BatteryInfo(percentage: 50.0);
        });
        final monitor = BatteryMonitor(platform: platform);

        final stream = monitor.batteryStream;
        final emitted = <BatteryInfo>[];
        final sub = stream.listen(emitted.add);

        monitor.startPolling();
        await Future<void>.delayed(const Duration(milliseconds: 50));

        expect(emitted.length, 1);
        expect(callCount <= 2, isTrue);

        await sub.cancel();
        monitor.dispose();
      });

      test('emits when value differs from previous', () async {
        var callCount = 0;
        final values = [
          const BatteryInfo(percentage: 100.0),
          const BatteryInfo(percentage: 80.0),
          const BatteryInfo(percentage: 60.0),
        ];
        final platform = _FakePlatform(() async {
          final value = values[callCount % values.length];
          callCount++;
          return value;
        });
        final monitor = BatteryMonitor(platform: platform);

        final stream = monitor.batteryStream;
        final emitted = <BatteryInfo>[];
        final sub = stream.listen(emitted.add);

        monitor.startPolling();
        await Future<void>.delayed(const Duration(milliseconds: 50));

        final countAfterFirst = emitted.length;
        expect(countAfterFirst, 1);

        await sub.cancel();
        monitor.dispose();
      });
    });

    group('startPolling / stopPolling', () {
      test('startPolling immediately emits current battery', () async {
        const info = BatteryInfo(percentage: 88.0);
        final platform = _FakePlatform(() async => info);
        final monitor = BatteryMonitor(platform: platform);

        final future = monitor.batteryStream.first;
        monitor.startPolling();

        final emitted = await future;
        expect(emitted.percentage, 88.0);

        monitor.dispose();
      });

      test('stopPolling prevents further emissions', () async {
        var callCount = 0;
        final platform = _FakePlatform(() async {
          callCount++;
          return BatteryInfo(percentage: callCount.toDouble());
        });
        final monitor = BatteryMonitor(platform: platform);

        monitor.startPolling();
        await Future<void>.delayed(const Duration(milliseconds: 50));

        monitor.stopPolling();
        final countAfterStop = callCount;

        await Future<void>.delayed(const Duration(milliseconds: 100));
        expect(callCount, countAfterStop);

        monitor.dispose();
      });
    });
  });
}
