enum CliProcessStatus { idle, starting, running, stopping, error }

class CliProcessState {
  final CliProcessStatus status;
  final int? pid;
  final DateTime? startTime;
  final int? exitCode;
  final String? errorMessage;

  const CliProcessState({
    this.status = CliProcessStatus.idle,
    this.pid,
    this.startTime,
    this.exitCode,
    this.errorMessage,
  });

  bool get isRunning => status == CliProcessStatus.running;

  CliProcessState copyWith({
    CliProcessStatus? status,
    int? pid,
    DateTime? startTime,
    int? exitCode,
    String? errorMessage,
  }) {
    return CliProcessState(
      status: status ?? this.status,
      pid: pid ?? this.pid,
      startTime: startTime ?? this.startTime,
      exitCode: exitCode ?? this.exitCode,
      errorMessage: errorMessage ?? this.errorMessage,
    );
  }

  Map<String, dynamic> toJson() => {
        'status': status.name,
        'pid': pid,
        'startTime': startTime?.toIso8601String(),
        'exitCode': exitCode,
        'errorMessage': errorMessage,
      };

  factory CliProcessState.fromJson(Map<String, dynamic> json) {
    return CliProcessState(
      status: CliProcessStatus.values.firstWhere(
        (s) => s.name == json['status'],
        orElse: () => CliProcessStatus.idle,
      ),
      pid: json['pid'] as int?,
      startTime:
          json['startTime'] != null ? DateTime.parse(json['startTime'] as String) : null,
      exitCode: json['exitCode'] as int?,
      errorMessage: json['errorMessage'] as String?,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is CliProcessState &&
          status == other.status &&
          pid == other.pid &&
          startTime == other.startTime &&
          exitCode == other.exitCode &&
          errorMessage == other.errorMessage;

  @override
  int get hashCode =>
      status.hashCode ^ pid.hashCode ^ startTime.hashCode ^ exitCode.hashCode ^ errorMessage.hashCode;

  @override
  String toString() =>
      'CliProcessState(status: $status, pid: $pid, exitCode: $exitCode)';
}
