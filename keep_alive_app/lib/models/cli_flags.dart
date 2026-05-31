class CliFlags {
  final int? durationMinutes;
  final DateTime? clockTime;
  final int? batteryThreshold;
  final bool simulateActivity;
  final bool enableLogging;

  static const _clearDuration = Object();
  static const _clearClockTime = Object();
  static const _clearBatteryThreshold = Object();

  const CliFlags({
    this.durationMinutes,
    this.clockTime,
    this.batteryThreshold,
    this.simulateActivity = false,
    this.enableLogging = false,
  });

  List<String> toArgs() {
    final args = <String>[];
    if (durationMinutes != null) {
      args.addAll(['--duration', durationMinutes.toString()]);
    }
    if (clockTime != null) {
      final hour = clockTime!.hour.toString().padLeft(2, '0');
      final minute = clockTime!.minute.toString().padLeft(2, '0');
      args.addAll(['--clock', '$hour:$minute']);
    }
    if (batteryThreshold != null) {
      args.addAll(['--battery', batteryThreshold.toString()]);
    }
    if (simulateActivity) {
      args.add('--active');
    }
    if (enableLogging) {
      args.add('--log');
    }
    return args;
  }

  CliFlags copyWith({
    Object? durationMinutes = _clearDuration,
    Object? clockTime = _clearClockTime,
    Object? batteryThreshold = _clearBatteryThreshold,
    bool? simulateActivity,
    bool? enableLogging,
  }) {
    return CliFlags(
      durationMinutes:
          identical(durationMinutes, _clearDuration) ? this.durationMinutes : durationMinutes as int?,
      clockTime:
          identical(clockTime, _clearClockTime) ? this.clockTime : clockTime as DateTime?,
      batteryThreshold:
          identical(batteryThreshold, _clearBatteryThreshold)
              ? this.batteryThreshold
              : batteryThreshold as int?,
      simulateActivity: simulateActivity ?? this.simulateActivity,
      enableLogging: enableLogging ?? this.enableLogging,
    );
  }

  Map<String, dynamic> toJson() => {
        'durationMinutes': durationMinutes,
        'clockTime': clockTime?.toIso8601String(),
        'batteryThreshold': batteryThreshold,
        'simulateActivity': simulateActivity,
        'enableLogging': enableLogging,
      };

  factory CliFlags.fromJson(Map<String, dynamic> json) {
    final clockRaw = json['clockTime'];
    return CliFlags(
      durationMinutes: json['durationMinutes'] as int?,
      clockTime: clockRaw != null ? DateTime.parse(clockRaw as String) : null,
      batteryThreshold: json['batteryThreshold'] as int?,
      simulateActivity: json['simulateActivity'] as bool? ?? false,
      enableLogging: json['enableLogging'] as bool? ?? false,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is CliFlags &&
          durationMinutes == other.durationMinutes &&
          clockTime == other.clockTime &&
          batteryThreshold == other.batteryThreshold &&
          simulateActivity == other.simulateActivity &&
          enableLogging == other.enableLogging;

  @override
  int get hashCode =>
      durationMinutes.hashCode ^
      clockTime.hashCode ^
      batteryThreshold.hashCode ^
      simulateActivity.hashCode ^
      enableLogging.hashCode;

  @override
  String toString() =>
      'CliFlags(durationMinutes: $durationMinutes, clockTime: $clockTime, batteryThreshold: $batteryThreshold, simulateActivity: $simulateActivity, enableLogging: $enableLogging)';
}
