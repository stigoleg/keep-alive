import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/exceptions.dart';
import 'package:keep_alive_app/services/github_api_service.dart';

import 'test_utils.dart';

void main() {
  group('GitHubApiService', () {
    group('getAssetNameForCurrentPlatform', () {
      test('returns a valid asset name string for the current platform', () {
        final dio = Dio()..httpClientAdapter = MockHttpAdapter((_) => responseBodyFromJson('{}'));
        final service = GitHubApiService(dio: dio);
        final name = service.getAssetNameForCurrentPlatform();
        expect(name, isNotEmpty);
        expect(name, contains('keep-alive'));
        expect(name, anyOf(endsWith('.tar.gz'), endsWith('.zip')));
      });

      test('contains platform OS name', () {
        final dio = Dio()..httpClientAdapter = MockHttpAdapter((_) => responseBodyFromJson('{}'));
        final service = GitHubApiService(dio: dio);
        final name = service.getAssetNameForCurrentPlatform();
        expect(
          name,
          anyOf(
            contains('Darwin'),
            contains('Linux'),
            contains('Windows'),
          ),
        );
      });
    });

    group('latestDownloadUrl', () {
      test('builds the GitHub /releases/latest/download URL for an asset', () {
        final dio = Dio()
          ..httpClientAdapter =
              MockHttpAdapter((_) => responseBodyFromJson('{}'));
        final service = GitHubApiService(dio: dio);
        final url =
            service.latestDownloadUrl('keep-alive_Darwin_arm64.tar.gz');
        expect(
          url,
          'https://github.com/stigoleg/keep-alive/releases/latest/download/'
          'keep-alive_Darwin_arm64.tar.gz',
        );
      });

      test('latestChecksumsUrl points at the same prefix', () {
        final dio = Dio()
          ..httpClientAdapter =
              MockHttpAdapter((_) => responseBodyFromJson('{}'));
        final service = GitHubApiService(dio: dio);
        expect(
          service.latestChecksumsUrl(),
          endsWith('/releases/latest/download/checksums.txt'),
        );
      });
    });

    group('getLatestRelease', () {
      test('parses release response correctly', () async {
        final json = jsonEncode({
          'tag_name': 'v1.5.3',
          'assets': [
            {
              'name': 'keep-alive_Darwin_arm64.tar.gz',
              'browser_download_url': 'https://example.com/dl',
              'size': 5000000,
            },
          ],
        });
        final dio = Dio()
          ..httpClientAdapter = MockHttpAdapter((_) => responseBodyFromJson(json));
        final service = GitHubApiService(dio: dio);
        final release = await service.getLatestRelease();
        expect(release.tagName, 'v1.5.3');
        expect(release.assets.length, 1);
        expect(release.assets[0].name, 'keep-alive_Darwin_arm64.tar.gz');
      });

      test('handles empty assets list', () async {
        final json = jsonEncode({
          'tag_name': 'v1.0.0',
          'assets': [],
        });
        final dio = Dio()
          ..httpClientAdapter = MockHttpAdapter((_) => responseBodyFromJson(json));
        final service = GitHubApiService(dio: dio);
        final release = await service.getLatestRelease();
        expect(release.tagName, 'v1.0.0');
        expect(release.assets, isEmpty);
      });

      test('throws DownloadException on HTTP error', () async {
        final dio = Dio()
          ..httpClientAdapter = MockHttpAdapter(
            (_) => responseBodyFromJson('Not Found', statusCode: 404),
          );
        final service = GitHubApiService(dio: dio);
        expect(
          () => service.getLatestRelease(),
          throwsA(isA<DownloadException>()),
        );
      });
    });
  });
}
