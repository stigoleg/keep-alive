import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/ui/widgets/error_boundary.dart';

Widget buildErrorBoundary({required Widget child}) {
  return MaterialApp(
    home: Scaffold(body: ErrorBoundary(child: child)),
  );
}

void main() {
  group('ErrorBoundary', () {
    testWidgets('renders child when no error', (tester) async {
      await tester.pumpWidget(
        buildErrorBoundary(
          child: const Text('All good'),
        ),
      );

      expect(find.text('All good'), findsOneWidget);
    });

    testWidgets('shows fallback UI when FlutterError occurs', (tester) async {
      await tester.pumpWidget(
        buildErrorBoundary(
          child: const Text('Normal child'),
        ),
      );

      FlutterError.onError?.call(
        const FlutterErrorDetails(exception: 'Test exception'),
      );
      await tester.pump();

      expect(find.text('Something went wrong'), findsOneWidget);
      expect(find.text('Restart App'), findsOneWidget);
    });

    testWidgets('fallback UI shows error message', (tester) async {
      await tester.pumpWidget(
        buildErrorBoundary(
          child: const Text('Normal child'),
        ),
      );

      FlutterError.onError?.call(
        const FlutterErrorDetails(exception: 'Connection failed'),
      );
      await tester.pump();

      expect(find.text('Something went wrong'), findsOneWidget);
      expect(find.textContaining('Exception'), findsOneWidget);
    });

    testWidgets('fallback UI has error icon', (tester) async {
      await tester.pumpWidget(
        buildErrorBoundary(
          child: const Text('Normal child'),
        ),
      );

      FlutterError.onError?.call(
        const FlutterErrorDetails(exception: 'Test error'),
      );
      await tester.pump();

      expect(find.byIcon(Icons.error), findsOneWidget);
      expect(find.byIcon(Icons.refresh), findsOneWidget);
    });
  });
}
