class TrayMenu {
  TrayMenu._();

  static const String showLabel = 'Show KeepAlive';
  static const String settingsLabel = 'Preferences\u2026';
  static const String quitLabel = 'Quit';
  static const String separator = '-';

  static List<String> menuLabels() {
    return [
      showLabel,
      settingsLabel,
      separator,
      quitLabel,
    ];
  }
}
