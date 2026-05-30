import 'package:flutter/material.dart';

import '../theme/app_theme.dart';
import 'battery_section.dart';
import 'cli_status_footer.dart';
import 'status_header.dart';
import 'timer_section.dart';
import 'toggle_section.dart';

class PopupPanel extends StatelessWidget {
  const PopupPanel({super.key});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      backgroundColor: Colors.transparent,
      body: Container(
        decoration: BoxDecoration(
          color: theme.colorScheme.surface,
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
        ),
        clipBehavior: Clip.antiAlias,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const StatusHeader(),
              _buildDivider(theme),
              const ToggleSection(),
              _buildDivider(theme),
              const TimerSection(),
              _buildDivider(theme),
              const BatterySection(),
              _buildDivider(theme),
              const CliStatusFooter(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildDivider(ThemeData theme) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: AppTheme.spacing12),
      child: Divider(
        height: 1,
        thickness: 0.5,
        color: theme.dividerColor.withValues(alpha: 0.3),
      ),
    );
  }
}
