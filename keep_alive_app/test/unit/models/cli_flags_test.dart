import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/cli_flags.dart';

void main() {
  group('CliFlags', () {
    group('toArgs', () {
      test('empty flags emits empty args', () {
        const flags = CliFlags();
        expect(flags.toArgs(), []);
      });

      test('enableLogging emits --log', () {
        const flags = CliFlags(enableLogging: true);
        expect(flags.toArgs(), ['--log']);
      });

      test('durationMinutes emits --duration flag', () {
        const flags = CliFlags(durationMinutes: 120, enableLogging: true);
        expect(flags.toArgs(), ['--duration', '120', '--log']);
      });

      test('clockTime emits --clock flag in HH:mm format', () {
        final clockTime = DateTime(2025, 1, 1, 17, 30);
        final flags = CliFlags(clockTime: clockTime, enableLogging: true);
        expect(flags.toArgs(), ['--clock', '17:30', '--log']);
      });

      test('clockTime pads single-digit hours', () {
        final clockTime = DateTime(2025, 1, 1, 9, 5);
        final flags = CliFlags(clockTime: clockTime, enableLogging: true);
        expect(flags.toArgs(), ['--clock', '09:05', '--log']);
      });

      test('batteryThreshold emits --battery flag', () {
        const flags = CliFlags(batteryThreshold: 30, enableLogging: true);
        expect(flags.toArgs(), ['--battery', '30', '--log']);
      });

      test('simulateActivity emits --active flag', () {
        const flags = CliFlags(simulateActivity: true, enableLogging: true);
        expect(flags.toArgs(), ['--active', '--log']);
      });

      test('all flags combined', () {
        const flags = CliFlags(
          durationMinutes: 90,
          batteryThreshold: 20,
          simulateActivity: true,
          enableLogging: true,
        );
        final args = flags.toArgs();
        expect(args, containsAll(['--duration', '90']));
        expect(args, containsAll(['--battery', '20']));
        expect(args, contains('--active'));
        expect(args.last, '--log');
      });

      test('--log is omitted when enableLogging is false', () {
        const flags = CliFlags(
          durationMinutes: 90,
          simulateActivity: true,
        );
        final args = flags.toArgs();
        expect(args, isNot(contains('--log')));
      });

      test('both duration and clock time flags are emitted if both set', () {
        final clockTime = DateTime(2025, 1, 1, 18, 0);
        final flags = CliFlags(durationMinutes: 60, clockTime: clockTime, enableLogging: true);
        final args = flags.toArgs();
        expect(args, containsAll(['--duration', '60']));
        expect(args, containsAll(['--clock', '18:00']));
      });
    });

    group('copyWith', () {
      test('copies all fields unchanged', () {
        final clockTime = DateTime(2025, 1, 1, 12, 0);
        final original = CliFlags(
          durationMinutes: 30,
          clockTime: clockTime,
          batteryThreshold: 50,
          simulateActivity: true,
          enableLogging: true,
        );
        final copied = original.copyWith();
        expect(copied, original);
      });

      test('clears nullable fields when passed null', () {
        final clockTime = DateTime(2025, 1, 1, 12, 0);
        final original = CliFlags(
          durationMinutes: 30,
          clockTime: clockTime,
          batteryThreshold: 50,
        );
        final cleared = original.copyWith(
          durationMinutes: null,
          clockTime: null,
          batteryThreshold: null,
        );
        expect(cleared.durationMinutes, isNull);
        expect(cleared.clockTime, isNull);
        expect(cleared.batteryThreshold, isNull);
      });

      test('updates specific fields', () {
        const original = CliFlags(durationMinutes: 30);
        final updated = original.copyWith(durationMinutes: 60);
        expect(updated.durationMinutes, 60);
        expect(updated.batteryThreshold, isNull);
      });
    });

    group('equality', () {
      test('identical flags are equal', () {
        const a = CliFlags(durationMinutes: 30, simulateActivity: true);
        const b = CliFlags(durationMinutes: 30, simulateActivity: true);
        expect(a, equals(b));
      });

      test('different flags are not equal', () {
        const a = CliFlags(durationMinutes: 30);
        const b = CliFlags(durationMinutes: 60);
        expect(a, isNot(equals(b)));
      });

      test('hashCode matches for equal flags', () {
        final clockTime = DateTime(2025, 1, 1, 12, 0);
        final a = CliFlags(clockTime: clockTime, batteryThreshold: 20);
        final b = CliFlags(clockTime: clockTime, batteryThreshold: 20);
        expect(a.hashCode, b.hashCode);
      });
    });

    group('JSON serialization', () {
      test('roundtrip preserves all fields', () {
        final clockTime = DateTime(2025, 6, 15, 14, 30);
        final original = CliFlags(
          durationMinutes: 45,
          clockTime: clockTime,
          batteryThreshold: 25,
          simulateActivity: true,
          enableLogging: true,
        );
        final json = original.toJson();
        final restored = CliFlags.fromJson(json);
        expect(restored, original);
      });

      test('roundtrip with null optional fields', () {
        const original = CliFlags(simulateActivity: false);
        final json = original.toJson();
        final restored = CliFlags.fromJson(json);
        expect(restored, original);
      });

      test('fromJson handles missing optional fields', () {
        final restored = CliFlags.fromJson({});
        expect(restored.durationMinutes, isNull);
        expect(restored.clockTime, isNull);
        expect(restored.batteryThreshold, isNull);
        expect(restored.simulateActivity, isFalse);
        expect(restored.enableLogging, isFalse);
      });
    });

    group('toString', () {
      test('produces descriptive string', () {
        const flags = CliFlags(durationMinutes: 30, simulateActivity: true);
        final str = flags.toString();
        expect(str, contains('CliFlags'));
        expect(str, contains('30'));
        expect(str, contains('true'));
      });
    });
  });
}
