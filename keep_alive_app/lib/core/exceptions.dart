sealed class AppException implements Exception {
  final String message;
  final Object? underlying;

  const AppException(this.message, {this.underlying});

  @override
  String toString() => '$runtimeType: $message';
}

class CliBinaryException extends AppException {
  const CliBinaryException(super.message, {super.underlying});
}

class CliProcessException extends AppException {
  const CliProcessException(super.message, {super.underlying});
}

class DownloadException extends AppException {
  const DownloadException(super.message, {super.underlying});
}

/// Raised by the download/update flow when the active CLI is already at the
/// latest available version. Intentionally NOT a [DownloadException] so the
/// provider can route it as an info-level signal (banner + INFO log) rather
/// than the loud SEVERE error treatment a real download failure deserves.
class AlreadyUpToDateException extends AppException {
  const AlreadyUpToDateException(super.message, {super.underlying});
}

class PlatformException extends AppException {
  const PlatformException(super.message, {super.underlying});
}
