import 'dart:io' show exit;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'app.dart';
import 'core/constants.dart';
import 'core/logger.dart';
import 'platform/platform_interface.dart';
import 'services/instance_lock.dart';

void main(List<String> args) async {
  WidgetsFlutterBinding.ensureInitialized();
  AppLogger.init();

  AppLogger.info('KeepAlive app starting (${AppConstants.appVersion})');

  // Single-instance guard: bail out before window/tray init if another
  // live instance owns the lockfile. Best-effort focus the existing one.
  final lock = await InstanceLock.acquire();
  if (lock == null) {
    try {
      await KeepAlivePlatform.instance.activateExistingInstance();
    } catch (_) {}
    exit(0);
  }

  try {
    await windowManager.ensureInitialized();
    AppLogger.info('Window manager initialized');
  } catch (e, stack) {
    AppLogger.error('Window manager failed to initialize', e, stack);
  }

  try {
    await KeepAlivePlatform.instance.waitUntilNativeReady();
    AppLogger.info('Native platform ready');
  } catch (e, stack) {
    AppLogger.error('Platform not ready, continuing anyway', e, stack);
  }

  AppLogger.info('Launching app widget tree');
  runApp(const ProviderScope(child: KeepAliveApp()));
}
