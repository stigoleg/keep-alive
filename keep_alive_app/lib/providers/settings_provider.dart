import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/cli_flags.dart';
import '../repositories/settings_repository.dart';

class AppSettingsState {
  final bool keepAwake;
  final bool simulateActivity;
  final bool enableLogging;
  final int? batteryThreshold;
  final bool batteryThresholdEnabled;
  final int? durationMinutes;
  final DateTime? clockTime;
  final bool autoStart;
  final bool startMinimized;
  final bool showCountdownInMenuBar;

  const AppSettingsState({
    this.keepAwake = false,
    this.simulateActivity = false,
    this.enableLogging = false,
    this.batteryThreshold,
    this.batteryThresholdEnabled = false,
    this.durationMinutes,
    this.clockTime,
    this.autoStart = false,
    this.startMinimized = false,
    this.showCountdownInMenuBar = false,
  });

  CliFlags toCliFlags() => CliFlags(
        durationMinutes: durationMinutes,
        clockTime: clockTime,
        batteryThreshold: batteryThresholdEnabled ? batteryThreshold : null,
        simulateActivity: simulateActivity,
        enableLogging: enableLogging,
      );

  AppSettingsState copyWith({
    bool? keepAwake,
    bool? simulateActivity,
    bool? enableLogging,
    Object? batteryThreshold = _clearBatteryThreshold,
    bool? batteryThresholdEnabled,
    Object? durationMinutes = _clearDurationMinutes,
    Object? clockTime = _clearClockTime,
    bool? autoStart,
    bool? startMinimized,
    bool? showCountdownInMenuBar,
  }) {
    return AppSettingsState(
      keepAwake: keepAwake ?? this.keepAwake,
      simulateActivity: simulateActivity ?? this.simulateActivity,
      enableLogging: enableLogging ?? this.enableLogging,
      batteryThreshold: identical(batteryThreshold, _clearBatteryThreshold)
          ? this.batteryThreshold
          : batteryThreshold as int?,
      batteryThresholdEnabled:
          batteryThresholdEnabled ?? this.batteryThresholdEnabled,
      durationMinutes: identical(durationMinutes, _clearDurationMinutes)
          ? this.durationMinutes
          : durationMinutes as int?,
      clockTime: identical(clockTime, _clearClockTime)
          ? this.clockTime
          : clockTime as DateTime?,
      autoStart: autoStart ?? this.autoStart,
      startMinimized: startMinimized ?? this.startMinimized,
      showCountdownInMenuBar:
          showCountdownInMenuBar ?? this.showCountdownInMenuBar,
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
          batteryThresholdEnabled == other.batteryThresholdEnabled &&
          durationMinutes == other.durationMinutes &&
          clockTime == other.clockTime &&
          autoStart == other.autoStart &&
          startMinimized == other.startMinimized &&
          showCountdownInMenuBar == other.showCountdownInMenuBar;

  @override
  int get hashCode =>
      keepAwake.hashCode ^
      simulateActivity.hashCode ^
      enableLogging.hashCode ^
      batteryThreshold.hashCode ^
      batteryThresholdEnabled.hashCode ^
      durationMinutes.hashCode ^
      clockTime.hashCode ^
      autoStart.hashCode ^
      startMinimized.hashCode ^
      showCountdownInMenuBar.hashCode;

  @override
  String toString() =>
      'AppSettingsState(keepAwake: $keepAwake, simulateActivity: $simulateActivity, '
      'enableLogging: $enableLogging, batteryThreshold: $batteryThreshold, '
      'batteryThresholdEnabled: $batteryThresholdEnabled, '
      'durationMinutes: $durationMinutes, clockTime: $clockTime, '
      'autoStart: $autoStart, startMinimized: $startMinimized, '
      'showCountdownInMenuBar: $showCountdownInMenuBar)';

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
      batteryThresholdEnabled: await _repository.getBatteryThresholdEnabled(),
      durationMinutes: await _repository.getDurationMinutes(),
      clockTime: await _repository.getClockTime(),
      autoStart: await _repository.getAutoStart(),
      startMinimized: await _repository.getStartMinimized(),
      showCountdownInMenuBar: await _repository.getShowCountdownInMenuBar(),
    );
  }

  Future<void> saveToDisk() async {
    final s = state;
    await _repository.setKeepAwake(s.keepAwake);
    await _repository.setSimulateActivity(s.simulateActivity);
    await _repository.setEnableLogging(s.enableLogging);
    await _repository.setBatteryThreshold(s.batteryThreshold);
    await _repository.setBatteryThresholdEnabled(s.batteryThresholdEnabled);
    await _repository.setDurationMinutes(s.durationMinutes);
    await _repository.setClockTime(s.clockTime);
    await _repository.setAutoStart(s.autoStart);
    await _repository.setStartMinimized(s.startMinimized);
    await _repository.setShowCountdownInMenuBar(s.showCountdownInMenuBar);
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

  Future<void> setBatteryThresholdEnabled(bool value) async {
    await _repository.setBatteryThresholdEnabled(value);
    state = state.copyWith(batteryThresholdEnabled: value);
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

  Future<void> setShowCountdownInMenuBar(bool value) async {
    await _repository.setShowCountdownInMenuBar(value);
    state = state.copyWith(showCountdownInMenuBar: value);
  }
}

final appSettingsProvider =
    NotifierProvider<AppSettingsNotifier, AppSettingsState>(
  AppSettingsNotifier.new,
);
