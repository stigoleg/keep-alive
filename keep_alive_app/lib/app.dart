import 'dart:async' show unawaited;
import 'dart:io' show exit;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'core/logger.dart';
import 'providers/cli_binary_provider.dart';
import 'providers/process_provider.dart';
import 'providers/settings_provider.dart';
import 'ui/theme/app_theme.dart';
import 'ui/theme/linux_theme.dart';
import 'ui/theme/macos_theme.dart';
import 'ui/theme/windows_theme.dart';
import 'ui/popup/popup_panel.dart';
import 'ui/tray/tray_manager.dart';
import 'utils/platform_utils.dart';

class KeepAliveApp extends ConsumerStatefulWidget {
  const KeepAliveApp({super.key});

  @override
  ConsumerState<KeepAliveApp> createState() => _KeepAliveAppState();
}

class _KeepAliveAppState extends ConsumerState<KeepAliveApp>
    with WindowListener {
  final TrayManager _trayManager = TrayManager();
  bool _popupVisible = false;
  bool _quitting = false;

  @override
  void initState() {
    super.initState();
    windowManager.addListener(this);
    _initApp();
  }

  @override
  void dispose() {
    windowManager.removeListener(this);
    _trayManager.dispose();
    super.dispose();
  }

  Future<void> _initApp() async {
    try {
      await ref.read(appSettingsProvider.notifier).restoreFromDisk();
    } catch (e) {
      AppLogger.error('Failed to restore settings', e);
    }

    try {
      await _trayManager.initialize(
        onTogglePopup: _togglePopup,
        onQuit: _handleQuit,
      );
    } catch (e) {
      AppLogger.error('Failed to initialize tray, running headless', e);
      return;
    }

    unawaited(ref.read(cliBinaryProvider.notifier).checkAndInstall());

    ref.listenManual(cliProcessProvider, (_, next) {
      _trayManager.setActiveState(next.isRunning);
    });

    final settings = ref.read(appSettingsProvider);
    if (settings.keepAwake) {
      final flags = settings.toCliFlags();
      try {
        unawaited(ref.read(cliProcessProvider.notifier).startSession(flags));
      } catch (e) {
        AppLogger.error('Failed to auto-start session', e);
      }
    }
  }

  void _togglePopup() {
    if (_quitting) return;
    _popupVisible = !_popupVisible;
    _updateWindowVisibility();
  }

  Future<void> _updateWindowVisibility() async {
    if (_popupVisible) {
      await _configurePopupWindow();
    } else {
      await windowManager.hide();
    }
  }

  Future<void> _configurePopupWindow() async {
    final width = _popupWidth;
    await windowManager.setSize(Size(width, 480));
    await windowManager.setResizable(false);
    await windowManager.setMinimizable(false);
    await windowManager.setMaximizable(false);
    await windowManager.setAlwaysOnTop(true);
    await windowManager.setSkipTaskbar(true);
    await windowManager.center();
    await windowManager.show();
    await windowManager.focus();
  }

  double get _popupWidth {
    if (PlatformUtils.isMacOS) return AppTheme.popupWidthMacOS;
    if (PlatformUtils.isWindows) return AppTheme.popupWidthWindows;
    return AppTheme.popupWidthLinux;
  }

  Future<void> _handleQuit() async {
    if (_quitting) return;
    _quitting = true;
    AppLogger.info('Quitting KeepAlive');

    try {
      await ref.read(cliProcessProvider.notifier).stopSession();
    } catch (e) {
      AppLogger.error('Error stopping CLI on quit', e);
    }

    _trayManager.dispose();
    await windowManager.destroy();
    exit(0);
  }

  @override
  void onWindowBlur() {
    if (_popupVisible && !_quitting) {
      _popupVisible = false;
      windowManager.hide();
    }
  }

  @override
  void onWindowClose() {
    _handleQuit();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'KeepAlive',
      debugShowCheckedModeBanner: false,
      themeMode: ThemeMode.system,
      theme: _lightTheme,
      darkTheme: _darkTheme,
      home: const PopupPanel(),
    );
  }
}

ThemeData get _resolveLight {
  if (PlatformUtils.isMacOS) return MacOSTheme.lightTheme;
  if (PlatformUtils.isWindows) return WindowsTheme.lightTheme;
  return LinuxTheme.lightTheme;
}

ThemeData get _resolveDark {
  if (PlatformUtils.isMacOS) return MacOSTheme.darkTheme;
  if (PlatformUtils.isWindows) return WindowsTheme.darkTheme;
  return LinuxTheme.darkTheme;
}

final ThemeData _lightTheme = _resolveLight;
final ThemeData _darkTheme = _resolveDark;
