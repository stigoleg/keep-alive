import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

class ErrorBanner extends StatelessWidget {
  final String message;
  final VoidCallback? onRetry;
  final VoidCallback? onDismiss;

  const ErrorBanner({
    super.key,
    required this.message,
    this.onRetry,
    this.onDismiss,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Container(
      margin: const EdgeInsets.only(bottom: AppTheme.spacing4),
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing10,
        vertical: AppTheme.spacing8,
      ),
      decoration: BoxDecoration(
        color: AppTheme.errorColor.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        border: Border.all(
          color: AppTheme.errorColor.withValues(alpha: 0.25),
          width: 0.5,
        ),
      ),
      child: Row(
        children: [
          const Icon(Icons.error_outline,
              size: AppTheme.iconSmall, color: AppTheme.errorColor),
          const SizedBox(width: AppTheme.spacing8),
          Expanded(
            child: Text(
              message,
              style: theme.textTheme.bodySmall?.copyWith(
                color: AppTheme.errorColor,
              ),
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
            ),
          ),
          if (onRetry != null) ...[
            const SizedBox(width: AppTheme.spacing8),
            _CompactButton(
              icon: Icons.refresh,
              tooltip: 'Retry',
              onPressed: onRetry,
            ),
          ],
          if (onDismiss != null) ...[
            const SizedBox(width: AppTheme.spacing4),
            _CompactButton(
              icon: Icons.close,
              tooltip: 'Dismiss',
              onPressed: onDismiss,
            ),
          ],
        ],
      ),
    );
  }
}

class _CompactButton extends StatelessWidget {
  final IconData icon;
  final String tooltip;
  final VoidCallback? onPressed;

  const _CompactButton({
    required this.icon,
    required this.tooltip,
    this.onPressed,
  });

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      child: InkWell(
        onTap: onPressed,
        borderRadius: BorderRadius.circular(4),
        child: Padding(
          padding: const EdgeInsets.all(4),
          child: Icon(icon, size: AppTheme.iconSmall,
              color: AppTheme.errorColor),
        ),
      ),
    );
  }
}
