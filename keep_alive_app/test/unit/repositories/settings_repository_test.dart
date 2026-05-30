import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/repositories/settings_repository.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  group('SettingsRepository', () {
    late SettingsRepository repository;

    setUp(() {
      SharedPreferences.setMockInitialValues({});
      repository = SettingsRepository();
    });

    group('bool settings', () {
      test('keepAwake defaults to false', () async {
        final value = await repository.getKeepAwake();
        expect(value, isFalse);
      });

      test('keepAwake roundtrip', () async {
        await repository.setKeepAwake(true);
        final value = await repository.getKeepAwake();
        expect(value, isTrue);
      });

      test('simulateActivity defaults to false', () async {
        final value = await repository.getSimulateActivity();
        expect(value, isFalse);
      });

      test('simulateActivity roundtrip', () async {
        await repository.setSimulateActivity(true);
        final value = await repository.getSimulateActivity();
        expect(value, isTrue);
      });

      test('enableLogging defaults to false', () async {
        final value = await repository.getEnableLogging();
        expect(value, isFalse);
      });

      test('enableLogging roundtrip', () async {
        await repository.setEnableLogging(true);
        final value = await repository.getEnableLogging();
        expect(value, isTrue);
      });

      test('autoStart defaults to false', () async {
        final value = await repository.getAutoStart();
        expect(value, isFalse);
      });

      test('autoStart roundtrip', () async {
        await repository.setAutoStart(true);
        final value = await repository.getAutoStart();
        expect(value, isTrue);
      });

      test('startMinimized defaults to false', () async {
        final value = await repository.getStartMinimized();
        expect(value, isFalse);
      });

      test('startMinimized roundtrip', () async {
        await repository.setStartMinimized(true);
        final value = await repository.getStartMinimized();
        expect(value, isTrue);
      });
    });

    group('nullable int settings', () {
      test('batteryThreshold defaults to null', () async {
        final value = await repository.getBatteryThreshold();
        expect(value, isNull);
      });

      test('batteryThreshold roundtrip', () async {
        await repository.setBatteryThreshold(30);
        var value = await repository.getBatteryThreshold();
        expect(value, 30);

        await repository.setBatteryThreshold(null);
        value = await repository.getBatteryThreshold();
        expect(value, isNull);
      });

      test('durationMinutes defaults to null', () async {
        final value = await repository.getDurationMinutes();
        expect(value, isNull);
      });

      test('durationMinutes roundtrip', () async {
        await repository.setDurationMinutes(120);
        var value = await repository.getDurationMinutes();
        expect(value, 120);

        await repository.setDurationMinutes(null);
        value = await repository.getDurationMinutes();
        expect(value, isNull);
      });
    });

    group('DateTime settings', () {
      test('clockTime defaults to null', () async {
        final value = await repository.getClockTime();
        expect(value, isNull);
      });

      test('clockTime roundtrip', () async {
        final now = DateTime(2025, 6, 15, 14, 30);
        await repository.setClockTime(now);
        final value = await repository.getClockTime();
        expect(value, now);
      });

      test('clockTime clears with null', () async {
        final now = DateTime(2025, 6, 15, 14, 30);
        await repository.setClockTime(now);
        await repository.setClockTime(null);
        final value = await repository.getClockTime();
        expect(value, isNull);
      });
    });

    group('persistence across instances', () {
      test('values survive new repository instances', () async {
        await repository.setKeepAwake(true);
        await repository.setBatteryThreshold(25);
        await repository.setDurationMinutes(60);

        final newRepository = SettingsRepository();
        expect(await newRepository.getKeepAwake(), isTrue);
        expect(await newRepository.getBatteryThreshold(), 25);
        expect(await newRepository.getDurationMinutes(), 60);
      });
    });

    group('all settings together', () {
      test('persist and restore all settings', () async {
        final clockTime = DateTime(2025, 12, 25, 10, 0);

        await repository.setKeepAwake(true);
        await repository.setSimulateActivity(true);
        await repository.setEnableLogging(true);
        await repository.setBatteryThreshold(20);
        await repository.setDurationMinutes(90);
        await repository.setClockTime(clockTime);
        await repository.setAutoStart(true);
        await repository.setStartMinimized(true);

        final newRepository = SettingsRepository();
        expect(await newRepository.getKeepAwake(), isTrue);
        expect(await newRepository.getSimulateActivity(), isTrue);
        expect(await newRepository.getEnableLogging(), isTrue);
        expect(await newRepository.getBatteryThreshold(), 20);
        expect(await newRepository.getDurationMinutes(), 90);
        expect(await newRepository.getClockTime(), clockTime);
        expect(await newRepository.getAutoStart(), isTrue);
        expect(await newRepository.getStartMinimized(), isTrue);
      });
    });
  });
}
