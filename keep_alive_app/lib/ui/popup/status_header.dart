import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/cli_process_state.dart';
import '../../providers/battery_provider.dart';
import '../../providers/process_provider.dart';
import '../../providers/session_provider.dart';
import '../../providers/settings_provider.dart';
import '../../utils/format_utils.dart';
import '../theme/app_theme.dart';

class StatusHeader extends ConsumerStatefulWidget {
  const StatusHeader({super.key, this.onOpenSettings});

  final VoidCallback? onOpenSettings;

  @override
  ConsumerState<StatusHeader> createState() => _StatusHeaderState();
}

class _StatusHeaderState extends ConsumerState<StatusHeader> {
  Timer? _countdownTimer;

  @override
  void dispose() {
    _stopCountdown();
    super.dispose();
  }

  void _stopCountdown() {
    _countdownTimer?.cancel();
    _countdownTimer = null;
  }

  void _syncCountdownTimer(bool isActive, int? durationMinutes) {
    final needsTimer = isActive && durationMinutes != null;
    if (needsTimer && _countdownTimer == null) {
      _countdownTimer = Timer.periodic(const Duration(seconds: 1), (_) {
        if (mounted) setState(() {});
      });
    } else if (!needsTimer && _countdownTimer != null) {
      _stopCountdown();
    }
  }

  @override
  Widget build(BuildContext context) {
    final processState = ref.watch(cliProcessProvider);
    final settings = ref.watch(appSettingsProvider);
    final batteryAsync = ref.watch(batteryStateProvider);

    final theme = Theme.of(context);
    final isActive = processState.isRunning;
    final isError = processState.status == CliProcessStatus.error;

    _syncCountdownTimer(isActive, settings.durationMinutes);

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing12,
        vertical: AppTheme.spacing10,
      ),
      child: Row(
        children: [
          _StatusDot(isActive: isActive, isError: isError),
          const SizedBox(width: AppTheme.spacing8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  _statusText(isActive, isError, settings.durationMinutes,
                      processState.startTime),
                  style: theme.textTheme.bodyMedium?.copyWith(
                    color: isError
                        ? AppTheme.errorColor
                        : theme.colorScheme.onSurface,
                    fontWeight: FontWeight.w600,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                if (isError && processState.errorMessage != null) ...[
                  const SizedBox(height: 2),
                  Text(
                    processState.errorMessage!,
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: AppTheme.errorColor.withValues(alpha: 0.8),
                    ),
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ],
            ),
          ),
          if (isError)
            _RestartButton(
              onPressed: () {
                ref.read(cliProcessProvider.notifier).clearError();
                ref.read(sessionProvider).toggleKeepAwake(true);
              },
            ),
          if (widget.onOpenSettings != null) ...[
            if (isError) const SizedBox(width: AppTheme.spacing4),
            IconButton(
              onPressed: widget.onOpenSettings,
              icon: const Icon(Icons.settings, size: AppTheme.iconSmall),
              padding: EdgeInsets.zero,
              constraints: const BoxConstraints(
                minWidth: 28,
                minHeight: 28,
              ),
              style: IconButton.styleFrom(
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
          ],
          const SizedBox(width: AppTheme.spacing8),
          batteryAsync.when(
            data: (battery) => _BatteryBadge(percentage: battery.percentage),
            loading: () => const _BatteryBadge(percentage: null),
            error: (_, __) => const SizedBox.shrink(),
          ),
        ],
      ),
    );
  }

  String _statusText(bool isActive, bool isError, int? durationMinutes,
      DateTime? startTime) {
    if (isError) return 'Crashed';
    if (!isActive) return 'Idle';

    if (startTime != null && durationMinutes != null) {
      return 'Active \u2014 ${FormatUtils.remainingTime(startTime, durationMinutes)}';
    }
    return 'Active';
  }
}

class _RestartButton extends StatelessWidget {
  final VoidCallback onPressed;

  const _RestartButton({required this.onPressed});

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      onPressed: onPressed,
      icon: const Icon(Icons.refresh, size: AppTheme.iconSmall),
      label: const Text('Restart'),
      style: TextButton.styleFrom(
        foregroundColor: AppTheme.errorColor,
        padding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing6,
          vertical: AppTheme.spacing4,
        ),
        minimumSize: Size.zero,
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
      ),
    );
  }
}

class _StatusDot extends StatefulWidget {
  final bool isActive;
  final bool isError;

  const _StatusDot({required this.isActive, this.isError = false});

  @override
  State<_StatusDot> createState() => _StatusDotState();
}

class _StatusDotState extends State<_StatusDot>
    with SingleTickerProviderStateMixin {
  late AnimationController _controller;
  late Animation<double> _pulse;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 1),
    );
    _pulse = Tween<double>(begin: 1.0, end: 0.4).animate(
      CurvedAnimation(parent: _controller, curve: Curves.easeInOut),
    );
    if (widget.isActive || widget.isError) _startPulse();
  }

  @override
  void didUpdateWidget(_StatusDot oldWidget) {
    super.didUpdateWidget(oldWidget);
    final wasAnimating = oldWidget.isActive || oldWidget.isError;
    final isAnimating = widget.isActive || widget.isError;
    if (isAnimating != wasAnimating) {
      if (isAnimating) {
        _startPulse();
      } else {
        _stopPulse();
      }
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _startPulse() {
    _controller.repeat(reverse: true);
  }

  void _stopPulse() {
    _controller.forward(from: 0).then((_) => _controller.stop());
  }

  @override
  Widget build(BuildContext context) {
    Color color;
    if (widget.isError) {
      color = AppTheme.errorColor;
    } else if (widget.isActive) {
      color = AppTheme.activeColor;
    } else {
      color = Theme.of(context).colorScheme.onSurface.withValues(alpha: 0.4);
    }

    return AnimatedBuilder(
      animation: _pulse,
      builder: (context, child) {
        final animate = widget.isActive || widget.isError;
        final opacity = animate ? _pulse.value : 1.0;
        return Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: color.withValues(alpha: opacity),
          ),
        );
      },
    );
  }
}

class _BatteryBadge extends StatelessWidget {
  final double? percentage;

  const _BatteryBadge({this.percentage});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    if (percentage == null) {
      return Icon(Icons.battery_unknown,
          size: AppTheme.iconSmall,
          color: theme.colorScheme.onSurface.withValues(alpha: 0.4));
    }

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.battery_std,
            size: AppTheme.iconSmall,
            color: theme.colorScheme.onSurface.withValues(alpha: 0.6)),
        const SizedBox(width: AppTheme.spacing4),
        Text(
          FormatUtils.battery(percentage!),
          style: theme.textTheme.bodySmall,
        ),
      ],
    );
  }
}
