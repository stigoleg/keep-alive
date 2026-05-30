import 'dart:async' show unawaited;
import 'dart:io' show exit;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'core/constants.dart';
import 'core/logger.dart';
import 'providers/cli_binary_provider.dart';
import 'providers/process_provider.dart';
import 'providers/settings_provider.dart';
import 'ui/theme/app_theme.dart';
import 'ui/theme/linux_theme.dart';
import 'ui/theme/macos_theme.dart';
import 'ui/theme/windows_theme.dart';
import 'ui/popup/popup_panel.dart';
import 'ui/settings/settings_window.dart';
import 'ui/tray/tray_manager.dart';
import 'utils/platform_utils.dart';

class KeepAliveApp extends ConsumerStatefulWidget {
  const KeepAliveApp({super.key});

  @override
  ConsumerState<KeepAliveApp> createState() => _KeepAliveAppState();
}

class _KeepAliveAppState extends ConsumerState<KeepAliveApp>
    with WidgetsBindingObserver, WindowListener {
  final TrayManager _trayManager = TrayManager();
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();
  late final MethodChannel _platformChannel;
  bool _popupVisible = false;
  bool _quitting = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    windowManager.addListener(this);

    _platformChannel = const MethodChannel(AppConstants.platformChannelName);
    _platformChannel.setMethodCallHandler(_handlePlatformMethodCall);

    _initApp();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    windowManager.removeListener(this);
    _platformChannel.setMethodCallHandler(null);
    _trayManager.dispose();
    super.dispose();
  }

  Future<void> _initApp() async {
    AppLogger.info('Initializing app lifecycle');

    try {
      await ref.read(appSettingsProvider.notifier).restoreFromDisk();
      AppLogger.info('Settings restored from disk');
    } catch (e) {
      AppLogger.error('Failed to restore settings', e);
    }

    try {
      await _trayManager.initialize(
        onTogglePopup: _togglePopup,
        onOpenSettings: _openSettings,
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
    if (settings.autoStart && settings.keepAwake) {
      AppLogger.info('Auto-starting keep-alive session from previous state');
      final flags = settings.toCliFlags();
      try {
        unawaited(ref.read(cliProcessProvider.notifier).startSession(flags));
      } catch (e) {
        AppLogger.error('Failed to auto-start session', e);
      }
    }

    AppLogger.info('App initialization complete');
  }

  void _togglePopup() {
    if (_quitting) return;
    _popupVisible = !_popupVisible;
    _updateWindowVisibility();
  }

  Future<void> _openSettings() async {
    if (_quitting) return;
    final needsShow = !_popupVisible;
    if (needsShow) {
      _popupVisible = true;
      await _updateWindowVisibility();
    }
    final navigator = _navigatorKey.currentState;
    if (navigator != null && mounted) {
      // ignore: use_build_context_synchronously
      showDialog(
        context: navigator.context,
        barrierDismissible: true,
        builder: (_) => SettingsDialog(
          onClose: () => navigator.pop(),
        ),
      );
    }
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
    await windowManager.setTitle(AppConstants.appName);
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
    AppLogger.info('Quitting KeepAlive — saving state and cleaning up');

    try {
      await ref.read(appSettingsProvider.notifier).saveToDisk();
      AppLogger.info('Settings saved');
    } catch (e) {
      AppLogger.error('Failed to save settings on quit', e);
    }

    try {
      await ref.read(cliProcessProvider.notifier).stopSession();
      AppLogger.info('CLI process stopped');
    } catch (e) {
      AppLogger.error('Error stopping CLI on quit', e);
    }

    _trayManager.dispose();

    try {
      await windowManager.destroy();
    } catch (e) {
      AppLogger.error('Error destroying window on quit', e);
    }

    exit(0);
  }

  Future<dynamic> _handlePlatformMethodCall(MethodCall call) async {
    if (call.method == 'systemShutdown') {
      AppLogger.info('System shutdown signal received from native platform');
      await _handleQuit();
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    AppLogger.debug('App lifecycle state changed: $state');
    if (state == AppLifecycleState.detached) {
      AppLogger.info('App detaching, triggering cleanup');
      _handleQuit();
    }
  }

  @override
  void onWindowBlur() {
    if (_popupVisible && !_quitting) {
      _popupVisible = false;
      unawaited(windowManager.hide());
    }
  }

  @override
  void onWindowClose() {
    AppLogger.info('Window close event received');
    _handleQuit();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: AppConstants.appName,
      debugShowCheckedModeBanner: false,
      navigatorKey: _navigatorKey,
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
