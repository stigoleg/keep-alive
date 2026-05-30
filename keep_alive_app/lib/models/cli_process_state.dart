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

  @override
  String toString() =>
      'CliProcessState(status: $status, pid: $pid, exitCode: $exitCode)';
}
