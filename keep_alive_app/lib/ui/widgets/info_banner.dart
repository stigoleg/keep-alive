import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

/// Non-error notice banner used to surface informational signals like
/// "Already on the latest version" — sibling of [ErrorBanner], but tinted
/// with the primary colour so the user doesn't read it as a failure.
class InfoBanner extends StatelessWidget {
  final String message;
  final VoidCallback? onDismiss;

  const InfoBanner({
    super.key,
    required this.message,
    this.onDismiss,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final accent = theme.colorScheme.primary;

    return Container(
      margin: const EdgeInsets.only(bottom: AppTheme.spacing4),
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing10,
        vertical: AppTheme.spacing8,
      ),
      decoration: BoxDecoration(
        color: accent.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        border: Border.all(
          color: accent.withValues(alpha: 0.25),
          width: 0.5,
        ),
      ),
      child: Row(
        children: [
          Icon(Icons.info_outline,
              size: AppTheme.iconSmall, color: accent),
          const SizedBox(width: AppTheme.spacing8),
          Expanded(
            child: Text(
              message,
              style: theme.textTheme.bodySmall?.copyWith(color: accent),
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
            ),
          ),
          if (onDismiss != null) ...[
            const SizedBox(width: AppTheme.spacing4),
            Tooltip(
              message: 'Dismiss',
              child: InkWell(
                onTap: onDismiss,
                borderRadius: BorderRadius.circular(4),
                child: Padding(
                  padding: const EdgeInsets.all(4),
                  child: Icon(Icons.close,
                      size: AppTheme.iconSmall, color: accent),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}
