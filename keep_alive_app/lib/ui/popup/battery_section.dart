import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/battery_provider.dart';
import '../../providers/session_provider.dart';
import '../../providers/settings_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/battery_slider.dart';

class BatterySection extends ConsumerWidget {
  const BatterySection({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(appSettingsProvider);
    final batteryAsync = ref.watch(batteryStateProvider);

    final currentBattery = batteryAsync.valueOrNull?.percentage ?? 100.0;
    final threshold = settings.batteryThreshold;
    final belowThreshold = threshold != null && currentBattery < threshold;

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing12,
        vertical: AppTheme.spacing4,
      ),
      child: BatterySlider(
        value: threshold ?? 50,
        label: 'Stop when battery drops to',
        disabled: belowThreshold,
        onChanged: (value) {
          ref.read(appSettingsProvider.notifier).setBatteryThreshold(value);
          ref.read(sessionProvider).applySettingsAndRestart();
        },
      ),
    );
  }
}
