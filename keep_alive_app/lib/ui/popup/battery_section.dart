import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/battery_provider.dart';
import '../../providers/session_provider.dart';
import '../../providers/settings_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/battery_slider.dart';
import '../widgets/toggle_switch.dart';

class BatterySection extends ConsumerWidget {
  const BatterySection({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(appSettingsProvider);
    final batteryAsync = ref.watch(batteryStateProvider);

    final currentBattery = batteryAsync.valueOrNull?.percentage ?? 100.0;
    final maxThreshold = _maxThresholdFor(currentBattery);
    final threshold = _clampThreshold(settings.batteryThreshold, maxThreshold);

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing4,
        vertical: AppTheme.spacing4,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ToggleSwitch(
            label: 'Stop when battery drops to',
            value: settings.batteryThresholdEnabled,
            onChanged: (value) async {
              final notifier = ref.read(appSettingsProvider.notifier);
              if (value) {
                await notifier.setBatteryThreshold(threshold);
              }
              await notifier.setBatteryThresholdEnabled(value);
              ref.read(sessionProvider).applySettingsAndRestart();
            },
          ),
          if (settings.batteryThresholdEnabled)
            Padding(
              padding: const EdgeInsets.only(
                left: AppTheme.spacing8,
                right: AppTheme.spacing8,
              ),
              child: BatterySlider(
                value: threshold,
                maxValue: maxThreshold,
                label: 'Threshold:',
                onChanged: (value) async {
                  final safeValue = _clampThreshold(value, maxThreshold);
                  await ref
                      .read(appSettingsProvider.notifier)
                      .setBatteryThreshold(safeValue);
                  ref.read(sessionProvider).applySettingsAndRestart();
                },
              ),
            ),
        ],
      ),
    );
  }

  int _maxThresholdFor(double currentBattery) {
    return (currentBattery.floor() - 1).clamp(1, 99).toInt();
  }

  int _clampThreshold(int? value, int maxThreshold) {
    return (value ?? maxThreshold).clamp(1, maxThreshold).toInt();
  }
}
