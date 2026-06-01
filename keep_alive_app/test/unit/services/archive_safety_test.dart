import 'dart:io';

import 'package:archive/archive.dart';
import 'package:archive/archive_io.dart' as archive_io;
import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/exceptions.dart';
import 'package:keep_alive_app/services/cli_download_service.dart';
import 'package:keep_alive_app/services/github_api_service.dart';

import 'test_utils.dart';

void main() {
  group('CliDownloadService archive safety', () {
    late Directory appSupport;

    setUp(() async {
      appSupport = await Directory.systemTemp.createTemp('archive_safety_');
    });

    tearDown(() async {
      if (appSupport.existsSync()) {
        await appSupport.delete(recursive: true);
      }
    });

    test('rejects archives with path-traversal entries', () async {
      // Use the actual platform's expected asset name so the test works
      // regardless of where it runs.
      final assetName =
          GitHubApiService(dio: Dio()).getAssetNameForCurrentPlatform();
      final isZip = assetName.toLowerCase().endsWith('.zip');
      final maliciousBytes =
          isZip ? _buildMaliciousZip() : _buildMaliciousTarGz();
      final assetSha = sha256.convert(maliciousBytes).toString();

      final service = _serviceFor(
        appSupportDir: appSupport.path,
        assetName: assetName,
        assetBytes: maliciousBytes,
        checksumBody: '$assetSha  $assetName\n',
      );

      await expectLater(
        () => service.downloadLatest(),
        throwsA(
          isA<DownloadException>().having(
            (e) => e.message,
            'message',
            anyOf(contains('Unsafe archive entry'), contains('Failed to install')),
          ),
        ),
      );
    });
  });

  group('CliDownloadService checksum verification', () {
    late Directory appSupport;

    setUp(() async {
      appSupport = await Directory.systemTemp.createTemp('checksum_test_');
    });

    tearDown(() async {
      if (appSupport.existsSync()) {
        await appSupport.delete(recursive: true);
      }
    });

    test('rejects archive whose SHA256 does not match checksums.txt',
        () async {
      final assetName =
          GitHubApiService(dio: Dio()).getAssetNameForCurrentPlatform();
      final isZip = assetName.toLowerCase().endsWith('.zip');
      final archiveBytes = isZip ? _buildBenignZip() : _buildBenignTarGz();
      final wrongSha = sha256.convert(<int>[0]).toString();

      final service = _serviceFor(
        appSupportDir: appSupport.path,
        assetName: assetName,
        assetBytes: archiveBytes,
        checksumBody: '$wrongSha  $assetName\n',
      );

      await expectLater(
        () => service.downloadLatest(),
        throwsA(
          isA<DownloadException>().having(
            (e) => e.message,
            'message',
            contains('Checksum mismatch'),
          ),
        ),
      );
    });

    test('accepts archive whose SHA256 matches checksums.txt', () async {
      final assetName =
          GitHubApiService(dio: Dio()).getAssetNameForCurrentPlatform();
      final isZip = assetName.toLowerCase().endsWith('.zip');
      final archiveBytes = isZip ? _buildBenignZip() : _buildBenignTarGz();
      final goodSha = sha256.convert(archiveBytes).toString();

      final service = _serviceFor(
        appSupportDir: appSupport.path,
        assetName: assetName,
        assetBytes: archiveBytes,
        checksumBody: '$goodSha  $assetName\n',
      );

      // Should not throw on checksum step; later steps may still fail
      // (binary not present in archive), but the message proves the
      // checksum step passed first.
      try {
        await service.downloadLatest();
      } on DownloadException catch (e) {
        expect(e.message, isNot(contains('Checksum mismatch')));
      }
    });
  });
}

CliDownloadService _serviceFor({
  required String appSupportDir,
  required String assetName,
  required List<int> assetBytes,
  required String checksumBody,
}) {
  Dio buildDio() {
    final dio = Dio();
    dio.httpClientAdapter = _DispatchAdapter((options) {
      final url = options.uri.toString();
      // Specific-suffix matches first so the /releases/latest/download/
      // prefix on both URLs doesn't trip the order.
      if (url.endsWith('checksums.txt')) {
        return _plainTextBody(checksumBody);
      }
      if (url.endsWith(assetName)) {
        return _bytesBody(assetBytes);
      }
      return responseBodyFromJson('{}', statusCode: 404);
    });
    return dio;
  }

  return CliDownloadService(
    apiService: GitHubApiService(dio: buildDio()),
    dio: buildDio(),
    appSupportDir: appSupportDir,
  );
}

ResponseBody _plainTextBody(String body) {
  return ResponseBody.fromString(
    body,
    200,
    headers: {
      'content-type': ['text/plain; charset=utf-8'],
    },
  );
}

ResponseBody _bytesBody(List<int> bytes) {
  return ResponseBody.fromBytes(
    bytes,
    200,
    headers: {
      'content-type': ['application/octet-stream'],
    },
  );
}

class _DispatchAdapter implements HttpClientAdapter {
  final ResponseBody Function(RequestOptions options) handler;
  _DispatchAdapter(this.handler);

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    final body = handler(options);
    // Dio's `download()` writes the response stream to a file. Provide a
    // ResponseBody backed by the bytes; Dio handles streaming for both
    // file-saving and in-memory uses.
    return body;
  }

  @override
  void close({bool force = false}) {}
}

/// Builds a zip archive containing a single entry whose name escapes the
/// extraction directory via `../../`.
List<int> _buildMaliciousZip() {
  final archive = Archive();
  final payload = <int>[0xCA, 0xFE, 0xBA, 0xBE];
  archive.addFile(
    ArchiveFile('../../etc/passwd_owned', payload.length, payload),
  );
  return ZipEncoder().encode(archive);
}

/// Builds a tar.gz archive containing a single path-traversal entry.
List<int> _buildMaliciousTarGz() {
  final archive = Archive();
  final payload = <int>[0xCA, 0xFE, 0xBA, 0xBE];
  archive.addFile(
    ArchiveFile('../../etc/passwd_owned', payload.length, payload),
  );
  final tar = TarEncoder().encode(archive);
  return const archive_io.GZipEncoder().encodeBytes(tar);
}

/// Builds a benign zip with a single regular file.
List<int> _buildBenignZip() {
  final archive = Archive();
  final payload = <int>[0x68, 0x69];
  archive.addFile(ArchiveFile('keep-alive/keepalive', payload.length, payload));
  return ZipEncoder().encode(archive);
}

/// Builds a benign tar.gz containing a single regular file. Used for the
/// checksum tests; we don't care about extraction success there.
List<int> _buildBenignTarGz() {
  final archive = Archive();
  final payload = <int>[0x68, 0x69]; // "hi"
  archive.addFile(ArchiveFile('keep-alive/keepalive', payload.length, payload));
  final tar = TarEncoder().encode(archive);
  return const archive_io.GZipEncoder().encodeBytes(tar);
}
