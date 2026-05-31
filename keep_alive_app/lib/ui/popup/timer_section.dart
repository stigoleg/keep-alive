import 'package:flutter/cupertino.dart';
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

class _ClockPicker extends StatelessWidget {
  final DateTime? clockTime;
  final ValueChanged<DateTime> onChanged;

  const _ClockPicker({this.clockTime, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final hasTime = clockTime != null;
    final display = hasTime
        ? FormatUtils.timeOfDay(clockTime!)
        : 'Pick a time';

    return InkWell(
      onTap: () => _showTimePicker(context),
      borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing16,
          vertical: AppTheme.spacing12,
        ),
        decoration: BoxDecoration(
          border: Border.all(
            color: theme.colorScheme.outline.withValues(alpha: 0.5),
          ),
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              Icons.access_time,
              size: AppTheme.iconMedium,
              color: theme.colorScheme.primary,
            ),
            const SizedBox(width: AppTheme.spacing8),
            Text(
              display,
              style: theme.textTheme.titleMedium?.copyWith(
                color: hasTime
                    ? theme.colorScheme.onSurface
                    : theme.colorScheme.onSurface.withValues(alpha: 0.5),
                fontWeight: FontWeight.w600,
              ),
            ),
            const SizedBox(width: AppTheme.spacing4),
            Icon(
              Icons.arrow_drop_down,
              size: AppTheme.iconSmall,
              color: theme.colorScheme.onSurface.withValues(alpha: 0.6),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _showTimePicker(BuildContext context) async {
    final now = DateTime.now();
    final initial = clockTime ??
        DateTime(now.year, now.month, now.day, now.hour + 1);

    DateTime selected = initial;
    final theme = Theme.of(context);
    final use24h = MediaQuery.of(context).alwaysUse24HourFormat;

    final confirmed = await showDialog<bool>(
      context: context,
      barrierDismissible: true,
      builder: (dialogContext) {
        return Dialog(
          backgroundColor: theme.colorScheme.surface,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
          ),
          child: SizedBox(
            width: 280,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Padding(
                  padding: const EdgeInsets.only(
                    top: AppTheme.spacing12,
                    bottom: AppTheme.spacing4,
                  ),
                  child: Text('Until time',
                      style: theme.textTheme.titleMedium),
                ),
                SizedBox(
                  height: 180,
                  child: CupertinoTheme(
                    data: CupertinoThemeData(
                      brightness: theme.brightness,
                      textTheme: CupertinoTextThemeData(
                        dateTimePickerTextStyle: TextStyle(
                          fontSize: 20,
                          color: theme.colorScheme.onSurface,
                        ),
                      ),
                    ),
                    child: CupertinoDatePicker(
                      mode: CupertinoDatePickerMode.time,
                      use24hFormat: use24h,
                      initialDateTime: initial,
                      onDateTimeChanged: (dt) => selected = dt,
                    ),
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(
                    AppTheme.spacing8,
                    AppTheme.spacing4,
                    AppTheme.spacing8,
                    AppTheme.spacing8,
                  ),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      TextButton(
                        onPressed: () =>
                            Navigator.of(dialogContext).pop(false),
                        child: const Text('Cancel'),
                      ),
                      const SizedBox(width: AppTheme.spacing4),
                      FilledButton(
                        onPressed: () =>
                            Navigator.of(dialogContext).pop(true),
                        child: const Text('Done'),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );

    if (confirmed != true || !context.mounted) return;

    final today = DateTime.now();
    var date = DateTime(
      today.year,
      today.month,
      today.day,
      selected.hour,
      selected.minute,
    );
    if (date.isBefore(today)) {
      date = date.add(const Duration(days: 1));
    }

    onChanged(date);
  }
}
