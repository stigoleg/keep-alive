/// Formatting utilities for durations, battery percentages, etc.
class FormatUtils {
  FormatUtils._();

  static String duration(int totalMinutes) {
    if (totalMinutes <= 0) return '0m';
    final hours = totalMinutes ~/ 60;
    final minutes = totalMinutes % 60;
    if (hours == 0) return '${minutes}m';
    if (minutes == 0) return '${hours}h';
    return '${hours}h ${minutes}m';
  }

  static String remainingTime(DateTime startTime, int durationMinutes) {
    final endTime = startTime.add(Duration(minutes: durationMinutes));
    final remaining = endTime.difference(DateTime.now());
    if (remaining.isNegative) return '0m remaining';
    return '${duration(remaining.inMinutes)} remaining';
  }

  static String battery(double percentage) {
    return '${percentage.round()}%';
  }

  static String timeOfDay(DateTime dt) {
    final hour = dt.hour;
    final minute = dt.minute.toString().padLeft(2, '0');
    final period = hour >= 12 ? 'PM' : 'AM';
    final displayHour = hour == 0 ? 12 : (hour > 12 ? hour - 12 : hour);
    return '$displayHour:$minute $period';
  }

  static String timeOfDay24(DateTime dt) {
    final hour = dt.hour.toString().padLeft(2, '0');
    final minute = dt.minute.toString().padLeft(2, '0');
    return '$hour:$minute';
  }
}
