import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/cli_process_state.dart';
import 'package:keep_alive_app/providers/process_provider.dart';
import 'package:keep_alive_app/providers/settings_provider.dart';
import 'package:keep_alive_app/ui/popup/status_header.dart';

class FakeCliProcessNotifier extends CliProcessNotifier {
  final CliProcessState _state;
  FakeCliProcessNotifier(this._state);
  @override
  CliProcessState build() => _state;
}

class FakeSettingsNotifier extends AppSettingsNotifier {
  final AppSettingsState _state;
  FakeSettingsNotifier(this._state);
  @override
  AppSettingsState build() => _state;
}

Widget buildStatusHeader({
  CliProcessState processState = const CliProcessState(),
  AppSettingsState settings = const AppSettingsState(),
}) {
  return ProviderScope(
    overrides: [
      cliProcessProvider.overrideWith(() => FakeCliProcessNotifier(processState)),
      appSettingsProvider.overrideWith(() => FakeSettingsNotifier(settings)),
    ],
    child: const MaterialApp(
      home: Scaffold(body: StatusHeader()),
    ),
  );
}

void main() {
  group('StatusHeader', () {
    testWidgets('displays Idle when not active', (tester) async {
      await tester.pumpWidget(buildStatusHeader());
      expect(find.text('Idle'), findsOneWidget);
    });

    testWidgets('displays Active when running', (tester) async {
      await tester.pumpWidget(
        buildStatusHeader(
          processState: const CliProcessState(
            status: CliProcessStatus.running,
            pid: 1234,
          ),
          settings: const AppSettingsState(keepAwake: true),
        ),
      );
      expect(find.text('Active'), findsOneWidget);
    });

    testWidgets('displays remaining time when both startTime and duration set',
        (tester) async {
      final startTime = DateTime.now().subtract(const Duration(minutes: 15));

      await tester.pumpWidget(
        buildStatusHeader(
          processState: CliProcessState(
            status: CliProcessStatus.running,
            pid: 1234,
            startTime: startTime,
          ),
          settings: const AppSettingsState(
            keepAwake: true,
            durationMinutes: 120,
          ),
        ),
      );

      expect(find.textContaining('Active'), findsOneWidget);
    });

    testWidgets('displays Crashed when process is in error state',
        (tester) async {
      await tester.pumpWidget(
        buildStatusHeader(
          processState: const CliProcessState(
            status: CliProcessStatus.error,
            errorMessage: 'Process crashed',
          ),
        ),
      );
      expect(find.text('Crashed'), findsOneWidget);
    });

    testWidgets('displays error message when in error state', (tester) async {
      await tester.pumpWidget(
        buildStatusHeader(
          processState: const CliProcessState(
            status: CliProcessStatus.error,
            errorMessage: 'Connection refused',
          ),
        ),
      );
      expect(find.text('Connection refused'), findsOneWidget);
    });

    testWidgets('shows Restart button when in error state', (tester) async {
      await tester.pumpWidget(
        buildStatusHeader(
          processState: const CliProcessState(
            status: CliProcessStatus.error,
            errorMessage: 'Timeout',
          ),
        ),
      );
      expect(find.text('Restart'), findsOneWidget);
    });

    testWidgets('does not show Restart button when not in error',
        (tester) async {
      await tester.pumpWidget(buildStatusHeader());
      expect(find.text('Restart'), findsNothing);
    });
  });
}
