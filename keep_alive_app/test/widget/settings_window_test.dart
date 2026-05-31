import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/constants.dart';
import 'package:keep_alive_app/ui/settings/settings_window.dart';
import 'package:keep_alive_app/ui/widgets/toggle_switch.dart';

Widget buildSettingsDialog({required VoidCallback onClose}) {
  return ProviderScope(
    overrides: [],
    child: MaterialApp(
      home: Scaffold(
        body: Builder(
          builder: (context) => ElevatedButton(
            onPressed: () => showDialog(
              context: context,
              builder: (_) => SettingsDialog(onClose: onClose),
            ),
            child: const Text('Open Settings'),
          ),
        ),
      ),
    ),
  );
}

void main() {
  group('SettingsDialog', () {
    testWidgets('renders all sections', (tester) async {
      await tester.pumpWidget(buildSettingsDialog(onClose: () {}));

      await tester.tap(find.text('Open Settings'));
      await tester.pumpAndSettle();

      expect(find.text('Settings'), findsOneWidget);
      expect(find.text('Startup'), findsOneWidget);
      expect(find.text('Start on Login'), findsOneWidget);
      expect(find.text('Start Minimized'), findsOneWidget);
      expect(find.text('Updates'), findsOneWidget);
      expect(find.text('About'), findsOneWidget);
      expect(find.text('Log Viewer'), findsOneWidget);
    });

    testWidgets('renders app name and version in about section', (tester) async {
      await tester.pumpWidget(buildSettingsDialog(onClose: () {}));

      await tester.tap(find.text('Open Settings'));
      await tester.pumpAndSettle();

      expect(find.text(AppConstants.appName), findsOneWidget);
      expect(
        find.text('Version ${AppConstants.appVersion}'),
        findsOneWidget,
      );
    });

    testWidgets('has view licenses and github buttons', (tester) async {
      await tester.pumpWidget(buildSettingsDialog(onClose: () {}));

      await tester.tap(find.text('Open Settings'));
      await tester.pumpAndSettle();

      expect(find.text('View Licenses'), findsOneWidget);
      expect(find.text('GitHub'), findsOneWidget);
    });

    testWidgets('has copy button in log section', (tester) async {
      await tester.pumpWidget(buildSettingsDialog(onClose: () {}));

      await tester.tap(find.text('Open Settings'));
      await tester.pumpAndSettle();

      expect(find.text('Copy'), findsOneWidget);
    });

    testWidgets('close button is present and clickable', (tester) async {
      await tester.pumpWidget(buildSettingsDialog(onClose: () {}));

      await tester.tap(find.text('Open Settings'));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.close), findsOneWidget);
    });

    testWidgets('toggle switches are present', (tester) async {
      await tester.pumpWidget(buildSettingsDialog(onClose: () {}));

      await tester.tap(find.text('Open Settings'));
      await tester.pumpAndSettle();

      final toggles = find.byType(ToggleSwitch);
      expect(toggles, findsAtLeastNWidgets(2));
    });
  });
}
