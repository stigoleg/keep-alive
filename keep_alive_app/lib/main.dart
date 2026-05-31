import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'app.dart';
import 'core/constants.dart';
import 'core/logger.dart';
import 'platform/platform_interface.dart';

void main(List<String> args) async {
  WidgetsFlutterBinding.ensureInitialized();
  AppLogger.init();

  AppLogger.info('KeepAlive app starting (${AppConstants.appVersion})');

  await windowManager.ensureInitialized();

  await KeepAlivePlatform.instance.waitUntilNativeReady();

  AppLogger.info('Native platform ready, launching app');

  runApp(const ProviderScope(child: KeepAliveApp()));
}
