import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/ui/widgets/error_banner.dart';

Widget buildErrorBanner({
  required String message,
  VoidCallback? onRetry,
  VoidCallback? onDismiss,
}) {
  return MaterialApp(
    home: Scaffold(
      body: ErrorBanner(
        message: message,
        onRetry: onRetry,
        onDismiss: onDismiss,
      ),
    ),
  );
}

void main() {
  group('ErrorBanner', () {
    testWidgets('renders error message', (tester) async {
      await tester.pumpWidget(
        buildErrorBanner(message: 'Something went wrong'),
      );

      expect(find.text('Something went wrong'), findsOneWidget);
    });

    testWidgets('renders error icon', (tester) async {
      await tester.pumpWidget(
        buildErrorBanner(message: 'Error occurred'),
      );

      expect(find.byIcon(Icons.error_outline), findsOneWidget);
    });

    testWidgets('renders retry button when onRetry is provided', (tester) async {
      await tester.pumpWidget(
        buildErrorBanner(
          message: 'Error occurred',
          onRetry: () {},
        ),
      );

      expect(find.byIcon(Icons.refresh), findsOneWidget);
    });

    testWidgets('does not render retry button when onRetry is null',
        (tester) async {
      await tester.pumpWidget(
        buildErrorBanner(
          message: 'Error occurred',
          onRetry: null,
        ),
      );

      expect(find.byIcon(Icons.refresh), findsNothing);
    });

    testWidgets('renders dismiss button when onDismiss is provided',
        (tester) async {
      await tester.pumpWidget(
        buildErrorBanner(
          message: 'Error occurred',
          onRetry: () {},
          onDismiss: () {},
        ),
      );

      expect(find.byIcon(Icons.close), findsOneWidget);
    });

    testWidgets('does not render dismiss button when onDismiss is null',
        (tester) async {
      await tester.pumpWidget(
        buildErrorBanner(
          message: 'Error occurred',
          onRetry: () {},
          onDismiss: null,
        ),
      );

      expect(find.byIcon(Icons.close), findsNothing);
    });

    testWidgets('calls onRetry when retry button tapped', (tester) async {
      bool retryCalled = false;
      await tester.pumpWidget(
        buildErrorBanner(
          message: 'Error occurred',
          onRetry: () => retryCalled = true,
        ),
      );

      await tester.tap(find.byIcon(Icons.refresh));
      expect(retryCalled, isTrue);
    });

    testWidgets('calls onDismiss when dismiss button tapped', (tester) async {
      bool dismissCalled = false;
      await tester.pumpWidget(
        buildErrorBanner(
          message: 'Error occurred',
          onRetry: () {},
          onDismiss: () => dismissCalled = true,
        ),
      );

      await tester.tap(find.byIcon(Icons.close));
      expect(dismissCalled, isTrue);
    });
  });
}
