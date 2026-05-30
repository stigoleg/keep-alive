import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/battery_info.dart';
import '../services/battery_monitor.dart';

final batteryMonitorProvider = Provider<BatteryMonitor>((ref) {
  final monitor = BatteryMonitor();
  ref.onDispose(monitor.dispose);
  return monitor;
});

final batteryStateProvider = StreamProvider<BatteryInfo>((ref) {
  final monitor = ref.watch(batteryMonitorProvider);
  monitor.startPolling();

  ref.onDispose(monitor.stopPolling);

  return monitor.batteryStream;
});
