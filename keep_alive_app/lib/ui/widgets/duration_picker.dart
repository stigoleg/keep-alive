import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

class DurationPicker extends StatefulWidget {
  final int? durationMinutes;
  final ValueChanged<int?> onChanged;

  const DurationPicker({
    super.key,
    this.durationMinutes,
    required this.onChanged,
  });

  @override
  State<DurationPicker> createState() => _DurationPickerState();
}

class _DurationPickerState extends State<DurationPicker> {
  late int _hours;
  late int _minutes;

  @override
  void initState() {
    super.initState();
    final total = widget.durationMinutes ?? 0;
    _hours = total ~/ 60;
    _minutes = total % 60;
  }

  @override
  void didUpdateWidget(DurationPicker oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.durationMinutes != widget.durationMinutes) {
      final total = widget.durationMinutes ?? 0;
      _hours = total ~/ 60;
      _minutes = total % 60;
    }
  }

  void _emit() {
    final total = _hours * 60 + _minutes;
    widget.onChanged(total > 0 ? total : null);
  }

  void _incrementHours() {
    setState(() {
      _hours = (_hours + 1) % 24;
      if (_hours == 0 && _minutes == 0) _minutes = 5;
    });
    _emit();
  }

  void _decrementHours() {
    setState(() {
      _hours = (_hours - 1) % 24;
      if (_hours < 0) _hours = 23;
      if (_hours == 0 && _minutes == 0) _minutes = 5;
    });
    _emit();
  }

  void _incrementMinutes() {
    setState(() {
      _minutes = (_minutes + 5) % 60;
      if (_hours == 0 && _minutes == 0) _minutes = 5;
    });
    _emit();
  }

  void _decrementMinutes() {
    setState(() {
      _minutes = (_minutes - 5) % 60;
      if (_minutes < 0) _minutes = 55;
      if (_hours == 0 && _minutes == 0) _minutes = 5;
    });
    _emit();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final unitStyle = theme.textTheme.bodyMedium?.copyWith(
      fontWeight: FontWeight.w600,
    );
    final labelStyle = theme.textTheme.bodySmall;

    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        _StepperUnit(
          value: _hours,
          label: 'hr',
          valueStyle: unitStyle,
          labelStyle: labelStyle,
          onIncrement: _incrementHours,
          onDecrement: _decrementHours,
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: AppTheme.spacing4),
          child: Text(':', style: unitStyle),
        ),
        _StepperUnit(
          value: _minutes,
          label: 'min',
          valueStyle: unitStyle,
          labelStyle: labelStyle,
          onIncrement: _incrementMinutes,
          onDecrement: _decrementMinutes,
          padValue: true,
        ),
      ],
    );
  }
}

class _StepperUnit extends StatelessWidget {
  final int value;
  final String label;
  final TextStyle? valueStyle;
  final TextStyle? labelStyle;
  final VoidCallback onIncrement;
  final VoidCallback onDecrement;
  final bool padValue;

  const _StepperUnit({
    required this.value,
    required this.label,
    this.valueStyle,
    this.labelStyle,
    required this.onIncrement,
    required this.onDecrement,
    this.padValue = false,
  });

  @override
  Widget build(BuildContext context) {
    final displayValue = padValue ? value.toString().padLeft(2, '0') : '$value';

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Column(
          children: [
            _ArrowButton(
              icon: Icons.keyboard_arrow_up,
              onTap: onIncrement,
            ),
            const SizedBox(height: AppTheme.spacing4),
            _ArrowButton(
              icon: Icons.keyboard_arrow_down,
              onTap: onDecrement,
            ),
          ],
        ),
        const SizedBox(width: AppTheme.spacing8),
        SizedBox(
          width: 48,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(displayValue, style: valueStyle, textAlign: TextAlign.center),
              Text(label, style: labelStyle, textAlign: TextAlign.center),
            ],
          ),
        ),
      ],
    );
  }
}

class _ArrowButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onTap;

  const _ArrowButton({required this.icon, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
      child: Container(
        width: 28,
        height: 24,
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
          color: theme.colorScheme.surfaceContainerHighest,
        ),
        child: Icon(icon, size: 18, color: theme.colorScheme.onSurface),
      ),
    );
  }
}
