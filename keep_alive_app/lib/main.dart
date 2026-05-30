import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'app.dart';
import 'core/constants.dart';
import 'core/logger.dart';

void main(List<String> args) async {
  WidgetsFlutterBinding.ensureInitialized();
  AppLogger.init();

  AppLogger.info('KeepAlive app starting (${AppConstants.appVersion})');

  final startHidden = args.contains('--hidden');

  await windowManager.ensureInitialized();

  if (startHidden) {
    await windowManager.hide();
  }

  await windowManager.setTitle(AppConstants.appName);

  AppLogger.info('Window manager initialized, launching app');

  runApp(const ProviderScope(child: KeepAliveApp()));
}
