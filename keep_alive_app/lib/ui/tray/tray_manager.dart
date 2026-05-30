import 'package:flutter/services.dart';
import 'package:system_tray/system_tray.dart';

import '../../core/constants.dart';
import '../../core/logger.dart';
import 'tray_menu.dart';

class TrayManager {
  final SystemTray _systemTray = SystemTray();
  bool _initialized = false;
  bool _isActive = false;

  VoidCallback? onTogglePopup;
  VoidCallback? onOpenSettings;
  VoidCallback? onQuit;

  Future<void> initialize({
    required VoidCallback onTogglePopup,
    required VoidCallback onOpenSettings,
    required VoidCallback onQuit,
  }) async {
    this.onTogglePopup = onTogglePopup;
    this.onOpenSettings = onOpenSettings;
    this.onQuit = onQuit;

    try {
      await _systemTray.initSystemTray(
        title: AppConstants.appName,
        iconPath: _idleIcon,
        toolTip: _idleTooltip,
      );

      _systemTray.registerSystemTrayEventHandler(_handleSystemTrayEvent);

      await _buildContextMenu();
      _initialized = true;
      AppLogger.info('System tray initialized');
    } catch (e) {
      AppLogger.error('Failed to initialize system tray', e);
      rethrow;
    }
  }

  void setActiveState(bool isActive) {
    if (!_initialized || _isActive == isActive) return;
    _isActive = isActive;

    final icon = isActive ? _activeIcon : _idleIcon;
    final tooltip = isActive ? _activeTooltip : _idleTooltip;

    _systemTray.setImage(icon);
    _systemTray.setToolTip(tooltip);
  }

  void updateTooltip(String text) {
    if (!_initialized) return;
    _systemTray.setToolTip(text);
  }

  void dispose() {}

  static const String _idleIcon = 'assets/icons/tray_icon.png';
  static const String _activeIcon = 'assets/icons/tray_icon_active.png';
  static const String _idleTooltip = 'KeepAlive \u2014 Idle';
  static const String _activeTooltip = 'KeepAlive \u2014 System Active';

  Future<void> _buildContextMenu() async {
    try {
      await _systemTray.setContextMenu(
        TrayMenu.buildContextMenu(
          onShow: () => onTogglePopup?.call(),
          onSettings: () => onOpenSettings?.call(),
          onQuit: () => onQuit?.call(),
        ),
      );
    } on PlatformException catch (e) {
      AppLogger.error('Failed to set context menu', e);
    }
  }

  void _handleSystemTrayEvent(String eventName) {
    AppLogger.debug('System tray event: $eventName');
    if (eventName == 'leftMouseUp' || eventName == 'LeftMouseUp') {
      onTogglePopup?.call();
    }
  }
}
