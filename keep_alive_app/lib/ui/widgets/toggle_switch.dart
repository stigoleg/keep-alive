import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

class ToggleSwitch extends StatelessWidget {
  final String label;
  final String? description;
  final bool value;
  final ValueChanged<bool>? onChanged;
  final bool enabled;
  final String? tooltip;

  const ToggleSwitch({
    super.key,
    required this.label,
    this.description,
    required this.value,
    this.onChanged,
    this.enabled = true,
    this.tooltip,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final effectiveEnabled = enabled && onChanged != null;

    return Tooltip(
      message: enabled ? (tooltip ?? '') : (tooltip ?? 'Unavailable'),
      child: Opacity(
        opacity: effectiveEnabled ? 1.0 : 0.45,
        child: InkWell(
          onTap: effectiveEnabled
              ? () => onChanged?.call(!value)
              : null,
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
          child: IgnorePointer(
            ignoring: !effectiveEnabled,
            child: Padding(
              padding: const EdgeInsets.symmetric(
                horizontal: AppTheme.spacing4,
                vertical: AppTheme.spacing6,
              ),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(
                          label,
                          style: theme.textTheme.titleMedium?.copyWith(
                            color: theme.colorScheme.onSurface,
                          ),
                        ),
                        if (description != null) ...[
                          const SizedBox(height: AppTheme.spacing4),
                          Text(
                            description!,
                            style: theme.textTheme.bodySmall,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ],
                      ],
                    ),
                  ),
                  const SizedBox(width: AppTheme.spacing8),
                  Switch(
                    value: value,
                    onChanged: effectiveEnabled ? onChanged : null,
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
