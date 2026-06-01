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

  bool get isDownloading => status == DownloadStatus.downloading;

  DownloadState copyWith({
    DownloadStatus? status,
    double? progress,
    String? installedVersion,
    String? latestVersion,
    Object? errorMessage = _keepErrorMessage,
  }) {
    return DownloadState(
      status: status ?? this.status,
      progress: progress ?? this.progress,
      installedVersion: installedVersion ?? this.installedVersion,
      latestVersion: latestVersion ?? this.latestVersion,
      errorMessage: identical(errorMessage, _keepErrorMessage)
          ? this.errorMessage
          : errorMessage as String?,
    );
  }

  static const _keepErrorMessage = Object();

  Map<String, dynamic> toJson() => {
    'status': status.name,
    'progress': progress,
    'installedVersion': installedVersion,
    'latestVersion': latestVersion,
    'errorMessage': errorMessage,
  };

  factory DownloadState.fromJson(Map<String, dynamic> json) {
    return DownloadState(
      status: DownloadStatus.values.firstWhere(
        (s) => s.name == json['status'],
        orElse: () => DownloadStatus.notInstalled,
      ),
      progress: (json['progress'] as num?)?.toDouble() ?? 0.0,
      installedVersion: json['installedVersion'] as String?,
      latestVersion: json['latestVersion'] as String?,
      errorMessage: json['errorMessage'] as String?,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is DownloadState &&
          status == other.status &&
          progress == other.progress &&
          installedVersion == other.installedVersion &&
          latestVersion == other.latestVersion &&
          errorMessage == other.errorMessage;

  @override
  int get hashCode =>
      status.hashCode ^
      progress.hashCode ^
      installedVersion.hashCode ^
      latestVersion.hashCode ^
      errorMessage.hashCode;

  @override
  String toString() =>
      'DownloadState(status: $status, progress: $progress, installedVersion: $installedVersion)';
}
