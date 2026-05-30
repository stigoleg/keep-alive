import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
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

      test('returns true when binary file exists', () async {
        final path = await service.binaryPath;
        await File(path).create(recursive: true);
        final result = await service.isBinaryInstalled();
        expect(result, isTrue);
      });
    });

    group('getInstalledVersion', () {
      test('returns null when version file does not exist', () async {
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
    });

    group('binaryPath', () {
      test('returns correct path with keepalive name', () async {
        final path = await service.binaryPath;
        expect(path, contains(tempDir.path));
        expect(path, contains('keepalive'));
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
