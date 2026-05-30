import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/battery_provider.dart';
import '../../providers/process_provider.dart';
import '../../providers/settings_provider.dart';
import '../../utils/format_utils.dart';
import '../theme/app_theme.dart';

class StatusHeader extends ConsumerWidget {
  const StatusHeader({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final processState = ref.watch(cliProcessProvider);
    final settings = ref.watch(appSettingsProvider);
    final batteryAsync = ref.watch(batteryStateProvider);

    final theme = Theme.of(context);
    final isActive = processState.isRunning;

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing12,
        vertical: AppTheme.spacing10,
      ),
      child: Row(
        children: [
          _StatusDot(isActive: isActive),
          const SizedBox(width: AppTheme.spacing8),
          Expanded(
            child: Text(
              _statusText(isActive, settings.durationMinutes,
                  processState.startTime),
              style: theme.textTheme.bodyMedium?.copyWith(
                color: theme.colorScheme.onSurface,
                fontWeight: FontWeight.w600,
              ),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
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

  String _statusText(
      bool isActive, int? durationMinutes, DateTime? startTime) {
    if (!isActive) return 'Idle';

    if (startTime != null && durationMinutes != null) {
      return 'Active \u2014 ${FormatUtils.remainingTime(startTime, durationMinutes)}';
    }
    return 'Active';
  }
}

class _StatusDot extends StatefulWidget {
  final bool isActive;

  const _StatusDot({required this.isActive});

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
    if (widget.isActive) _startPulse();
  }

  @override
  void didUpdateWidget(_StatusDot oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.isActive != oldWidget.isActive) {
      if (widget.isActive) {
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
    final color = widget.isActive
        ? AppTheme.activeColor
        : Theme.of(context).colorScheme.onSurface.withValues(alpha: 0.4);

    return AnimatedBuilder(
      animation: _pulse,
      builder: (context, child) {
        final opacity = widget.isActive ? _pulse.value : 1.0;
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
