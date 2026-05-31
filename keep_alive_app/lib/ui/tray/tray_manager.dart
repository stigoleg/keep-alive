import 'dart:async';
import 'dart:ui' show VoidCallback;

import '../../core/constants.dart';
import '../../core/logger.dart';
import '../../platform/platform_interface.dart';
import 'tray_menu.dart';

class TrayManager {
  final KeepAlivePlatform _platform = KeepAlivePlatform.instance;
  StreamSubscription<String>? _eventSubscription;
  bool _initialized = false;
  bool _isActive = false;
  bool _isError = false;

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
      AppLogger.info('Setting tray icon: $_idleIcon');
      await _platform.setTrayIcon(_idleIcon);
      AppLogger.info('Tray icon set, setting tooltip');
      await _platform.setTrayTooltip(_idleTooltip);
      AppLogger.info('Tray tooltip set, subscribing to events');

      _eventSubscription = _platform.trayEventStream.listen(_handleTrayEvent);

      _initialized = true;
      AppLogger.info('System tray initialized via platform channel');
    } catch (e, stack) {
      AppLogger.error('Failed to initialize system tray: $e', e, stack);
      rethrow;
    }
  }

  void setActiveState(bool isActive) {
    if (!_initialized || _isActive == isActive) return;
    _isActive = isActive;

    final icon = _resolveIcon();
    final tooltip = _resolveTooltip();

    _platform.setTrayIcon(icon);
    _platform.setTrayTooltip(tooltip);
  }

  void setErrorState(bool isError) {
    if (!_initialized || _isError == isError) return;
    _isError = isError;

    final icon = _resolveIcon();
    final tooltip = _resolveTooltip();

    try {
      _platform.setTrayIcon(icon);
      _platform.setTrayTooltip(tooltip);
    } catch (e) {
      AppLogger.warning('Failed to set error tray icon, falling back: $e');
      _platform.setTrayIcon(_idleIcon);
      _platform.setTrayTooltip(tooltip);
    }
  }

  void updateTooltip(String text) {
    if (!_initialized) return;
    _platform.setTrayTooltip(text);
  }

  void dispose() {
    _eventSubscription?.cancel();
    _eventSubscription = null;
  }

  static const String _idleIcon = AppConstants.trayIconIdle;
  static const String _activeIcon = AppConstants.trayIconActive;
  static const String _errorIcon = AppConstants.trayIconError;
  static const String _idleTooltip = 'KeepAlive \u2014 Idle';
  static const String _activeTooltip = 'KeepAlive \u2014 System Active';
  static const String _errorTooltip = 'KeepAlive \u2014 Error';

  String _resolveIcon() {
    if (_isError) return _errorIcon;
    if (_isActive) return _activeIcon;
    return _idleIcon;
  }

  String _resolveTooltip() {
    if (_isError) return _errorTooltip;
    if (_isActive) return _activeTooltip;
    return _idleTooltip;
  }

  Future<void> _handleTrayEvent(String eventName) async {
    AppLogger.debug('System tray event: $eventName');

    switch (eventName) {
      case AppConstants.trayEventLeftClick:
        onTogglePopup?.call();
      case AppConstants.trayEventRightClick:
        final selectedIndex = await _platform.showContextMenu(
          TrayMenu.menuLabels(),
        );
        if (selectedIndex != null) {
          _handleContextMenuSelection(selectedIndex);
        }
      case AppConstants.trayEventPopoverDismissed:
        break;
    }
  }

  void _handleContextMenuSelection(int index) {
    switch (index) {
      case 0:
        onTogglePopup?.call();
      case 1:
        onOpenSettings?.call();
      case 2:
        onQuit?.call();
    }
  }
}
