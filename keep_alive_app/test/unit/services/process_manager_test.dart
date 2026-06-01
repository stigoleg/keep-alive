import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/constants.dart';
import 'package:keep_alive_app/core/exceptions.dart';
import 'package:keep_alive_app/models/cli_flags.dart';
import 'package:keep_alive_app/services/cli_download_service.dart';
import 'package:keep_alive_app/services/github_api_service.dart';
import 'package:keep_alive_app/services/process_manager.dart';

import 'test_utils.dart';

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
      await Directory.systemTemp.createTemp('process_manager_test_');

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
  late Directory tempDir;
  late ProcessManager processManager;

  setUp(() async {
    tempDir = await createTestEnvironment();
    processManager = ProcessManager(
      downloadService: testDownloadService(tempDir.path),
    );
  });

  tearDown(() async {
    await processManager.dispose();
    if (tempDir.existsSync()) {
      await tempDir.delete(recursive: true);
    }
  });

  group('ProcessManager', () {
    group('start', () {
      test('starts a process successfully', () async {
        await processManager.start(const CliFlags());
        expect(processManager.isRunning, isTrue);
        expect(processManager.pid, isNotNull);
      });

      test('prevents double-start when already running', () async {
        await processManager.start(const CliFlags());
        final firstPid = processManager.pid;

        await processManager.start(const CliFlags());
        expect(processManager.pid, firstPid);
      });

      test('serializes concurrent start calls to a single spawn', () async {
        // Issue 50 starts in parallel — even though each await is async, the
        // Lock must serialize them so the second-onwards observe _hasProcess
        // and return without spawning a second process.
        final futures = List.generate(
          50,
          (_) => processManager.start(const CliFlags()),
        );
        await Future.wait(futures);

        expect(processManager.isRunning, isTrue);
        expect(processManager.pid, isNotNull);
      });

      test('throws CliProcessException for missing binary', () async {
        final badDir = await Directory.systemTemp.createTemp('bad_binary_');
        try {
          final badManager = ProcessManager(
            downloadService: testDownloadService(badDir.path),
          );
          addTearDown(badManager.dispose);

          expect(
            () => badManager.start(const CliFlags()),
            throwsA(isA<CliProcessException>()),
          );
        } finally {
          if (badDir.existsSync()) {
            await badDir.delete(recursive: true);
          }
        }
      });
    });

    group('stop', () {
      test('stops a running process gracefully', () async {
        await processManager.start(const CliFlags());
        expect(processManager.isRunning, isTrue);

        await processManager.stop();
        expect(processManager.isRunning, isFalse);
      });

      test('does nothing when no process is running', () async {
        await processManager.stop();
        expect(processManager.isRunning, isFalse);
      });
    });

    group('restart', () {
      test('stops and starts with new flags', () async {
        await processManager.start(const CliFlags());
        final firstPid = processManager.pid;

        await processManager.restart(const CliFlags(
          durationMinutes: 60,
          simulateActivity: true,
        ));

        expect(processManager.isRunning, isTrue);
        expect(processManager.pid, isNot(firstPid));
      });

      test('restart works when no process is running', () async {
        await processManager.restart(const CliFlags(durationMinutes: 30));
        expect(processManager.isRunning, isTrue);
      });
    });

    group('stdout streaming', () {
      test('receives stdout output from process', () async {
        final output = <String>[];
        final subscription =
            processManager.stdoutStream.listen(output.add);
        addTearDown(subscription.cancel);

        await processManager.start(const CliFlags());

        await Future<void>.delayed(const Duration(seconds: 1));
        expect(output.isNotEmpty, isTrue,
            reason: 'Should receive at least one line of stdout output');

        await processManager.stop();
      });
    });

    group('dispose', () {
      test('kills running process and cleans up', () async {
        await processManager.start(const CliFlags());
        expect(processManager.isRunning, isTrue);

        await processManager.dispose();

        expect(processManager.isRunning, isFalse);
        expect(processManager.pid, isNull);
      });
    });

    group('ring buffer', () {
      test('stdoutLines captures process output', () async {
        await processManager.start(const CliFlags());
        await Future<void>.delayed(const Duration(milliseconds: 500));
        await processManager.stop();

        expect(processManager.stdoutLines.isNotEmpty, isTrue);
      });

      test('stderrLines starts empty', () async {
        await processManager.start(const CliFlags());
        await Future<void>.delayed(const Duration(milliseconds: 200));
        await processManager.stop();

        expect(processManager.stderrLines, isEmpty);
      });
    });
  });
}
