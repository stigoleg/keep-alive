import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/constants.dart';
import 'package:keep_alive_app/core/exceptions.dart';
import 'package:keep_alive_app/services/cli_download_service.dart';
import 'package:keep_alive_app/services/github_api_service.dart';

import 'test_utils.dart';

void main() {
  group('CliDownloadService', () {
    late Directory tempDir;
    late CliDownloadService service;

    Dio testDio() => Dio()
      ..httpClientAdapter = MockHttpAdapter(
        (_) => responseBodyFromJson('{}'),
      );

    GitHubApiService testApiService() => GitHubApiService(dio: testDio());

    setUp(() async {
      tempDir = await Directory.systemTemp.createTemp('keepalive_test_');
      service = CliDownloadService(
        apiService: testApiService(),
        dio: testDio(),
        appSupportDir: tempDir.path,
      );
    });

    tearDown(() async {
      if (tempDir.existsSync()) {
        await tempDir.delete(recursive: true);
      }
    });

    group('isBinaryInstalled', () {
      test('returns false when binary does not exist', () async {
        final result = await service.isBinaryInstalled();
        expect(result, isFalse);
      });

      test('returns true when binary file exists in app support dir', () async {
        final path = await service.binaryPath;
        await File(path).create(recursive: true);
        final result = await service.isBinaryInstalled();
        expect(result, isTrue);
      });
    });

    group('getInstalledVersion', () {
      test('returns null when version file and binary do not exist', () async {
        final result = await service.getInstalledVersion();
        expect(result, isNull);
      });

      test('returns version from version file', () async {
        final vPath = await service.versionFilePath;
        File(vPath)
          ..createSync(recursive: true)
          ..writeAsStringSync('v1.5.3\n');
        final result = await service.getInstalledVersion();
        expect(result, 'v1.5.3');
      });

      test('trims whitespace from version file', () async {
        final vPath = await service.versionFilePath;
        File(vPath)
          ..createSync(recursive: true)
          ..writeAsStringSync('  v2.0.0  \n');
        final result = await service.getInstalledVersion();
        expect(result, 'v2.0.0');
      });

      test('falls back to binary version parsing when version file absent', () async {
        final binaryPath = await service.binaryPath;
        await _createMockBinary(binaryPath, 'Keep-Alive Version: 1.0.0\n');

        final result = await service.getInstalledVersion();
        expect(result, 'v1.0.0');
      });

      test('returns version from file even when binary also exists', () async {
        final vPath = await service.versionFilePath;
        File(vPath)
          ..createSync(recursive: true)
          ..writeAsStringSync('v2.1.0\n');

        final binaryPath = await service.binaryPath;
        await _createMockBinary(binaryPath, 'Keep-Alive Version: 1.0.0\n');

        final result = await service.getInstalledVersion();
        expect(result, 'v2.1.0');
      });
    });

    group('binaryPath', () {
      test('returns correct path with keepalive name', () async {
        final path = await service.binaryPath;
        expect(path, contains(tempDir.path));
        expect(path, contains('keepalive'));
      });

      test('isUsingSystemBinary starts as false', () {
        expect(service.isUsingSystemBinary, isFalse);
      });
    });

    group('getSystemBinaryVersion', () {
      test('parses version from binary output', () async {
        final binaryPath = await service.binaryPath;
        await _createMockBinary(binaryPath, 'Keep-Alive Version: 1.5.3\n');

        final version = await service.getSystemBinaryVersion(binaryPath);
        expect(version, 'v1.5.3');
      });

      test('parses version with extra output before version', () async {
        final binaryPath = await service.binaryPath;
        await _createMockBinary(
          binaryPath,
          'Some startup info\nKeep-Alive Version: 2.0.0\n',
        );

        final version = await service.getSystemBinaryVersion(binaryPath);
        expect(version, 'v2.0.0');
      });

      test('returns null for non-executable file', () async {
        final result = await service.getSystemBinaryVersion('/nonexistent/path');
        expect(result, isNull);
      });

      test('returns null when version cannot be parsed', () async {
        final binaryPath = await service.binaryPath;
        await _createMockBinary(binaryPath, 'No version here\n');

        final result = await service.getSystemBinaryVersion(binaryPath);
        expect(result, isNull);
      });
    });

    group('versionFilePath', () {
      test('returns path ending with .version', () async {
        final path = await service.versionFilePath;
        expect(path, endsWith('.version'));
        expect(path, contains(tempDir.path));
      });
    });
  });

  group('CliDownloadService.ensureCliInstalled', () {
    late Directory tempDir;
    late Directory bundledDir;

    setUp(() async {
      tempDir = await Directory.systemTemp.createTemp('keepalive_resolve_');
      bundledDir = await Directory.systemTemp.createTemp('keepalive_bundled_');
    });

    tearDown(() async {
      if (tempDir.existsSync()) await tempDir.delete(recursive: true);
      if (bundledDir.existsSync()) await bundledDir.delete(recursive: true);
    });

    CliDownloadService buildService({String? bundledPath}) {
      final dio = Dio()
        ..httpClientAdapter = MockHttpAdapter(
          (_) => responseBodyFromJson('{}'),
        );
      return CliDownloadService(
        apiService: GitHubApiService(dio: dio),
        dio: dio,
        appSupportDir: tempDir.path,
        bundledCliLookup: () async => bundledPath,
      );
    }

    test('prefers bundled CLI over installed binary', () async {
      if (Platform.isWindows) return;
      final bundledPath = '${bundledDir.path}/keepalive';
      await _createMockBinary(bundledPath, 'Keep-Alive Version: 1.5.4\n');

      final managedPath = '${tempDir.path}/keepalive';
      await _createMockBinary(managedPath, 'Keep-Alive Version: 1.5.4\n');

      final service = buildService(bundledPath: bundledPath);
      await service.ensureCliInstalled();

      expect(service.isUsingSystemBinary, isTrue);
      expect(await service.binaryPath, bundledPath);
    });

    test('rejects stale PATH binary below minimum version', () async {
      if (Platform.isWindows) return;
      final service = buildService();

      // Stale binary 1.5.3 lives on PATH; min required is 1.5.4. The adopt
      // step must refuse it so a downgraded Homebrew install cannot mask the
      // fixed bundled CLI.
      const staleVersion = 'Keep-Alive Version: 1.5.3';
      final stalePath = '${tempDir.path}/stale_keepalive';
      await _createMockBinary(stalePath, '$staleVersion\n');

      final ok = await service.tryAdoptForTest(stalePath, requireMin: true);
      expect(ok, isFalse,
          reason: 'CLI below ${AppConstants.minimumCliVersion} must be rejected');
      expect(service.isUsingSystemBinary, isFalse);
    });

    test('accepts PATH binary that meets minimum version', () async {
      if (Platform.isWindows) return;
      final service = buildService();
      final goodPath = '${tempDir.path}/good_keepalive';
      await _createMockBinary(goodPath, 'Keep-Alive Version: 1.5.4\n');

      final ok = await service.tryAdoptForTest(goodPath, requireMin: true);
      expect(ok, isTrue);
      expect(service.isUsingSystemBinary, isTrue);
      expect(await service.binaryPath, goodPath);
    });
  });

  group('CliDownloadService error handling', () {
    test('throws DownloadException when getLatestRelease returns no assets', () async {
      final tempDir = await Directory.systemTemp.createTemp('keepalive_test_');
      try {
        final releaseJson = jsonEncode({
          'tag_name': 'v1.0.0',
          'assets': [],
        });
        final adapter = MockHttpAdapter((_) => responseBodyFromJson(releaseJson));
        final dio = Dio()..httpClientAdapter = adapter;
        final apiService = GitHubApiService(dio: dio);
        final service = CliDownloadService(
          apiService: apiService,
          dio: dio,
          appSupportDir: tempDir.path,
        );

        expect(
          () => service.downloadLatest(),
          throwsA(isA<DownloadException>()),
        );
      } finally {
        await tempDir.delete(recursive: true);
      }
    });
  });
}

Future<void> _createMockBinary(String path, String output) async {
  final file = File(path);
  final parent = file.parent;
  if (!parent.existsSync()) {
    parent.createSync(recursive: true);
  }

  if (Platform.isWindows) {
    final batContent = '@echo off\r\necho $output\r\n';
    await file.writeAsString(batContent);
  } else {
    final scriptContent = '#!/bin/sh\necho "$output"';
    await file.writeAsString(scriptContent);
    await Process.run('chmod', ['+x', path]);
  }
}
