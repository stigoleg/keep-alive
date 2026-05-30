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

class PlatformException extends AppException {
  const PlatformException(super.message, {super.underlying});
}
