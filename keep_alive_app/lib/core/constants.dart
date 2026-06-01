/// App-wide constants for the KeepAlive menu bar app.
class AppConstants {
  AppConstants._();

  static const String appName = 'KeepAlive';
  static const String appVersion = '1.0.0';
  static const String githubRepo = 'stigoleg/keep-alive';
  static const String githubUrl = 'https://github.com/stigoleg/keep-alive';
  static const String cliBinaryName = 'keepalive';
  static const String cliReleaseBaseName = 'keep-alive';
  static const String githubApiBaseUrl = 'https://api.github.com';
  static const String githubReleasesPath =
      '/repos/stigoleg/keep-alive/releases';
  static const String githubDownloadBaseUrl =
      'https://github.com/stigoleg/keep-alive/releases/download';

  static const String cliVersionArg = '--version';
  static const String cliLogArg = '--log';

  /// Minimum CLI version the GUI accepts from external installs.
  /// v1.5.4 is required because v1.5.3 still depended on the `--headless`
  /// flag (removed in commit 0eb6faf, replaced by stdin-TTY auto-detection);
  /// when launched as a Flutter subprocess v1.5.3 silently exits code 1.
  /// Bumping this back below v1.5.4 is unsafe until package managers also
  /// catch up.
  static const String minimumCliVersion = 'v1.5.4';

  static const Duration batteryPollInterval = Duration(seconds: 30);
  static const Duration updateCheckInterval = Duration(hours: 24);

  static const String platformChannelName =
      'com.stigoleg.keepAliveApp/platform';

  // Platform channel method names
  static const String methodGetPlatformName = 'getPlatformName';
  static const String methodSetAutoStart = 'setAutoStart';
  static const String methodIsAutoStartEnabled = 'isAutoStartEnabled';
  static const String methodSetTrayIcon = 'setTrayIcon';
  static const String methodSetTrayTooltip = 'setTrayTooltip';
  static const String methodSetStatusBarTitle = 'setStatusBarTitle';
  static const String methodShowContextMenu = 'showContextMenu';
  static const String methodShowPopover = 'showPopover';
  static const String methodHidePopover = 'hidePopover';
  static const String methodGetAppSupportDir = 'getAppSupportDir';
  static const String methodGetBatteryInfo = 'getBatteryInfo';
  static const String methodGetBundledCliPath = 'getBundledCliPath';
  static const String methodEnsureActivitySimulationPermission =
      'ensureActivitySimulationPermission';
  static const String methodAssignProcessToJobObject =
      'assignProcessToJobObject';
  static const String methodActivateExistingInstance =
      'activateExistingInstance';
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
  static const int packageManagerInstallTimeoutSeconds = 120;
  static const int settingsFlushDebounceMs = 250;

  static const String downloadUrlCacheFile = '.download_url_cache';
  static const String offlineMode = 'KeepAlive running in offline mode';

  /// Name of the file in app support that records the running CLI's PID,
  /// used by the stale-process sweeper on startup and by force-kill on quit.
  static const String cliPidFile = 'keepalive.pid';

  /// Name of the file in app support that records the Flutter app's PID,
  /// used by the single-instance lock.
  static const String appInstanceLockFile = 'keepalive_app.pid';

  /// Hard ceiling on how long the quit path will wait for a graceful CLI
  /// stop before force-killing via the PID file.
  static const int quitGracefulTimeoutSeconds = 3;

  static const String homebrewTapRepo = 'stigoleg/homebrew-tap';
  static const String homebrewFormula = 'keepalive';

  static const String scoopBucketName = 'stigoleg';
  static const String scoopBucketUrl =
      'https://github.com/stigoleg/scoop-bucket.git';
  static const String scoopPackage = 'keepalive';
}
