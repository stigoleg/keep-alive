import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/battery_info.dart';
import 'package:keep_alive_app/providers/battery_provider.dart';
import 'package:keep_alive_app/providers/settings_provider.dart';
import 'package:keep_alive_app/ui/popup/battery_section.dart';

class FakeSettingsNotifier extends AppSettingsNotifier {
  FakeSettingsNotifier(this._state);

  final AppSettingsState _state;

  @override
  AppSettingsState build() => _state;
}

Widget buildBatterySection({
  required AppSettingsState settings,
  required double currentBattery,
}) {
  return ProviderScope(
    overrides: [
      appSettingsProvider.overrideWith(() => FakeSettingsNotifier(settings)),
      batteryStateProvider.overrideWith(
        (ref) => Stream.value(
          BatteryInfo(percentage: currentBattery, isPresent: true),
        ),
      ),
    ],
    child: const MaterialApp(home: Scaffold(body: BatterySection())),
  );
}

void main() {
  group('BatterySection', () {
    testWidgets('caps slider below current battery percentage', (tester) async {
      await tester.pumpWidget(
        buildBatterySection(
          settings: const AppSettingsState(
            keepAwake: true,
            batteryThresholdEnabled: true,
            batteryThreshold: 93,
          ),
          currentBattery: 91,
        ),
      );
      await tester.pump();

      final slider = tester.widget<Slider>(find.byType(Slider));
      expect(slider.max, 90);
      expect(slider.value, 90);
      expect(find.text('90%'), findsOneWidget);
    });
  });
}
