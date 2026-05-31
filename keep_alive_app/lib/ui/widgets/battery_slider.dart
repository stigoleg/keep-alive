import 'package:flutter/material.dart';

import '../theme/app_theme.dart';
import '../../utils/format_utils.dart';

class BatterySlider extends StatelessWidget {
  final int value;
  final ValueChanged<int> onChanged;
  final String? label;
  final bool disabled;

  const BatterySlider({
    super.key,
    required this.value,
    required this.onChanged,
    this.label,
    this.disabled = false,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (label != null) ...[
          Row(
            children: [
              Icon(
                Icons.battery_std,
                size: AppTheme.iconSmall,
                color: theme.colorScheme.onSurface.withValues(alpha: 0.6),
              ),
              const SizedBox(width: AppTheme.spacing6),
              Expanded(
                child: Text(
                  label!,
                  style: theme.textTheme.bodySmall,
                ),
              ),
            ],
          ),
          const SizedBox(height: AppTheme.spacing4),
        ],
        Row(
          children: [
            Expanded(
              child: Opacity(
                opacity: disabled ? 0.45 : 1.0,
                child: Slider(
                  value: value.toDouble(),
                  min: 1,
                  max: 100,
                  divisions: 99,
                  activeColor: disabled
                      ? theme.colorScheme.error
                      : theme.colorScheme.primary,
                  onChanged: disabled ? null : (v) => onChanged(v.round()),
                ),
              ),
            ),
            const SizedBox(width: AppTheme.spacing8),
            SizedBox(
              width: 40,
              child: Text(
                FormatUtils.battery(value.toDouble()),
                style: theme.textTheme.titleMedium?.copyWith(
                  color: disabled
                      ? theme.colorScheme.error
                      : theme.colorScheme.onSurface,
                  fontWeight: FontWeight.w700,
                ),
                textAlign: TextAlign.center,
              ),
            ),
          ],
        ),
        if (disabled)
          Padding(
            padding: const EdgeInsets.only(left: AppTheme.spacing12),
            child: Text(
              'Current battery is below threshold',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.error,
                fontStyle: FontStyle.italic,
              ),
            ),
          ),
      ],
    );
  }
}
