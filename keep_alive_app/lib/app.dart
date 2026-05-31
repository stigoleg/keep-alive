import 'dart:async';
import 'dart:io' show exit;

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'core/constants.dart';
import 'core/logger.dart';
import 'models/cli_process_state.dart';
import 'platform/platform_interface.dart';
import 'providers/cli_binary_provider.dart';
import 'providers/process_provider.dart';
import 'providers/settings_provider.dart';
import 'ui/theme/linux_theme.dart';
import 'ui/theme/macos_theme.dart';
import 'ui/theme/windows_theme.dart';
import 'ui/popup/popup_panel.dart';
import 'ui/settings/settings_window.dart';
import 'ui/tray/tray_manager.dart';
import 'ui/widgets/error_boundary.dart';
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
  final KeepAlivePlatform _platform = KeepAlivePlatform.instance;
  StreamSubscription<String>? _shutdownSubscription;
  bool _popupVisible = false;
  bool _quitting = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    windowManager.addListener(this);

    _shutdownSubscription = _platform.trayEventStream.listen((event) {
      if (event == 'systemShutdown') {
        AppLogger.info('System shutdown signal received from native platform');
        _handleQuit();
      } else if (event == AppConstants.trayEventPopoverDismissed) {
        _popupVisible = false;
      }
    });

    _initApp();
  }

  @override
  void dispose() {
    _shutdownSubscription?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    windowManager.removeListener(this);
    _trayManager.dispose();
    super.dispose();
  }

  Future<void> _initApp() async {
    AppLogger.info('Initializing app lifecycle');

    try {
      await _configureMainWindow();
    } catch (e) {
      AppLogger.error('Failed to configure main window', e);
    }

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
      _trayManager.setErrorState(next.status == CliProcessStatus.error);
      if (next.status == CliProcessStatus.error) {
        AppLogger.warning('CLI process in error state: ${next.errorMessage}');
      }
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

  Future<void> _configureMainWindow() async {
    await windowManager.setTitle(AppConstants.appName);
    await windowManager.setResizable(false);
    await windowManager.setMinimizable(false);
    await windowManager.setMaximizable(false);
    await windowManager.setSize(const Size(320, 500));
    await windowManager.waitUntilReadyToShow();
    await windowManager.hide();
  }

  Future<void> _togglePopup() async {
    if (_quitting) return;

    if (_popupVisible) {
      _popupVisible = false;
      await _platform.hidePopover();
    } else {
      _popupVisible = true;
      await _platform.showPopover();
    }
  }

  Future<void> _openSettings() async {
    if (_quitting) return;

    if (!_popupVisible) {
      _popupVisible = true;
      await _platform.showPopover();
    }

    final navigator = _navigatorKey.currentState;
    if (navigator != null && mounted) {
      showDialog(
        context: navigator.context,
        barrierDismissible: true,
        builder: (_) => SettingsDialog(
          onClose: () => navigator.pop(),
        ),
      );
    }
  }

  Future<void> _handleQuit() async {
    if (_quitting) return;
    _quitting = true;
    AppLogger.info('Quitting KeepAlive — saving state and cleaning up');

    await _platform.hidePopover();

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

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    AppLogger.debug('App lifecycle state changed: $state');
    if (state == AppLifecycleState.detached) {
      AppLogger.info('App detaching, triggering cleanup');
      _handleQuit();
    }
  }

  @override
  void onWindowClose() {
    AppLogger.info('Window close event received');
    _handleQuit();
  }

  @override
  Widget build(BuildContext context) {
    Widget app = MaterialApp(
      title: AppConstants.appName,
      debugShowCheckedModeBanner: false,
      navigatorKey: _navigatorKey,
      themeMode: ThemeMode.system,
      theme: _lightTheme,
      darkTheme: _darkTheme,
      home: const ErrorBoundary(
        child: PopupPanel(),
      ),
    );

    if (PlatformUtils.isMacOS) {
      app = CupertinoTheme(
        data: CupertinoThemeData(
          brightness: MediaQuery.platformBrightnessOf(context),
          primaryColor: CupertinoColors.systemBlue,
        ),
        child: app,
      );
    }

    return app;
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
