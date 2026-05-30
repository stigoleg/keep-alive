import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/constants.dart';
import 'package:keep_alive_app/models/cli_flags.dart';
import 'package:keep_alive_app/models/cli_process_state.dart';
import 'package:keep_alive_app/providers/process_provider.dart';
import 'package:keep_alive_app/services/cli_download_service.dart';
import 'package:keep_alive_app/services/github_api_service.dart';
import 'package:keep_alive_app/services/process_manager.dart';

import '../services/test_utils.dart';

Dio testDio() => Dio()
  ..httpClientAdapter = MockHttpAdapter(
    (_) => responseBodyFromJson('{}'),
  );

GitHubApiService testApiService() => GitHubApiService(dio: testDio());

CliDownloadService testDownloadService(String appSupportDir) {
  return CliDownloadService(
    apiService: testApiService(),
    dio: testDio(),
    appSupportDir: appSupportDir,
  );
}

Future<Directory> createTestEnvironment() async {
  final tempDir =
      await Directory.systemTemp.createTemp('process_provider_test_');

  final binaryName = Platform.isWindows
      ? '${AppConstants.cliBinaryName}.exe'
      : AppConstants.cliBinaryName;

  final scriptPath = '${tempDir.path}/$binaryName';

  if (Platform.isWindows) {
    await File(scriptPath).writeAsString('@echo off\r\n'
        'echo "KeepAlive CLI running"\r\n'
        'ping -n 31 127.0.0.1 >nul\r\n'
        'echo "done"\r\n');
  } else {
    await File(scriptPath).writeAsString(
      '#!/bin/sh\necho "KeepAlive CLI running"\nsleep 30\necho "done"\n',
    );
    await Process.run('chmod', ['+x', scriptPath]);
  }

  return tempDir;
}

void main() {
  group('CliProcessNotifier', () {
    late Directory tempDir;
    late ProcessManager processManager;
    late ProviderContainer container;

    setUp(() async {
      tempDir = await createTestEnvironment();
      processManager = ProcessManager(
        downloadService: testDownloadService(tempDir.path),
      );

      container = ProviderContainer(
        overrides: [
          processManagerProvider.overrideWithValue(processManager),
        ],
      );
    });

    tearDown(() async {
      container.dispose();
      processManager.dispose();
      if (tempDir.existsSync()) {
        await tempDir.delete(recursive: true);
      }
    });

    test('initial state is idle', () {
      final state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.idle);
      expect(state.isRunning, isFalse);
    });

    test('startSession transitions to running', () async {
      await container
          .read(cliProcessProvider.notifier)
          .startSession(const CliFlags());

      final state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.running);
      expect(state.isRunning, isTrue);
      expect(state.pid, isNotNull);

      await container.read(cliProcessProvider.notifier).stopSession();
    });

    test('stopSession transitions to idle', () async {
      await container
          .read(cliProcessProvider.notifier)
          .startSession(const CliFlags());
      await container.read(cliProcessProvider.notifier).stopSession();

      final state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.idle);
      expect(state.isRunning, isFalse);
    });

    test('restartSession restarts the process', () async {
      await container
          .read(cliProcessProvider.notifier)
          .startSession(const CliFlags());

      final firstState = container.read(cliProcessProvider);
      final firstPid = firstState.pid;

      await container.read(cliProcessProvider.notifier).restartSession(
            const CliFlags(durationMinutes: 30),
          );

      final newState = container.read(cliProcessProvider);
      expect(newState.status, CliProcessStatus.running);
      expect(newState.pid, isNot(firstPid));

      await container.read(cliProcessProvider.notifier).stopSession();
    });

    test('stopSession when idle does nothing', () async {
      final before = container.read(cliProcessProvider);
      expect(before.status, CliProcessStatus.idle);

      await container.read(cliProcessProvider.notifier).stopSession();

      final after = container.read(cliProcessProvider);
      expect(after.status, CliProcessStatus.idle);
    });
  });
}
