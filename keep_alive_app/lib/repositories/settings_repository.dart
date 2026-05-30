import 'package:shared_preferences/shared_preferences.dart';

/// Persists user preferences using shared_preferences.
class SettingsRepository {
  static const _keyKeepAwake = 'keepAwake';
  static const _keySimulateActivity = 'simulateActivity';
  static const _keyEnableLogging = 'enableLogging';
  static const _keyBatteryThreshold = 'batteryThreshold';
  static const _keyDurationMinutes = 'durationMinutes';
  static const _keyClockTime = 'clockTime';
  static const _keyAutoStart = 'autoStart';
  static const _keyStartMinimized = 'startMinimized';

  SharedPreferences? _prefs;

  Future<SharedPreferences> get _instance async {
    _prefs ??= await SharedPreferences.getInstance();
    return _prefs!;
  }

  Future<void> setKeepAwake(bool value) async {
    final prefs = await _instance;
    await prefs.setBool(_keyKeepAwake, value);
  }

  Future<bool> getKeepAwake() async {
    final prefs = await _instance;
    return prefs.getBool(_keyKeepAwake) ?? false;
  }

  Future<void> setSimulateActivity(bool value) async {
    final prefs = await _instance;
    await prefs.setBool(_keySimulateActivity, value);
  }

  Future<bool> getSimulateActivity() async {
    final prefs = await _instance;
    return prefs.getBool(_keySimulateActivity) ?? false;
  }

  Future<void> setEnableLogging(bool value) async {
    final prefs = await _instance;
    await prefs.setBool(_keyEnableLogging, value);
  }

  Future<bool> getEnableLogging() async {
    final prefs = await _instance;
    return prefs.getBool(_keyEnableLogging) ?? false;
  }

  Future<void> setBatteryThreshold(int? value) async {
    final prefs = await _instance;
    if (value == null) {
      await prefs.remove(_keyBatteryThreshold);
    } else {
      await prefs.setInt(_keyBatteryThreshold, value);
    }
  }

  Future<int?> getBatteryThreshold() async {
    final prefs = await _instance;
    return prefs.getInt(_keyBatteryThreshold);
  }

  Future<void> setDurationMinutes(int? value) async {
    final prefs = await _instance;
    if (value == null) {
      await prefs.remove(_keyDurationMinutes);
    } else {
      await prefs.setInt(_keyDurationMinutes, value);
    }
  }

  Future<int?> getDurationMinutes() async {
    final prefs = await _instance;
    return prefs.getInt(_keyDurationMinutes);
  }

  Future<void> setClockTime(DateTime? value) async {
    final prefs = await _instance;
    if (value == null) {
      await prefs.remove(_keyClockTime);
    } else {
      await prefs.setString(_keyClockTime, value.toIso8601String());
    }
  }

  Future<DateTime?> getClockTime() async {
    final prefs = await _instance;
    final raw = prefs.getString(_keyClockTime);
    if (raw == null) return null;
    return DateTime.tryParse(raw);
  }

  Future<void> setAutoStart(bool value) async {
    final prefs = await _instance;
    await prefs.setBool(_keyAutoStart, value);
  }

  Future<bool> getAutoStart() async {
    final prefs = await _instance;
    return prefs.getBool(_keyAutoStart) ?? false;
  }

  Future<void> setStartMinimized(bool value) async {
    final prefs = await _instance;
    await prefs.setBool(_keyStartMinimized, value);
  }

  Future<bool> getStartMinimized() async {
    final prefs = await _instance;
    return prefs.getBool(_keyStartMinimized) ?? false;
  }
}
