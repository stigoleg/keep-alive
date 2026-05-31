import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/ui/widgets/toggle_switch.dart';
import 'package:keep_alive_app/utils/platform_utils.dart';

Widget _buildToggleSwitch({
  required bool value,
  ValueChanged<bool>? onChanged,
  bool enabled = true,
  String label = 'Test Label',
  String? description,
  String? tooltip,
}) {
  return MaterialApp(
    home: Scaffold(
      body: ToggleSwitch(
        label: label,
        description: description,
        value: value,
        onChanged: onChanged,
        enabled: enabled,
        tooltip: tooltip,
      ),
    ),
  );
}

Type get _switchType => PlatformUtils.isMacOS ? CupertinoSwitch : Switch;

void main() {
  group('ToggleSwitch', () {
    testWidgets('renders label and switch', (tester) async {
      await tester.pumpWidget(_buildToggleSwitch(value: false));
      expect(find.text('Test Label'), findsOneWidget);
      final switchType = _switchType;
      if (switchType == CupertinoSwitch) {
        expect(find.byType(CupertinoSwitch), findsOneWidget);
      } else {
        expect(find.byType(Switch), findsOneWidget);
      }
    });

    testWidgets('renders description when provided', (tester) async {
      await tester.pumpWidget(
        _buildToggleSwitch(
          value: false,
          description: 'A helpful description',
        ),
      );
      expect(find.text('A helpful description'), findsOneWidget);
    });

    testWidgets('does not render description when null', (tester) async {
      await tester.pumpWidget(_buildToggleSwitch(value: false));
      final found = find.textContaining('description');
      expect(found, findsNothing);
    });

    testWidgets('calls onChanged with opposite value when tapped', (tester) async {
      bool? toggledValue;
      await tester.pumpWidget(
        _buildToggleSwitch(
          value: false,
          onChanged: (v) => toggledValue = v,
        ),
      );

      await tester.tap(find.byType(InkWell));
      expect(toggledValue, isTrue);

      toggledValue = null;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ToggleSwitch(
              label: 'Test Label',
              value: true,
              onChanged: (v) => toggledValue = v,
            ),
          ),
        ),
      );
      await tester.tap(find.byType(InkWell));
      expect(toggledValue, isFalse);
    });

    testWidgets('renders reduced opacity when disabled', (tester) async {
      await tester.pumpWidget(
        _buildToggleSwitch(value: false, enabled: false),
      );

      final opacities = find.byWidgetPredicate(
        (w) => w is Opacity && w.opacity == 0.45,
      );
      expect(opacities, findsOneWidget);
    });

    testWidgets('switch is disabled when enabled is false', (tester) async {
      await tester.pumpWidget(
        _buildToggleSwitch(value: false, enabled: false),
      );

      final switchType = _switchType;
      if (switchType == CupertinoSwitch) {
        final switchWidget =
            tester.widget<CupertinoSwitch>(find.byType(CupertinoSwitch));
        expect(switchWidget.onChanged, isNull);
      } else {
        final switchWidget = tester.widget<Switch>(find.byType(Switch));
        expect(switchWidget.onChanged, isNull);
      }
    });

    testWidgets('switch is disabled when onChanged is null', (tester) async {
      await tester.pumpWidget(
        _buildToggleSwitch(value: false, onChanged: null),
      );

      final switchType = _switchType;
      if (switchType == CupertinoSwitch) {
        final switchWidget =
            tester.widget<CupertinoSwitch>(find.byType(CupertinoSwitch));
        expect(switchWidget.onChanged, isNull);
      } else {
        final switchWidget = tester.widget<Switch>(find.byType(Switch));
        expect(switchWidget.onChanged, isNull);
      }
    });

    testWidgets('renders tooltip with unavailable message when disabled',
        (tester) async {
      await tester.pumpWidget(
        _buildToggleSwitch(
          value: false,
          enabled: false,
          tooltip: 'CLI binary not installed',
        ),
      );

      final tooltip = tester.widget<Tooltip>(find.byType(Tooltip));
      expect(tooltip.message, contains('CLI binary not installed'));
    });

    testWidgets('renders tooltip with message when enabled', (tester) async {
      await tester.pumpWidget(
        _buildToggleSwitch(
          value: false,
          enabled: true,
          tooltip: 'Custom tooltip',
        ),
      );

      final tooltip = tester.widget<Tooltip>(find.byType(Tooltip));
      expect(tooltip.message, 'Custom tooltip');
    });

    testWidgets('does not call onChanged when disabled and tapped',
        (tester) async {
      bool? toggledValue;
      await tester.pumpWidget(
        _buildToggleSwitch(
          value: false,
          enabled: false,
          onChanged: (v) => toggledValue = v,
        ),
      );

      await tester.tap(find.byType(ToggleSwitch));
      expect(toggledValue, isNull);
    });
  });
}
