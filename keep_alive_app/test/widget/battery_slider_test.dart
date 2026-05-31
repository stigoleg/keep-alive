import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/ui/widgets/battery_slider.dart';
import 'package:keep_alive_app/utils/format_utils.dart';

Widget buildBatterySlider({
  required int value,
  ValueChanged<int>? onChanged,
  String? label,
  bool disabled = false,
  int maxValue = 100,
}) {
  return MaterialApp(
    home: Scaffold(
      body: BatterySlider(
        value: value,
        onChanged: onChanged ?? (_) {},
        label: label,
        disabled: disabled,
        maxValue: maxValue,
      ),
    ),
  );
}

void main() {
  group('BatterySlider', () {
    testWidgets('renders percentage label when label provided', (tester) async {
      await tester.pumpWidget(
        buildBatterySlider(value: 75, label: 'Stop when battery drops to'),
      );
      expect(find.text(FormatUtils.battery(75.0)), findsOneWidget);
    });

    testWidgets('renders custom label when provided', (tester) async {
      await tester.pumpWidget(
        buildBatterySlider(value: 50, label: 'Stop when battery drops to'),
      );
      expect(find.text('Stop when battery drops to'), findsOneWidget);
    });

    testWidgets('renders without label row when null', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 50, label: null));

      expect(find.byIcon(Icons.battery_std), findsNothing);
    });

    testWidgets('renders slider', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 50));
      expect(find.byType(Slider), findsOneWidget);
    });

    testWidgets('slider value matches provided value', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 42));
      final slider = tester.widget<Slider>(find.byType(Slider));
      expect(slider.value, 42.0);
    });

    testWidgets('slider has correct min and max', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 50));
      final slider = tester.widget<Slider>(find.byType(Slider));
      expect(slider.min, 1.0);
      expect(slider.max, 100.0);
      expect(slider.divisions, 99);
    });

    testWidgets('uses provided max value', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 42, maxValue: 90));
      final slider = tester.widget<Slider>(find.byType(Slider));
      expect(slider.max, 90.0);
      expect(slider.divisions, 89);
    });

    testWidgets('clamps displayed value to provided max', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 93, maxValue: 90));
      final slider = tester.widget<Slider>(find.byType(Slider));
      expect(slider.value, 90.0);
      expect(find.text(FormatUtils.battery(90.0)), findsOneWidget);
    });

    testWidgets('calls onChanged with rounded value when slider dragged', (
      tester,
    ) async {
      int? receivedValue;
      await tester.pumpWidget(
        buildBatterySlider(value: 50, onChanged: (v) => receivedValue = v),
      );

      final slider = find.byType(Slider);
      await tester.drag(slider, const Offset(50, 0));
      await tester.pump();

      expect(receivedValue, isNotNull);
    });

    testWidgets('renders disabled state with reduced opacity', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 50, disabled: true));

      final opacityFinder = find.byWidgetPredicate(
        (w) => w is Opacity && w.opacity == 0.45,
      );
      expect(opacityFinder, findsOneWidget);
    });

    testWidgets('slider is disabled when disabled is true', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 50, disabled: true));

      final slider = tester.widget<Slider>(find.byType(Slider));
      expect(slider.onChanged, isNull);
    });

    testWidgets('shows warning text when disabled', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 50, disabled: true));

      expect(find.text('Current battery is below threshold'), findsOneWidget);
    });

    testWidgets('does not show warning text when enabled', (tester) async {
      await tester.pumpWidget(buildBatterySlider(value: 50, disabled: false));

      expect(find.text('Current battery is below threshold'), findsNothing);
    });

    testWidgets('percentage text shown in disabled state with label', (
      tester,
    ) async {
      await tester.pumpWidget(
        buildBatterySlider(
          value: 30,
          label: 'Stop when battery drops to',
          disabled: true,
        ),
      );

      expect(find.text(FormatUtils.battery(30.0)), findsOneWidget);
    });
  });
}
