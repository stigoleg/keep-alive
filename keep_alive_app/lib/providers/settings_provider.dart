import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/constants.dart';
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
  Timer? _flushDebounce;

  @override
  AppSettingsState build() {
    _repository = SettingsRepository();
    ref.onDispose(() {
      _flushDebounce?.cancel();
      _flushDebounce = null;
    });
    return const AppSettingsState();
  }

  void _scheduleFlush() {
    _flushDebounce?.cancel();
    _flushDebounce = Timer(
      const Duration(milliseconds: AppConstants.settingsFlushDebounceMs),
      () {
        _flushDebounce = null;
        unawaited(_writeAll(state));
      },
    );
  }

  Future<void> _writeAll(AppSettingsState s) async {
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

  Future<void> restoreFromDisk() async {
    state = AppSettingsState(
      keepAwake: false,
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
    _flushDebounce?.cancel();
    _flushDebounce = null;
    await _writeAll(state);
  }

  Future<void> setKeepAwake(bool value) async {
    state = state.copyWith(keepAwake: value);
    _scheduleFlush();
  }

  Future<void> setSimulateActivity(bool value) async {
    state = state.copyWith(simulateActivity: value);
    _scheduleFlush();
  }

  Future<void> setEnableLogging(bool value) async {
    state = state.copyWith(enableLogging: value);
    _scheduleFlush();
  }

  Future<void> setBatteryThreshold(int? value) async {
    state = state.copyWith(batteryThreshold: value);
    _scheduleFlush();
  }

  Future<void> setBatteryThresholdEnabled(bool value) async {
    state = state.copyWith(batteryThresholdEnabled: value);
    _scheduleFlush();
  }

  Future<void> setDurationMinutes(int? value) async {
    state = state.copyWith(durationMinutes: value);
    _scheduleFlush();
  }

  Future<void> setClockTime(DateTime? value) async {
    state = state.copyWith(clockTime: value);
    _scheduleFlush();
  }

  Future<void> setAutoStart(bool value) async {
    state = state.copyWith(autoStart: value);
    _scheduleFlush();
  }

  Future<void> setStartMinimized(bool value) async {
    state = state.copyWith(startMinimized: value);
    _scheduleFlush();
  }

  Future<void> setShowCountdownInMenuBar(bool value) async {
    state = state.copyWith(showCountdownInMenuBar: value);
    _scheduleFlush();
  }
}

final appSettingsProvider =
    NotifierProvider<AppSettingsNotifier, AppSettingsState>(
      AppSettingsNotifier.new,
    );
