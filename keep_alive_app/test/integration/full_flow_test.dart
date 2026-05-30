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

import '../unit/services/test_utils.dart';

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
      await Directory.systemTemp.createTemp('integration_test_');

  final binaryName = Platform.isWindows
      ? '${AppConstants.cliBinaryName}.exe'
      : AppConstants.cliBinaryName;

  final scriptPath = '${tempDir.path}/$binaryName';

  if (Platform.isWindows) {
    await File(scriptPath).writeAsString('@echo off\r\n'
        'echo "KeepAlive CLI running"\r\n'
        'ping -n 6 127.0.0.1 >nul\r\n'
        'echo "done"\r\n');
  } else {
    await File(scriptPath).writeAsString(
      '#!/bin/sh\necho "KeepAlive CLI running"\nsleep 5\necho "done"\n',
    );
    await Process.run('chmod', ['+x', scriptPath]);
  }

  final versionFilePath = '${tempDir.path}/${AppConstants.cliBinaryName}.version';
  await File(versionFilePath).writeAsString('1.0.0');

  return tempDir;
}

void main() {
  group('Full flow integration', () {
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

    test('download mock binary -> start session -> change flags -> stop -> cleanup',
        () async {
      // Verify initial state
      final initialState = container.read(cliProcessProvider);
      expect(initialState.status, CliProcessStatus.idle);
      expect(initialState.isRunning, isFalse);

      // Start session
      await container
          .read(cliProcessProvider.notifier)
          .startSession(const CliFlags(durationMinutes: 10));

      var state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.running);
      expect(state.isRunning, isTrue);
      expect(state.pid, isNotNull);

      // Change flags (restart)
      await container.read(cliProcessProvider.notifier).restartSession(
            const CliFlags(durationMinutes: 30, simulateActivity: true),
          );

      state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.running);
      expect(state.isRunning, isTrue);

      // Stop session
      await container.read(cliProcessProvider.notifier).stopSession();

      state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.idle);
      expect(state.isRunning, isFalse);

      // Verify cleanup
      expect(processManager.isRunning, isFalse);
      expect(processManager.pid, isNull);
    });

    test('stopSession when stopped is a no-op', () async {
      await container.read(cliProcessProvider.notifier).stopSession();
      final state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.idle);
    });

    test('start -> stop -> start again works', () async {
      await container
          .read(cliProcessProvider.notifier)
          .startSession(const CliFlags(durationMinutes: 5));

      var state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.running);

      await container.read(cliProcessProvider.notifier).stopSession();
      state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.idle);

      await container
          .read(cliProcessProvider.notifier)
          .startSession(const CliFlags(durationMinutes: 15));

      state = container.read(cliProcessProvider);
      expect(state.status, CliProcessStatus.running);

      await container.read(cliProcessProvider.notifier).stopSession();
    });

    test('process manager records stdout', () async {
      processManager.stdoutStream.listen((line) {
        // We just want to test that output is received
      });

      await container
          .read(cliProcessProvider.notifier)
          .startSession(const CliFlags());

      // Wait a bit for output
      await Future<void>.delayed(const Duration(seconds: 1));

      // Check that stdout buffer has content
      final lines = processManager.stdoutLines;
      expect(lines.isNotEmpty, isTrue);

      await container.read(cliProcessProvider.notifier).stopSession();
    });
  });
}
