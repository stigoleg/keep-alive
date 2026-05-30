import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/ui/widgets/duration_picker.dart';

Widget buildDurationPicker({
  int? durationMinutes,
  ValueChanged<int?>? onChanged,
}) {
  return MaterialApp(
    home: Scaffold(
      body: DurationPicker(
        durationMinutes: durationMinutes,
        onChanged: onChanged ?? (_) {},
      ),
    ),
  );
}

void main() {
  group('DurationPicker', () {
    testWidgets('renders hour and minute displays', (tester) async {
      await tester.pumpWidget(buildDurationPicker(durationMinutes: 65));

      expect(find.text('hr'), findsOneWidget);
      expect(find.text('min'), findsOneWidget);
      expect(find.text(':'), findsOneWidget);
    });

    testWidgets('displays hours and minutes correctly', (tester) async {
      await tester.pumpWidget(buildDurationPicker(durationMinutes: 125));

      expect(find.text('2'), findsOneWidget);
      expect(find.text('05'), findsOneWidget);
    });

    testWidgets('shows zeroes when duration is null', (tester) async {
      await tester.pumpWidget(buildDurationPicker(durationMinutes: null));

      expect(find.text('0'), findsOneWidget);
    });

    testWidgets('renders up and down arrow buttons', (tester) async {
      await tester.pumpWidget(buildDurationPicker(durationMinutes: 60));

      expect(find.byIcon(Icons.keyboard_arrow_up), findsNWidgets(2));
      expect(find.byIcon(Icons.keyboard_arrow_down), findsNWidgets(2));
    });

    testWidgets('calls onChanged when hour incremented', (tester) async {
      int? receivedValue;
      await tester.pumpWidget(
        buildDurationPicker(
          durationMinutes: 60,
          onChanged: (v) => receivedValue = v,
        ),
      );

      final upButtons = find.byIcon(Icons.keyboard_arrow_up);
      await tester.tap(upButtons.first);
      await tester.pump();

      expect(receivedValue, 120);
    });

    testWidgets('calls onChanged when minute incremented', (tester) async {
      int? receivedValue;
      await tester.pumpWidget(
        buildDurationPicker(
          durationMinutes: 25,
          onChanged: (v) => receivedValue = v,
        ),
      );

      final upButtons = find.byIcon(Icons.keyboard_arrow_up);
      await tester.tap(upButtons.last);
      await tester.pump();

      expect(receivedValue, 30);
    });

    testWidgets('calls onChanged when hour decremented from 0 wraps to 23h',
        (tester) async {
      int? receivedValue;
      await tester.pumpWidget(
        buildDurationPicker(
          durationMinutes: 0,
          onChanged: (v) => receivedValue = v,
        ),
      );

      final downButtons = find.byIcon(Icons.keyboard_arrow_down);
      await tester.tap(downButtons.first);
      await tester.pump();

      expect(receivedValue, 1380);
    });

    testWidgets('calls onChanged when minute decremented step by step',
        (tester) async {
      int? receivedValue;
      await tester.pumpWidget(
        buildDurationPicker(
          durationMinutes: 10,
          onChanged: (v) => receivedValue = v,
        ),
      );

      final downButtons = find.byIcon(Icons.keyboard_arrow_down);
      await tester.tap(downButtons.last);
      await tester.pump();

      expect(receivedValue, 5);
    });

    testWidgets('emits non-null when total is positive', (tester) async {
      int? receivedValue;
      await tester.pumpWidget(
        buildDurationPicker(
          durationMinutes: 5,
          onChanged: (v) => receivedValue = v,
        ),
      );

      final downButtons = find.byIcon(Icons.keyboard_arrow_down);
      await tester.tap(downButtons.last);
      await tester.pump();

      expect(receivedValue, 5);
    });

    testWidgets('hours wrap from 23 to 0 on increment', (tester) async {
      int? receivedValue;
      await tester.pumpWidget(
        buildDurationPicker(
          durationMinutes: 1380,
          onChanged: (v) => receivedValue = v,
        ),
      );

      final upButtons = find.byIcon(Icons.keyboard_arrow_up);
      await tester.tap(upButtons.first);
      await tester.pump();

      expect(receivedValue, 5);
    });
  });
}
