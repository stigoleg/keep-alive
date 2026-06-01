import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/logger.dart';
import '../models/cli_flags.dart';
import '../platform/platform_interface.dart';
import 'battery_provider.dart';
import 'cli_binary_provider.dart';
import 'process_provider.dart';
import 'settings_provider.dart';

/// Coalescing window for rapid setting-driven CLI restarts (e.g. battery slider
/// drag fires 60 Hz). Picked to feel instant while collapsing into one restart.
const Duration _restartCoalesceWindow = Duration(milliseconds: 350);

final sessionProvider = Provider<SessionOrchestrator>((ref) {
  final orchestrator = SessionOrchestrator(ref);
  ref.onDispose(orchestrator.dispose);
  return orchestrator;
});

class SessionOrchestrator {
  final Ref _ref;
  Timer? _restartDebounce;
  Completer<void>? _pendingCompleter;

  SessionOrchestrator(this._ref);

  void dispose() {
    _restartDebounce?.cancel();
    _restartDebounce = null;
    if (_pendingCompleter?.isCompleted == false) {
      _pendingCompleter?.complete();
    }
    _pendingCompleter = null;
  }

  Future<void> toggleKeepAwake(bool active) async {
    await _ref.read(appSettingsProvider.notifier).setKeepAwake(active);

    if (active) {
      try {
        AppLogger.info(
          'Waiting for CLI binary readiness before starting session',
        );
        await _ref.read(cliBinaryProvider.notifier).waitUntilReady();
      } catch (e) {
        AppLogger.error('CLI binary not ready: $e');
        await _ref.read(appSettingsProvider.notifier).setKeepAwake(false);
        rethrow;
      }

      final settings = await _settingsForCli();
      if (settings.simulateActivity) {
        await _ensureActivityPermission();
      }
      final flags = settings.toCliFlags();
      AppLogger.info('Starting keep-alive session with flags: $flags');
      try {
        await _ref.read(cliProcessProvider.notifier).startSession(flags);
      } catch (e) {
        await _ref.read(appSettingsProvider.notifier).setKeepAwake(false);
        rethrow;
      }
    } else {
      AppLogger.info('Stopping keep-alive session');
      try {
        await _ref.read(cliProcessProvider.notifier).stopSession();
      } catch (e) {
        AppLogger.error('Error stopping session', e);
      }
    }
  }

  Future<void> updateFlags(CliFlags flags) async {
    final keepAwake = _ref.read(appSettingsProvider).keepAwake;
    if (!keepAwake) return;

    final processState = _ref.read(cliProcessProvider);
    if (processState.isRunning) {
      AppLogger.info('Flags changed while running, restarting CLI');
      try {
        await _ref.read(cliProcessProvider.notifier).restartSession(flags);
      } catch (e) {
        AppLogger.error('Failed to restart CLI with new flags', e);
        await _ref.read(appSettingsProvider.notifier).setKeepAwake(false);
        rethrow;
      }
    }
  }

  /// Schedules a CLI restart that reflects the latest settings. Rapid calls
  /// coalesce into a single restart after [_restartCoalesceWindow], so
  /// dragging a slider or flipping a chain of toggles does not churn the CLI.
  /// All callers awaiting concurrently receive the same future, which
  /// completes once the eventual restart finishes.
  Future<void> applySettingsAndRestart() {
    _restartDebounce?.cancel();
    _pendingCompleter ??= Completer<void>();
    final completer = _pendingCompleter!;
    _restartDebounce = Timer(_restartCoalesceWindow, () {
      _restartDebounce = null;
      _pendingCompleter = null;
      _flushRestart().then(
        (_) {
          if (!completer.isCompleted) completer.complete();
        },
        onError: (Object e, StackTrace s) {
          if (!completer.isCompleted) completer.completeError(e, s);
        },
      );
    });
    return completer.future;
  }

  /// Fires any pending debounced restart immediately. Used by tests and by
  /// the quit path so it does not race with an in-flight debounce.
  Future<void> flushPendingRestart() async {
    if (_restartDebounce == null) return;
    _restartDebounce!.cancel();
    _restartDebounce = null;
    final completer = _pendingCompleter;
    _pendingCompleter = null;
    try {
      await _flushRestart();
      if (completer?.isCompleted == false) completer!.complete();
    } catch (e, s) {
      if (completer?.isCompleted == false) completer!.completeError(e, s);
      rethrow;
    }
  }

  Future<void> _flushRestart() async {
    var settings = _ref.read(appSettingsProvider);
    if (!settings.keepAwake) return;

    settings = await _settingsForCli();
    if (settings.simulateActivity) {
      await _ensureActivityPermission();
    }
    final flags = settings.toCliFlags();
    final processState = _ref.read(cliProcessProvider);

    if (processState.isRunning) {
      AppLogger.info('Settings changed, restarting CLI');
      try {
        await _ref.read(cliProcessProvider.notifier).restartSession(flags);
      } catch (e) {
        AppLogger.error('Failed to restart CLI after settings change', e);
      }
    } else {
      try {
        await _ref.read(cliProcessProvider.notifier).startSession(flags);
      } catch (e) {
        AppLogger.error('Failed to start CLI after settings change', e);
        await _ref.read(appSettingsProvider.notifier).setKeepAwake(false);
        rethrow;
      }
    }
  }

  Future<void> _ensureActivityPermission() async {
    final granted = await KeepAlivePlatform.instance
        .ensureActivitySimulationPermission();
    if (!granted) {
      AppLogger.warning(
        'Activity simulation permission not granted before session start; '
        'CLI will fall back to caffeinate -u for chat-app activity.',
      );
    }
  }

  Future<AppSettingsState> _settingsForCli() async {
    final settings = _ref.read(appSettingsProvider);
    if (!settings.batteryThresholdEnabled) return settings;

    final currentBattery = _ref
        .read(batteryStateProvider)
        .valueOrNull
        ?.percentage;
    if (currentBattery == null) return settings;

    final maxThreshold = currentBattery.floor() - 1;
    final notifier = _ref.read(appSettingsProvider.notifier);
    if (maxThreshold < 1) {
      AppLogger.warning(
        'Disabling battery threshold because current battery is too low',
      );
      await notifier.setBatteryThresholdEnabled(false);
      return _ref.read(appSettingsProvider);
    }

    final safeThreshold = (settings.batteryThreshold ?? maxThreshold)
        .clamp(1, maxThreshold)
        .toInt();
    if (settings.batteryThreshold != safeThreshold) {
      AppLogger.info(
        'Clamping battery threshold to $safeThreshold% before starting CLI',
      );
      await notifier.setBatteryThreshold(safeThreshold);
      return _ref.read(appSettingsProvider);
    }

    return settings;
  }
}
