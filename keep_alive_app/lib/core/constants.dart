/// App-wide constants for the KeepAlive menu bar app.
class AppConstants {
  AppConstants._();

  static const String appName = 'KeepAlive';
  static const String appVersion = '1.0.0';
  static const String githubRepo = 'stigoleg/keep-alive';
  static const String cliBinaryName = 'keepalive';
  static const String githubApiBaseUrl = 'https://api.github.com';
  static const String githubReleasesPath = '/repos/stigoleg/keep-alive/releases';
  static const String githubDownloadBaseUrl =
      'https://github.com/stigoleg/keep-alive/releases/download';

  static const String cliVersionArg = '--version';
  static const String cliLogArg = '--log';

  static const Duration batteryPollInterval = Duration(seconds: 30);
  static const Duration updateCheckInterval = Duration(hours: 24);

  static const String platformChannelName = 'com.stigoleg.keepAliveApp/platform';

  // Platform channel method names
  static const String methodGetPlatformName = 'getPlatformName';
  static const String methodSetAutoStart = 'setAutoStart';
  static const String methodIsAutoStartEnabled = 'isAutoStartEnabled';
  static const String methodSetTrayIcon = 'setTrayIcon';
  static const String methodSetTrayTooltip = 'setTrayTooltip';
  static const String methodShowContextMenu = 'showContextMenu';
  static const String methodShowPopover = 'showPopover';
  static const String methodHidePopover = 'hidePopover';
  static const String methodGetAppSupportDir = 'getAppSupportDir';
  static const String methodGetBatteryInfo = 'getBatteryInfo';
  static const String methodOnTrayEvent = 'onTrayEvent';

  // Tray events sent from native to Dart
  static const String trayEventLeftClick = 'leftClick';
  static const String trayEventRightClick = 'rightClick';
  static const String trayEventPopoverDismissed = 'popoverDismissed';

  // Tray icon assets
  static const String trayIconIdle = 'assets/icons/tray_icon.png';
  static const String trayIconActive = 'assets/icons/tray_icon_active.png';
  static const String trayIconError = 'assets/icons/tray_icon_error.png';

  static const int maxLogLines = 1000;
  static const int processGracefulTimeoutSeconds = 5;

  static const int downloadMaxRetries = 3;
  static const int downloadRetryBaseDelayMs = 1000;

  static const String downloadUrlCacheFile = '.download_url_cache';
  static const String offlineMode = 'KeepAlive running in offline mode';
}
