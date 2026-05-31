import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/download_state.dart';
import '../../core/logger.dart';
import '../../platform/platform_interface.dart';
import '../../providers/cli_binary_provider.dart';
import '../../providers/session_provider.dart';
import '../../providers/settings_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/toggle_switch.dart';
import 'battery_section.dart';
import 'timer_section.dart';

class ToggleSection extends ConsumerWidget {
  const ToggleSection({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(appSettingsProvider);
    final binaryState = ref.watch(cliBinaryProvider);

    final cliInstalled = binaryState.status == DownloadStatus.installed;

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing12,
        vertical: AppTheme.spacing4,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          ToggleSwitch(
            label: 'Keep System Awake',
            description: 'Prevent system sleep and screen lock',
            value: settings.keepAwake,
            enabled: cliInstalled,
            tooltip: cliInstalled ? null : 'CLI binary not installed',
            onChanged: (value) {
              ref.read(sessionProvider).toggleKeepAwake(value);
            },
          ),
          if (settings.keepAwake) ...[
            _Separator(),
            const TimerSection(),
            _Separator(),
            const BatterySection(),
            _Separator(),
            ToggleSwitch(
              label: 'Simulate Activity',
              description: 'Mimic user input to appear active',
              value: settings.simulateActivity,
              onChanged: (value) async {
                if (value) {
                  final allowed = await KeepAlivePlatform.instance
                      .ensureActivitySimulationPermission();
                  if (!allowed) {
                    AppLogger.warning(
                      'Activity simulation needs Accessibility permission before it can move the mouse',
                    );
                    if (context.mounted) {
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(
                          content: Text(
                            'Allow KeepAlive in Accessibility settings, then enable Simulate Activity again.',
                          ),
                        ),
                      );
                    }
                    return;
                  }
                }

                await ref
                    .read(appSettingsProvider.notifier)
                    .setSimulateActivity(value);
                await ref.read(sessionProvider).applySettingsAndRestart();
              },
            ),
          ],
        ],
      ),
    );
  }
}

class _Separator extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Divider(
      height: 1,
      thickness: 0.5,
      color: Theme.of(context).dividerColor.withValues(alpha: 0.4),
    );
  }
}
