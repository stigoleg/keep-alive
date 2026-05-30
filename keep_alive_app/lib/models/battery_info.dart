class BatteryInfo {
  final double percentage;
  final bool isCharging;
  final bool isPresent;

  const BatteryInfo({
    required this.percentage,
    this.isCharging = false,
    this.isPresent = true,
  });

  BatteryInfo copyWith({
    double? percentage,
    bool? isCharging,
    bool? isPresent,
  }) {
    return BatteryInfo(
      percentage: percentage ?? this.percentage,
      isCharging: isCharging ?? this.isCharging,
      isPresent: isPresent ?? this.isPresent,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is BatteryInfo &&
          percentage == other.percentage &&
          isCharging == other.isCharging &&
          isPresent == other.isPresent;

  @override
  int get hashCode => percentage.hashCode ^ isCharging.hashCode ^ isPresent.hashCode;

  @override
  String toString() =>
      'BatteryInfo(percentage: $percentage, isCharging: $isCharging, isPresent: $isPresent)';
}
