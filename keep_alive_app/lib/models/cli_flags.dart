class CliFlags {
  final int? durationMinutes;
  final DateTime? clockTime;
  final int? batteryThreshold;
  final bool simulateActivity;
  final bool enableLogging;

  const CliFlags({
    this.durationMinutes,
    this.clockTime,
    this.batteryThreshold,
    this.simulateActivity = false,
    this.enableLogging = false,
  });

  List<String> toArgs() {
    // TODO: implement flag conversion
    return [];
  }

  CliFlags copyWith({
    int? durationMinutes,
    DateTime? clockTime,
    int? batteryThreshold,
    bool? simulateActivity,
    bool? enableLogging,
  }) {
    return CliFlags(
      durationMinutes: durationMinutes ?? this.durationMinutes,
      clockTime: clockTime ?? this.clockTime,
      batteryThreshold: batteryThreshold ?? this.batteryThreshold,
      simulateActivity: simulateActivity ?? this.simulateActivity,
      enableLogging: enableLogging ?? this.enableLogging,
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
