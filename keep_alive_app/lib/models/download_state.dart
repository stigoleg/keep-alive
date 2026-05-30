enum DownloadStatus { notInstalled, downloading, installed, error }

class DownloadState {
  final DownloadStatus status;
  final double progress;
  final String? installedVersion;
  final String? latestVersion;
  final String? errorMessage;

  const DownloadState({
    this.status = DownloadStatus.notInstalled,
    this.progress = 0.0,
    this.installedVersion,
    this.latestVersion,
    this.errorMessage,
  });

  DownloadState copyWith({
    DownloadStatus? status,
    double? progress,
    String? installedVersion,
    String? latestVersion,
    String? errorMessage,
  }) {
    return DownloadState(
      status: status ?? this.status,
      progress: progress ?? this.progress,
      installedVersion: installedVersion ?? this.installedVersion,
      latestVersion: latestVersion ?? this.latestVersion,
      errorMessage: errorMessage ?? this.errorMessage,
    );
  }

  @override
  String toString() =>
      'DownloadState(status: $status, progress: $progress, installedVersion: $installedVersion)';
}
