import 'dart:async';
import 'dart:io' show exit;

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'core/constants.dart';
import 'core/logger.dart';
import 'models/cli_process_state.dart';
import 'models/download_state.dart';
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
  static const double _settingsPopupMinHeight = 480;

  final TrayManager _trayManager = TrayManager();
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();
  final KeepAlivePlatform _platform = KeepAlivePlatform.instance;
  StreamSubscription<String>? _shutdownSubscription;
  Timer? _menuBarCountdownTimer;
  bool _popupVisible = false;
  bool _settingsOpen = false;
  int _windowResizeRevision = 0;
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
        _handlePopoverDismissed();
      }
    });

    _initApp();
  }

  @override
  void dispose() {
    _stopMenuBarCountdown();
    _shutdownSubscription?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    windowManager.removeListener(this);
    _trayManager.dispose();
    super.dispose();
  }

  void _stopMenuBarCountdown() {
    _menuBarCountdownTimer?.cancel();
    _menuBarCountdownTimer = null;
    _platform.setStatusBarTitle('');
  }

  void _ensureMenuBarCountdownRunning() {
    if (_menuBarCountdownTimer != null) return;
    _menuBarCountdownTimer = Timer.periodic(const Duration(seconds: 1), (_) {
      _updateMenuBarCountdown();
    });
    _updateMenuBarCountdown();
  }

  void _syncMenuBarCountdown() {
    if (_quitting) {
      _stopMenuBarCountdown();
      return;
    }
    final settings = ref.read(appSettingsProvider);
    final processState = ref.read(cliProcessProvider);
    final shouldRun =
        settings.showCountdownInMenuBar &&
        processState.isRunning &&
        settings.durationMinutes != null &&
        processState.startTime != null;

    if (shouldRun) {
      _ensureMenuBarCountdownRunning();
    } else {
      _stopMenuBarCountdown();
    }
  }

  void _updateMenuBarCountdown() {
    if (_quitting) return;

    final settings = ref.read(appSettingsProvider);
    if (!settings.showCountdownInMenuBar) {
      _platform.setStatusBarTitle('');
      return;
    }

    final processState = ref.read(cliProcessProvider);
    if (!processState.isRunning ||
        settings.durationMinutes == null ||
        processState.startTime == null) {
      _platform.setStatusBarTitle('');
      return;
    }

    final endTime = processState.startTime!.add(
      Duration(minutes: settings.durationMinutes!),
    );
    final remaining = endTime.difference(DateTime.now());
    if (remaining.isNegative || remaining.inMinutes <= 0) {
      _platform.setStatusBarTitle('Done');
      return;
    }

    final text = _formatRemainingShort(remaining.inMinutes);
    _platform.setStatusBarTitle(text);
  }

  String _formatRemainingShort(int totalMinutes) {
    final hours = totalMinutes ~/ 60;
    final minutes = totalMinutes % 60;
    if (hours > 0 && minutes > 0) return '${hours}h${minutes}m';
    if (hours > 0) return '${hours}h';
    return '${minutes}m';
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
      AppLogger.info('Checking CLI binary availability');
      await ref.read(cliBinaryProvider.notifier).checkAndInstall();
      AppLogger.info('CLI binary check complete');
    } catch (e) {
      AppLogger.error('Failed to install CLI binary', e);
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

    ref.listenManual(cliProcessProvider, (_, next) {
      final isError = next.status == CliProcessStatus.error;
      _trayManager.updateTrayState(next.isRunning, isError);
      if (isError) {
        AppLogger.warning('CLI process in error state: ${next.errorMessage}');
      }
      _syncMenuBarCountdown();
    });

    ref.listenManual(appSettingsProvider, (_, __) {
      _syncMenuBarCountdown();
    });

    final settings = ref.read(appSettingsProvider);
    if (settings.autoStart && settings.keepAwake) {
      AppLogger.info('Auto-starting keep-alive session from previous state');
      final flags = settings.toCliFlags();
      try {
        final binaryReady =
            ref.read(cliBinaryProvider).status == DownloadStatus.installed;
        if (binaryReady) {
          unawaited(ref.read(cliProcessProvider.notifier).startSession(flags));
        } else {
          AppLogger.warning('Skipping auto-start: CLI binary not installed');
        }
      } catch (e) {
        AppLogger.error('Failed to auto-start session', e);
      }
    }

    _syncMenuBarCountdown();

    AppLogger.info('App initialization complete');
  }

  Future<void> _configureMainWindow() async {
    await windowManager.setTitle(AppConstants.appName);
    await windowManager.setSize(const Size(320, 300));
    await windowManager.waitUntilReadyToShow();
    await windowManager.hide();
    AppLogger.info('Main window configured and hidden');
  }

  Future<void> _togglePopup() async {
    if (_quitting) return;

    if (_popupVisible) {
      await _hidePopup();
    } else {
      _popupVisible = true;
      await _platform.showPopover();
    }
  }

  Future<void> _openSettings() async {
    if (_quitting) return;
    if (_settingsOpen) return;

    if (!_popupVisible) {
      _popupVisible = true;
      unawaited(_platform.showPopover());
    }

    void showSettingsDialog() {
      final navigator = _navigatorKey.currentState;
      if (navigator != null && mounted) {
        _setSettingsOpen(true);
        unawaited(
          showDialog(
            context: navigator.context,
            barrierDismissible: true,
            builder: (_) => SettingsDialog(onClose: () => navigator.pop()),
          ).whenComplete(() => _setSettingsOpen(false)),
        );
      }
    }

    if (_popupVisible) {
      showSettingsDialog();
    } else {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        showSettingsDialog();
      });
    }
  }

  Future<void> _hidePopup() async {
    _popupVisible = false;
    _closeSettingsDialog();
    await _platform.hidePopover();
  }

  void _handlePopoverDismissed() {
    _popupVisible = false;
    _closeSettingsDialog();
  }

  void _closeSettingsDialog() {
    if (!_settingsOpen) return;
    _setSettingsOpen(false);
    final navigator = _navigatorKey.currentState;
    if (navigator != null) {
      navigator.popUntil((route) => route.isFirst);
    }
  }

  void _setSettingsOpen(bool isOpen) {
    if (_settingsOpen == isOpen) return;
    if (!mounted) {
      _settingsOpen = isOpen;
      _windowResizeRevision++;
      return;
    }
    setState(() {
      _settingsOpen = isOpen;
      _windowResizeRevision++;
    });
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
  void onWindowBlur() {
    if (!_popupVisible || _quitting) return;
    unawaited(_hidePopup());
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
      home: ErrorBoundary(
        child: PopupPanel(
          onOpenSettings: _openSettings,
          minWindowHeight: _settingsOpen ? _settingsPopupMinHeight : null,
          resizeRevision: _windowResizeRevision,
        ),
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
