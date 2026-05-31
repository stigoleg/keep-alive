import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/logger.dart';
import '../models/cli_flags.dart';
import 'battery_provider.dart';
import 'cli_binary_provider.dart';
import '../platform/platform_interface.dart';
import 'process_provider.dart';
import 'settings_provider.dart';

final sessionProvider = Provider<SessionOrchestrator>((ref) {
  return SessionOrchestrator(ref);
});

class SessionOrchestrator {
  final Ref _ref;

  SessionOrchestrator(this._ref);

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

  Future<void> applySettingsAndRestart() async {
    var settings = _ref.read(appSettingsProvider);
    if (!settings.keepAwake) return;

    settings = await _settingsForCli();
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

  Future<AppSettingsState> _settingsForCli() async {
    var settings = _ref.read(appSettingsProvider);
    if (settings.simulateActivity) {
      final allowed = await KeepAlivePlatform.instance
          .ensureActivitySimulationPermission();
      if (!allowed) {
        AppLogger.warning(
          'Disabling activity simulation because Accessibility permission is missing',
        );
        await _ref
            .read(appSettingsProvider.notifier)
            .setSimulateActivity(false);
        settings = _ref.read(appSettingsProvider);
      }
    }

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
