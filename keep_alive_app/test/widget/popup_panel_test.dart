import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/download_state.dart';
import 'package:keep_alive_app/providers/cli_binary_provider.dart';
import 'package:keep_alive_app/providers/settings_provider.dart';
import 'package:keep_alive_app/ui/popup/popup_panel.dart';

class FakeCliBinaryNotifier extends CliBinaryNotifier {
  final DownloadState _state;
  FakeCliBinaryNotifier(this._state);
  @override
  DownloadState build() => _state;
}

class FakeSettingsNotifier extends AppSettingsNotifier {
  final AppSettingsState _state;
  FakeSettingsNotifier(this._state);
  @override
  AppSettingsState build() => _state;
}

Widget buildPopupPanel({
  AppSettingsState settings = const AppSettingsState(),
  DownloadState? binaryState,
}) {
  return ProviderScope(
    overrides: [
      appSettingsProvider.overrideWith(() => FakeSettingsNotifier(settings)),
      cliBinaryProvider.overrideWith(
        () => FakeCliBinaryNotifier(binaryState ?? const DownloadState()),
      ),
    ],
    child: const MaterialApp(home: Scaffold(body: PopupPanel())),
  );
}

void main() {
  group('PopupPanel', () {
    testWidgets('renders all main sections', (tester) async {
      await tester.pumpWidget(
        buildPopupPanel(
          settings: const AppSettingsState(keepAwake: true),
          binaryState: const DownloadState(status: DownloadStatus.installed),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Idle'), findsOneWidget);
      expect(find.text('Keep System Awake'), findsOneWidget);
      expect(find.text('Simulate Activity'), findsOneWidget);
      expect(find.text('Timer'), findsOneWidget);
    });

    testWidgets('renders dividers between sections', (tester) async {
      await tester.pumpWidget(
        buildPopupPanel(
          settings: const AppSettingsState(keepAwake: true),
          binaryState: const DownloadState(status: DownloadStatus.installed),
        ),
      );
      await tester.pumpAndSettle();

      final dividers = find.byType(Divider);
      expect(dividers, findsAtLeastNWidgets(3));
    });

    testWidgets('renders without error when settings are default', (
      tester,
    ) async {
      await tester.pumpWidget(buildPopupPanel());
      await tester.pumpAndSettle();

      expect(find.byType(PopupPanel), findsOneWidget);
    });

    testWidgets('timer section visible when keepAwake is on', (tester) async {
      await tester.pumpWidget(
        buildPopupPanel(
          settings: const AppSettingsState(keepAwake: true),
          binaryState: const DownloadState(status: DownloadStatus.installed),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Timer'), findsOneWidget);
      expect(find.text('Indefinite'), findsOneWidget);
    });

    testWidgets('dismisses CLI error banner', (tester) async {
      await tester.pumpWidget(
        buildPopupPanel(
          binaryState: const DownloadState(
            status: DownloadStatus.error,
            errorMessage: 'Binary not found',
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Binary not found'), findsOneWidget);

      await tester.tap(find.byTooltip('Dismiss'));
      await tester.pumpAndSettle();

      expect(find.text('Binary not found'), findsNothing);
    });
  });
}
