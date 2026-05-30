import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'app.dart';
import 'core/logger.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  AppLogger.init();

  await windowManager.ensureInitialized();
  await windowManager.hide();

  runApp(const ProviderScope(child: KeepAliveApp()));
}
