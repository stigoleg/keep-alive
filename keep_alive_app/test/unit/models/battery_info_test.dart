import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/battery_info.dart';

void main() {
  group('BatteryInfo', () {
    test('defaults', () {
      const info = BatteryInfo(percentage: 50.0);
      expect(info.percentage, 50.0);
      expect(info.isCharging, isFalse);
      expect(info.isPresent, isTrue);
    });

    group('copyWith', () {
      test('copies all fields unchanged', () {
        const original = BatteryInfo(
          percentage: 75.0,
          isCharging: true,
          isPresent: false,
        );
        final copied = original.copyWith();
        expect(copied, original);
      });

      test('updates specific fields', () {
        const original = BatteryInfo(percentage: 50.0);
        final updated = original.copyWith(percentage: 80.0, isCharging: true);
        expect(updated.percentage, 80.0);
        expect(updated.isCharging, isTrue);
        expect(updated.isPresent, isTrue);
      });
    });

    group('equality', () {
      test('identical values are equal', () {
        const a = BatteryInfo(percentage: 60.0, isCharging: true);
        const b = BatteryInfo(percentage: 60.0, isCharging: true);
        expect(a, equals(b));
      });

      test('different values are not equal', () {
        const a = BatteryInfo(percentage: 60.0);
        const b = BatteryInfo(percentage: 70.0);
        expect(a, isNot(equals(b)));
      });

      test('hashCode matches for equal values', () {
        const a = BatteryInfo(percentage: 42.0, isCharging: false, isPresent: true);
        const b = BatteryInfo(percentage: 42.0, isCharging: false, isPresent: true);
        expect(a.hashCode, b.hashCode);
      });
    });

    group('JSON serialization', () {
      test('roundtrip preserves all fields', () {
        const original = BatteryInfo(
          percentage: 88.5,
          isCharging: true,
          isPresent: false,
        );
        final json = original.toJson();
        final restored = BatteryInfo.fromJson(json);
        expect(restored.percentage, original.percentage);
        expect(restored.isCharging, original.isCharging);
        expect(restored.isPresent, original.isPresent);
        expect(restored, original);
      });

      test('fromJson handles missing optional fields with defaults', () {
        final restored = BatteryInfo.fromJson({'percentage': 30.0});
        expect(restored.percentage, 30.0);
        expect(restored.isCharging, isFalse);
        expect(restored.isPresent, isTrue);
      });
    });

    group('toString', () {
      test('produces descriptive string', () {
        const info = BatteryInfo(percentage: 100.0, isCharging: true);
        final str = info.toString();
        expect(str, contains('BatteryInfo'));
        expect(str, contains('100'));
      });
    });
  });
}
