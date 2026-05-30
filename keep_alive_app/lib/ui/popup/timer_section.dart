import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/session_provider.dart';
import '../../providers/settings_provider.dart';
import '../../utils/format_utils.dart';
import '../theme/app_theme.dart';
import '../widgets/duration_picker.dart';

enum _TimerMode { indefinite, duration, clock }

class TimerSection extends ConsumerStatefulWidget {
  const TimerSection({super.key});

  @override
  ConsumerState<TimerSection> createState() => _TimerSectionState();
}

class _TimerSectionState extends ConsumerState<TimerSection> {
  DateTime? _clockTime;

  _TimerMode _currentMode() {
    final settings = ref.read(appSettingsProvider);
    if (settings.clockTime != null) return _TimerMode.clock;
    if (settings.durationMinutes != null) return _TimerMode.duration;
    return _TimerMode.indefinite;
  }

  @override
  Widget build(BuildContext context) {
    final settings = ref.watch(appSettingsProvider);
    final theme = Theme.of(context);

    if (!settings.keepAwake) return const SizedBox.shrink();

    final mode = _currentMode();

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing12,
        vertical: AppTheme.spacing4,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text('Timer', style: theme.textTheme.titleMedium),
          const SizedBox(height: AppTheme.spacing4),
          Row(
            children: [
              _ModeChip(
                label: 'Indefinite',
                selected: mode == _TimerMode.indefinite,
                onTap: () => _setMode(_TimerMode.indefinite),
              ),
              const SizedBox(width: AppTheme.spacing6),
              _ModeChip(
                label: 'Duration',
                selected: mode == _TimerMode.duration,
                onTap: () => _setMode(_TimerMode.duration),
              ),
              const SizedBox(width: AppTheme.spacing6),
              _ModeChip(
                label: 'Until Time',
                selected: mode == _TimerMode.clock,
                onTap: () => _setMode(_TimerMode.clock),
              ),
            ],
          ),
          const SizedBox(height: AppTheme.spacing12),
          if (mode == _TimerMode.duration)
            DurationPicker(
              durationMinutes: settings.durationMinutes,
              onChanged: _onDurationChanged,
            ),
          if (mode == _TimerMode.clock) _buildClockPicker(settings.clockTime ?? _clockTime),
        ],
      ),
    );
  }

  void _setMode(_TimerMode mode) {
    final notifier = ref.read(appSettingsProvider.notifier);
    switch (mode) {
      case _TimerMode.indefinite:
        notifier.setDurationMinutes(null);
        notifier.setClockTime(null);
        _clockTime = null;
        break;
      case _TimerMode.duration:
        notifier.setClockTime(null);
        _clockTime = null;
        if (ref.read(appSettingsProvider).durationMinutes == null) {
          notifier.setDurationMinutes(60);
        }
        break;
      case _TimerMode.clock:
        notifier.setDurationMinutes(null);
        final now = DateTime.now();
        final nextHour = DateTime(now.year, now.month, now.day, now.hour + 1);
        _clockTime = nextHour;
        notifier.setClockTime(nextHour);
        break;
    }
    ref.read(sessionProvider).applySettingsAndRestart();
  }

  void _onDurationChanged(int? minutes) {
    ref.read(appSettingsProvider.notifier).setDurationMinutes(minutes);
    ref.read(sessionProvider).applySettingsAndRestart();
  }

  void _onClockTimeChanged(DateTime clockTime) {
    ref.read(appSettingsProvider.notifier).setClockTime(clockTime);
    ref.read(sessionProvider).applySettingsAndRestart();
  }

  Widget _buildClockPicker(DateTime? clockTime) {
    return _ClockPicker(
      clockTime: clockTime,
      onChanged: _onClockTimeChanged,
    );
  }
}

class _ModeChip extends StatelessWidget {
  final String label;
  final bool selected;
  final VoidCallback onTap;

  const _ModeChip({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Material(
      color: selected
          ? theme.colorScheme.primaryContainer
          : theme.colorScheme.surfaceContainerHighest,
      borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        child: Padding(
          padding: const EdgeInsets.symmetric(
            horizontal: AppTheme.spacing10,
            vertical: AppTheme.spacing6,
          ),
          child: Text(
            label,
            style: theme.textTheme.labelMedium?.copyWith(
              color: selected
                  ? theme.colorScheme.onPrimaryContainer
                  : theme.colorScheme.onSurface,
              fontWeight: selected ? FontWeight.w600 : FontWeight.normal,
            ),
          ),
        ),
      ),
    );
  }
}

class _ClockPicker extends StatefulWidget {
  final DateTime? clockTime;
  final ValueChanged<DateTime> onChanged;

  const _ClockPicker({this.clockTime, required this.onChanged});

  @override
  State<_ClockPicker> createState() => _ClockPickerState();
}

class _ClockPickerState extends State<_ClockPicker> {
  late TextEditingController _controller;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController(
      text: widget.clockTime != null
          ? FormatUtils.timeOfDay24(widget.clockTime!)
          : '',
    );
  }

  @override
  void didUpdateWidget(_ClockPicker oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.clockTime != widget.clockTime) {
      _controller.text = widget.clockTime != null
          ? FormatUtils.timeOfDay24(widget.clockTime!)
          : '';
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Container(
      padding: const EdgeInsets.all(AppTheme.spacing12),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.access_time,
              size: AppTheme.iconMedium,
              color: theme.colorScheme.onSurface.withValues(alpha: 0.6)),
          const SizedBox(width: AppTheme.spacing8),
          SizedBox(
            width: 80,
            child: TextField(
              controller: _controller,
              textAlign: TextAlign.center,
              style: theme.textTheme.headlineSmall,
              decoration: const InputDecoration(
                hintText: 'HH:MM',
                contentPadding: EdgeInsets.symmetric(
                  horizontal: AppTheme.spacing8,
                  vertical: AppTheme.spacing6,
                ),
              ),
              keyboardType: TextInputType.datetime,
              onSubmitted: (value) => _validateAndSet(value),
              onTapOutside: (_) {
                FocusScope.of(context).unfocus();
              },
            ),
          ),
          const SizedBox(width: AppTheme.spacing4),
          Text(
            '(24h)',
            style: theme.textTheme.bodySmall,
          ),
        ],
      ),
    );
  }

  void _validateAndSet(String value) {
    final parts = value.trim().split(':');
    if (parts.length != 2) return;

    final hour = int.tryParse(parts[0]);
    final minute = int.tryParse(parts[1]);
    if (hour == null || minute == null) return;
    if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return;

    final now = DateTime.now();
    var clockTime = DateTime(now.year, now.month, now.day, hour, minute);
    if (clockTime.isBefore(now)) {
      clockTime = clockTime.add(const Duration(days: 1));
    }

    widget.onChanged(clockTime);
  }
}
