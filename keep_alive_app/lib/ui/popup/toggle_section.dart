import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/download_state.dart';
import '../../providers/cli_binary_provider.dart';
import '../../providers/session_provider.dart';
import '../../providers/settings_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/toggle_switch.dart';
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
            const SizedBox(height: AppTheme.spacing8),
            const TimerSection(),
            _Separator(),
          ],
          ToggleSwitch(
            label: 'Simulate Activity',
            description: 'Mimic user input to appear active',
            value: settings.simulateActivity,
            onChanged: (value) {
              ref.read(appSettingsProvider.notifier).setSimulateActivity(value);
              ref.read(sessionProvider).applySettingsAndRestart();
            },
          ),
          _Separator(),
          ToggleSwitch(
            label: 'Enable Logging',
            description: 'Write debug output to log file',
            value: settings.enableLogging,
            onChanged: (value) {
              ref.read(appSettingsProvider.notifier).setEnableLogging(value);
              ref.read(sessionProvider).applySettingsAndRestart();
            },
          ),
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
