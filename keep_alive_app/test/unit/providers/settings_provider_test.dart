import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/providers/settings_provider.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  group('AppSettingsState', () {
    test('defaults have correct values', () {
      const state = AppSettingsState();
      expect(state.keepAwake, isFalse);
      expect(state.simulateActivity, isFalse);
      expect(state.enableLogging, isFalse);
      expect(state.batteryThreshold, isNull);
      expect(state.durationMinutes, isNull);
      expect(state.clockTime, isNull);
      expect(state.autoStart, isFalse);
      expect(state.startMinimized, isFalse);
    });

    test('toCliFlags converts correctly', () {
      final clockTime = DateTime(2025, 1, 15, 17, 0);
      final state = AppSettingsState(
        durationMinutes: 60,
        clockTime: clockTime,
        batteryThreshold: 30,
        batteryThresholdEnabled: true,
        simulateActivity: true,
        enableLogging: true,
      );

      final flags = state.toCliFlags();
      expect(flags.durationMinutes, 60);
      expect(flags.clockTime, clockTime);
      expect(flags.batteryThreshold, 30);
      expect(flags.simulateActivity, isTrue);
      expect(flags.enableLogging, isTrue);
    });

    test('toCliFlags with defaults produces empty flags', () {
      const state = AppSettingsState();
      final flags = state.toCliFlags();
      expect(flags.durationMinutes, isNull);
      expect(flags.clockTime, isNull);
      expect(flags.batteryThreshold, isNull);
      expect(flags.simulateActivity, isFalse);
      expect(flags.enableLogging, isFalse);
    });

    test('copyWith updates fields', () {
      const original = AppSettingsState();
      final updated = original.copyWith(
        keepAwake: true,
        batteryThreshold: 50,
      );
      expect(updated.keepAwake, isTrue);
      expect(updated.batteryThreshold, 50);
      expect(updated.simulateActivity, isFalse);
    });

    test('copyWith clears nullable fields with null', () {
      const original = AppSettingsState(
        batteryThreshold: 30,
        durationMinutes: 120,
      );
      final updated = original.copyWith(
        batteryThreshold: null,
        durationMinutes: null,
      );
      expect(updated.batteryThreshold, isNull);
      expect(updated.durationMinutes, isNull);
    });

    test('equality works correctly', () {
      const a = AppSettingsState(keepAwake: true, batteryThreshold: 30);
      const b = AppSettingsState(keepAwake: true, batteryThreshold: 30);
      const c = AppSettingsState(keepAwake: false, batteryThreshold: 30);
      const d = AppSettingsState(keepAwake: true, batteryThreshold: 50);

      expect(a, equals(b));
      expect(a, isNot(equals(c)));
      expect(a, isNot(equals(d)));
    });

    test('hashCode is consistent with equality', () {
      const a = AppSettingsState(keepAwake: true, batteryThreshold: 30);
      const b = AppSettingsState(keepAwake: true, batteryThreshold: 30);
      expect(a.hashCode, equals(b.hashCode));
    });

    test('toString contains field values', () {
      const state = AppSettingsState(
        keepAwake: true,
        batteryThreshold: 20,
      );
      final str = state.toString();
      expect(str, contains('keepAwake: true'));
      expect(str, contains('batteryThreshold: 20'));
    });
  });

  group('AppSettingsNotifier', () {
    late ProviderContainer container;

    setUp(() async {
      SharedPreferences.setMockInitialValues({});
      container = ProviderContainer();
    });

    tearDown(() {
      container.dispose();
    });

    test('initial state has defaults', () {
      final state = container.read(appSettingsProvider);
      expect(state.keepAwake, isFalse);
      expect(state.simulateActivity, isFalse);
      expect(state.enableLogging, isFalse);
      expect(state.autoStart, isFalse);
      expect(state.startMinimized, isFalse);
    });

    test('setKeepAwake updates state', () async {
      await container.read(appSettingsProvider.notifier).setKeepAwake(true);
      expect(container.read(appSettingsProvider).keepAwake, isTrue);
    });

    test('setSimulateActivity updates state', () async {
      await container.read(appSettingsProvider.notifier).setSimulateActivity(true);
      expect(container.read(appSettingsProvider).simulateActivity, isTrue);
    });

    test('setEnableLogging updates state', () async {
      await container.read(appSettingsProvider.notifier).setEnableLogging(true);
      expect(container.read(appSettingsProvider).enableLogging, isTrue);
    });

    test('setBatteryThreshold updates state with value', () async {
      await container.read(appSettingsProvider.notifier).setBatteryThreshold(30);
      expect(container.read(appSettingsProvider).batteryThreshold, 30);
    });

    test('setBatteryThreshold updates state with null', () async {
      await container.read(appSettingsProvider.notifier).setBatteryThreshold(50);
      await container.read(appSettingsProvider.notifier).setBatteryThreshold(null);
      expect(container.read(appSettingsProvider).batteryThreshold, isNull);
    });

    test('setDurationMinutes updates state', () async {
      await container.read(appSettingsProvider.notifier).setDurationMinutes(120);
      expect(container.read(appSettingsProvider).durationMinutes, 120);
    });

    test('setClockTime updates state', () async {
      final dt = DateTime(2025, 6, 1, 17, 0);
      await container.read(appSettingsProvider.notifier).setClockTime(dt);
      expect(container.read(appSettingsProvider).clockTime, dt);
    });

    test('setClockTime clears with null', () async {
      final dt = DateTime(2025, 6, 1, 17, 0);
      await container.read(appSettingsProvider.notifier).setClockTime(dt);
      await container.read(appSettingsProvider.notifier).setClockTime(null);
      expect(container.read(appSettingsProvider).clockTime, isNull);
    });

    test('setAutoStart updates state', () async {
      await container.read(appSettingsProvider.notifier).setAutoStart(true);
      expect(container.read(appSettingsProvider).autoStart, isTrue);
    });

    test('setStartMinimized updates state', () async {
      await container.read(appSettingsProvider.notifier).setStartMinimized(true);
      expect(container.read(appSettingsProvider).startMinimized, isTrue);
    });

    test('saveToDisk and restoreFromDisk roundtrip', () async {
      final notifier = container.read(appSettingsProvider.notifier);
      await notifier.setKeepAwake(true);
      await notifier.setBatteryThreshold(25);
      await notifier.setDurationMinutes(90);
      await notifier.setSimulateActivity(true);

      final before = container.read(appSettingsProvider);

      final container2 = ProviderContainer();
      await container2.read(appSettingsProvider.notifier).restoreFromDisk();
      final after = container2.read(appSettingsProvider);

      expect(after.keepAwake, before.keepAwake);
      expect(after.batteryThreshold, before.batteryThreshold);
      expect(after.durationMinutes, before.durationMinutes);
      expect(after.simulateActivity, before.simulateActivity);

      container2.dispose();
    });

    test('restoreFromDisk persists values between providers', () async {
      final notifier = container.read(appSettingsProvider.notifier);
      await notifier.setKeepAwake(true);
      await notifier.setEnableLogging(true);
      await notifier.setBatteryThreshold(40);
      await notifier.setDurationMinutes(30);

      final container2 = ProviderContainer();
      await container2.read(appSettingsProvider.notifier).restoreFromDisk();
      final state = container2.read(appSettingsProvider);

      expect(state.keepAwake, isTrue);
      expect(state.enableLogging, isTrue);
      expect(state.batteryThreshold, 40);
      expect(state.durationMinutes, 30);

      container2.dispose();
    });

    test('multiple updates are notified to listeners', () async {
      final states = <AppSettingsState>[];
      container.listen(appSettingsProvider, (prev, next) {
        states.add(next);
      });

      await container.read(appSettingsProvider.notifier).setKeepAwake(true);
      await container.read(appSettingsProvider.notifier).setBatteryThreshold(60);
      await container.read(appSettingsProvider.notifier).setSimulateActivity(true);

      expect(states.length, 3);
      expect(states[0].keepAwake, isTrue);
      expect(states[1].batteryThreshold, 60);
      expect(states[2].simulateActivity, isTrue);
    });
  });
}
