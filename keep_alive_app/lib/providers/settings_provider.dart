import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/cli_flags.dart';
import '../repositories/settings_repository.dart';

class AppSettingsState {
  final bool keepAwake;
  final bool simulateActivity;
  final bool enableLogging;
  final int? batteryThreshold;
  final int? durationMinutes;
  final DateTime? clockTime;
  final bool autoStart;
  final bool startMinimized;

  const AppSettingsState({
    this.keepAwake = false,
    this.simulateActivity = false,
    this.enableLogging = false,
    this.batteryThreshold,
    this.durationMinutes,
    this.clockTime,
    this.autoStart = false,
    this.startMinimized = false,
  });

  CliFlags toCliFlags() => CliFlags(
        durationMinutes: durationMinutes,
        clockTime: clockTime,
        batteryThreshold: batteryThreshold,
        simulateActivity: simulateActivity,
        enableLogging: enableLogging,
      );

  AppSettingsState copyWith({
    bool? keepAwake,
    bool? simulateActivity,
    bool? enableLogging,
    Object? batteryThreshold = _clearBatteryThreshold,
    Object? durationMinutes = _clearDurationMinutes,
    Object? clockTime = _clearClockTime,
    bool? autoStart,
    bool? startMinimized,
  }) {
    return AppSettingsState(
      keepAwake: keepAwake ?? this.keepAwake,
      simulateActivity: simulateActivity ?? this.simulateActivity,
      enableLogging: enableLogging ?? this.enableLogging,
      batteryThreshold: identical(batteryThreshold, _clearBatteryThreshold)
          ? this.batteryThreshold
          : batteryThreshold as int?,
      durationMinutes: identical(durationMinutes, _clearDurationMinutes)
          ? this.durationMinutes
          : durationMinutes as int?,
      clockTime: identical(clockTime, _clearClockTime)
          ? this.clockTime
          : clockTime as DateTime?,
      autoStart: autoStart ?? this.autoStart,
      startMinimized: startMinimized ?? this.startMinimized,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is AppSettingsState &&
          keepAwake == other.keepAwake &&
          simulateActivity == other.simulateActivity &&
          enableLogging == other.enableLogging &&
          batteryThreshold == other.batteryThreshold &&
          durationMinutes == other.durationMinutes &&
          clockTime == other.clockTime &&
          autoStart == other.autoStart &&
          startMinimized == other.startMinimized;

  @override
  int get hashCode =>
      keepAwake.hashCode ^
      simulateActivity.hashCode ^
      enableLogging.hashCode ^
      batteryThreshold.hashCode ^
      durationMinutes.hashCode ^
      clockTime.hashCode ^
      autoStart.hashCode ^
      startMinimized.hashCode;

  @override
  String toString() =>
      'AppSettingsState(keepAwake: $keepAwake, simulateActivity: $simulateActivity, '
      'enableLogging: $enableLogging, batteryThreshold: $batteryThreshold, '
      'durationMinutes: $durationMinutes, clockTime: $clockTime, '
      'autoStart: $autoStart, startMinimized: $startMinimized)';

  static const _clearBatteryThreshold = Object();
  static const _clearDurationMinutes = Object();
  static const _clearClockTime = Object();
}

class AppSettingsNotifier extends Notifier<AppSettingsState> {
  late final SettingsRepository _repository;

  @override
  AppSettingsState build() {
    _repository = SettingsRepository();
    return const AppSettingsState();
  }

  Future<void> restoreFromDisk() async {
    state = AppSettingsState(
      keepAwake: await _repository.getKeepAwake(),
      simulateActivity: await _repository.getSimulateActivity(),
      enableLogging: await _repository.getEnableLogging(),
      batteryThreshold: await _repository.getBatteryThreshold(),
      durationMinutes: await _repository.getDurationMinutes(),
      clockTime: await _repository.getClockTime(),
      autoStart: await _repository.getAutoStart(),
      startMinimized: await _repository.getStartMinimized(),
    );
  }

  Future<void> saveToDisk() async {
    final s = state;
    await _repository.setKeepAwake(s.keepAwake);
    await _repository.setSimulateActivity(s.simulateActivity);
    await _repository.setEnableLogging(s.enableLogging);
    await _repository.setBatteryThreshold(s.batteryThreshold);
    await _repository.setDurationMinutes(s.durationMinutes);
    await _repository.setClockTime(s.clockTime);
    await _repository.setAutoStart(s.autoStart);
    await _repository.setStartMinimized(s.startMinimized);
  }

  Future<void> setKeepAwake(bool value) async {
    await _repository.setKeepAwake(value);
    state = state.copyWith(keepAwake: value);
  }

  Future<void> setSimulateActivity(bool value) async {
    await _repository.setSimulateActivity(value);
    state = state.copyWith(simulateActivity: value);
  }

  Future<void> setEnableLogging(bool value) async {
    await _repository.setEnableLogging(value);
    state = state.copyWith(enableLogging: value);
  }

  Future<void> setBatteryThreshold(int? value) async {
    await _repository.setBatteryThreshold(value);
    state = state.copyWith(batteryThreshold: value);
  }

  Future<void> setDurationMinutes(int? value) async {
    await _repository.setDurationMinutes(value);
    state = state.copyWith(durationMinutes: value);
  }

  Future<void> setClockTime(DateTime? value) async {
    await _repository.setClockTime(value);
    state = state.copyWith(clockTime: value);
  }

  Future<void> setAutoStart(bool value) async {
    await _repository.setAutoStart(value);
    state = state.copyWith(autoStart: value);
  }

  Future<void> setStartMinimized(bool value) async {
    await _repository.setStartMinimized(value);
    state = state.copyWith(startMinimized: value);
  }
}

final appSettingsProvider =
    NotifierProvider<AppSettingsNotifier, AppSettingsState>(
  AppSettingsNotifier.new,
);
