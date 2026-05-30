import 'dart:io' show exit;

import 'package:flutter/material.dart';

import '../../core/logger.dart';
import '../theme/app_theme.dart';

class ErrorBoundary extends StatefulWidget {
  final Widget child;
  final VoidCallback? onRetry;

  const ErrorBoundary({super.key, required this.child, this.onRetry});

  @override
  State<ErrorBoundary> createState() => _ErrorBoundaryState();
}

class _ErrorBoundaryState extends State<ErrorBoundary> {
  Object? _error;
  StackTrace? _stackTrace;

  @override
  void initState() {
    super.initState();
    FlutterError.onError = _onFlutterError;
  }

  void _onFlutterError(FlutterErrorDetails details) {
    setState(() {
      _error = details.exception;
      _stackTrace = details.stack;
    });
    AppLogger.error(
      'Unhandled Flutter error: ${details.exception}',
      details.exception,
      details.stack,
    );
  }

  @override
  void dispose() {
    FlutterError.onError = null;
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final error = _error;
    if (error != null) {
      return _ErrorFallback(
        message: error.toString(),
        stackTrace: _stackTrace,
        onRestart: () {
          widget.onRetry?.call();
          exit(1);
        },
      );
    }
    return widget.child;
  }
}

class _ErrorFallback extends StatelessWidget {
  final String message;
  final StackTrace? stackTrace;
  final VoidCallback onRestart;

  const _ErrorFallback({
    required this.message,
    required this.onRestart,
    this.stackTrace,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Container(
      color: theme.colorScheme.surface,
      padding: const EdgeInsets.all(AppTheme.spacing16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              const Icon(Icons.error, color: AppTheme.errorColor, size: AppTheme.iconMedium),
              const SizedBox(width: AppTheme.spacing8),
              Expanded(
                child: Text(
                  'Something went wrong',
                  style: theme.textTheme.titleMedium?.copyWith(
                    color: AppTheme.errorColor,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: AppTheme.spacing12),
          Text(
            message,
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurface.withValues(alpha: 0.7),
            ),
          ),
          const SizedBox(height: AppTheme.spacing16),
          FilledButton.icon(
            onPressed: onRestart,
            icon: const Icon(Icons.refresh, size: AppTheme.iconSmall),
            label: const Text('Restart App'),
            style: FilledButton.styleFrom(
              minimumSize: const Size(double.infinity, 36),
            ),
          ),
        ],
      ),
    );
  }
}
