import 'dart:ui' show VoidCallback;

import 'package:system_tray/system_tray.dart';

class TrayMenu {
  TrayMenu._();

  static const String _showLabel = 'Show KeepAlive';
  static const String _quitLabel = 'Quit';

  static List<MenuItemBase> buildContextMenu({
    required VoidCallback onShow,
    required VoidCallback onQuit,
  }) {
    return [
      MenuItem(
        label: _showLabel,
        onClicked: onShow,
      ),
      MenuSeparator(),
      MenuItem(
        label: _quitLabel,
        onClicked: onQuit,
      ),
    ];
  }
}
