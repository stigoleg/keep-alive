/// App-wide constants for the KeepAlive menu bar app.
class AppConstants {
  AppConstants._();

  static const String appName = 'KeepAlive';
  static const String githubRepo = 'stigoleg/keep-alive';
  static const String cliBinaryName = 'keepalive';
  static const String githubApiBaseUrl = 'https://api.github.com';
  static const String githubReleasesPath = '/repos/stigoleg/keep-alive/releases';

  static const Duration batteryPollInterval = Duration(seconds: 30);
  static const Duration updateCheckInterval = Duration(hours: 24);

  static const int maxLogLines = 1000;
  static const int processGracefulTimeoutSeconds = 5;
}
