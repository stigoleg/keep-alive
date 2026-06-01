import 'dart:convert';
import 'dart:io';

import 'package:archive/archive.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/core/constants.dart';
import 'package:keep_alive_app/core/exceptions.dart';
import 'package:keep_alive_app/services/cli_download_service.dart';
import 'package:keep_alive_app/services/github_api_service.dart';
import 'package:path/path.dart' as p;

import 'test_utils.dart';

void main() {
  group('CliDownloadService', () {
    late Directory tempDir;
    late CliDownloadService service;

    Dio testDio() => Dio()
      ..httpClientAdapter = MockHttpAdapter((_) => responseBodyFromJson('{}'));

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

      test(
        'falls back to binary version parsing when version file absent',
        () async {
          final binaryPath = await service.binaryPath;
          await _createMockBinary(binaryPath, 'Keep-Alive Version: 1.0.0\n');

          final result = await service.getInstalledVersion();
          expect(result, 'v1.0.0');
        },
      );

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
        final result = await service.getSystemBinaryVersion(
          '/nonexistent/path',
        );
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

    test('rejects PATH binary that fails --version verification', () async {
      if (Platform.isWindows) return;
      final service = buildService();

      // A "binary" that exits non-zero on --version must not be adopted —
      // verification (not a hardcoded minimum) is the only adoption gate.
      final brokenPath = '${tempDir.path}/broken_keepalive';
      await File(brokenPath).writeAsString('#!/bin/sh\nexit 1\n');
      await Process.run('chmod', ['+x', brokenPath]);

      final ok = await service.tryAdoptForTest(brokenPath);
      expect(ok, isFalse, reason: 'Unverifiable CLI must be rejected');
      expect(service.isUsingSystemBinary, isFalse);
    });

    test('accepts any PATH binary that responds to --version', () async {
      if (Platform.isWindows) return;
      final service = buildService();
      final goodPath = '${tempDir.path}/good_keepalive';
      await _createMockBinary(goodPath, 'Keep-Alive Version: 1.5.4\n');

      final ok = await service.tryAdoptForTest(goodPath);
      expect(ok, isTrue);
      expect(service.isUsingSystemBinary, isTrue);
      expect(await service.binaryPath, goodPath);
    });
  });

  group('CliDownloadService.updateLatest', () {
    late Directory tempDir;

    setUp(() async {
      tempDir = await Directory.systemTemp.createTemp('keepalive_update_');
    });

    tearDown(() async {
      if (tempDir.existsSync()) await tempDir.delete(recursive: true);
    });

    test('installs binary from /releases/latest/download/ redirect',
        () async {
      final apiDio = Dio();
      final assetName = GitHubApiService(
        dio: apiDio,
      ).getAssetNameForCurrentPlatform();
      final archiveBytes = _archiveWithBinary(assetName);
      final dio = Dio()
        ..httpClientAdapter = MockHttpAdapter((options) {
          final uri = options.uri.toString();
          // Specific-suffix matches first so the shared /latest/download/
          // prefix does not bleed across handlers.
          if (uri.endsWith('checksums.txt')) {
            return ResponseBody.fromString('', 404);
          }
          if (uri.endsWith(assetName)) {
            expect(
              uri,
              contains('/releases/latest/download/'),
              reason: 'CLI must download via the stable redirect prefix',
            );
            return ResponseBody.fromBytes(archiveBytes, 200);
          }
          return ResponseBody.fromBytes(utf8.encode('not found'), 404);
        });

      // Stub anything that would let updateLatest discover Homebrew or
      // Scoop on the host machine, so the test exercises only the direct
      // /releases/latest/download/ path. We must also intercept calls that
      // come from the hardcoded `/opt/homebrew/bin/brew` fallback path
      // (the lookup tries File.existsSync() on those, which we cannot
      // mock — so instead we stub the subsequent invocations).
      Future<ProcessResult> downloadOnlyRunner(
        String executable,
        List<String> arguments, {
        bool runInShell = false,
      }) async {
        if ((executable == 'which' || executable == 'where') &&
            arguments.contains('brew')) {
          return ProcessResult(0, 1, '', '');
        }
        if (executable == 'powershell') {
          return ProcessResult(0, 1, '', '');
        }
        final isBrew =
            executable.endsWith('/brew') || executable == 'brew';
        if (isBrew) {
          // Pretend keepalive is not installed via Homebrew so the update
          // flow keeps walking and lands on the direct download path.
          return ProcessResult(0, 1, '', '');
        }
        return _testProcessRunner(
          executable,
          arguments,
          runInShell: runInShell,
        );
      }

      final service = CliDownloadService(
        apiService: GitHubApiService(dio: dio),
        dio: dio,
        appSupportDir: tempDir.path,
        processRunner: downloadOnlyRunner,
      );

      await service.updateLatest();

      final binaryPath = await service.binaryPath;
      expect(File(binaryPath).existsSync(), isTrue);
      // Version file is now sourced from the binary's --version output, not
      // a GitHub tag. The mock binary echoes "Keep-Alive Version: 9.9.9".
      expect(await service.getInstalledVersion(), 'v9.9.9');
      expect(service.installSource, CliInstallSource.appManaged);
    });

    test('uses Homebrew update when formula is installed', () async {
      if (Platform.isWindows) return;

      final prefix = Directory('${tempDir.path}/brew-prefix')
        ..createSync(recursive: true);
      final binaryPath = p.join(prefix.path, 'bin', 'keepalive');
      await _createMockBinary(binaryPath, 'Keep-Alive Version: 1.5.4\n');
      final calls = <String>[];

      Future<ProcessResult> processRunner(
        String executable,
        List<String> arguments, {
        bool runInShell = false,
      }) async {
        calls.add('$executable ${arguments.join(' ')}');
        if (executable == 'which' && arguments.join(' ') == 'brew') {
          return ProcessResult(1, 0, '/opt/homebrew/bin/brew\n', '');
        }
        if (executable == '/opt/homebrew/bin/brew') {
          final joined = arguments.join(' ');
          if (joined == 'list --formula keepalive') {
            return ProcessResult(2, 0, '', '');
          }
          if (joined == 'tap stigoleg/homebrew-tap') {
            return ProcessResult(3, 0, '', '');
          }
          if (joined == 'upgrade keepalive') {
            return ProcessResult(4, 0, '', '');
          }
          if (joined == '--prefix keepalive') {
            return ProcessResult(5, 0, '${prefix.path}\n', '');
          }
        }
        if (executable == binaryPath &&
            arguments.length == 1 &&
            arguments.single == AppConstants.cliVersionArg) {
          return ProcessResult(6, 0, 'Keep-Alive Version: 1.5.4\n', '');
        }
        return ProcessResult(7, 1, '', 'unexpected command');
      }

      final service = CliDownloadService(
        apiService: GitHubApiService(dio: Dio()),
        dio: Dio()
          ..httpClientAdapter = MockHttpAdapter((_) {
            fail('Homebrew update should not download release archives');
          }),
        appSupportDir: tempDir.path,
        processRunner: processRunner,
      );

      await service.updateLatest();

      expect(calls, contains('/opt/homebrew/bin/brew upgrade keepalive'));
      expect(service.installSource, CliInstallSource.homebrew);
      expect(await service.binaryPath, binaryPath);
    });

    test('refuses Homebrew downgrade and keeps current binary', () async {
      if (Platform.isWindows) return;

      // Bundled CLI is already adopted at v1.5.4; the Homebrew tap still
      // serves v1.5.3. This is the exact scenario that bit a real user:
      // brew upgrade succeeds but its binary is older than what we have,
      // and the older binary lacks the headless auto-detect that v1.5.4
      // introduced — so silently adopting it would crash next start.
      final prefix = Directory('${tempDir.path}/brew-prefix')
        ..createSync(recursive: true);
      final brewBinary = p.join(prefix.path, 'bin', 'keepalive');
      await _createMockBinary(brewBinary, 'Keep-Alive Version: 1.5.3\n');

      final bundledDir = await Directory.systemTemp.createTemp(
        'keepalive_bundled_',
      );
      addTearDown(() async {
        if (bundledDir.existsSync()) {
          await bundledDir.delete(recursive: true);
        }
      });
      final bundledPath = '${bundledDir.path}/keepalive';
      await _createMockBinary(bundledPath, 'Keep-Alive Version: 1.5.4\n');

      Future<ProcessResult> processRunner(
        String executable,
        List<String> arguments, {
        bool runInShell = false,
      }) async {
        if (executable == 'which' && arguments.join(' ') == 'brew') {
          return ProcessResult(1, 0, '/opt/homebrew/bin/brew\n', '');
        }
        if (executable == '/opt/homebrew/bin/brew') {
          final joined = arguments.join(' ');
          if (joined == 'list --formula keepalive') {
            return ProcessResult(2, 0, '', '');
          }
          if (joined == 'tap stigoleg/homebrew-tap') {
            return ProcessResult(3, 0, '', '');
          }
          if (joined == 'upgrade keepalive') {
            return ProcessResult(4, 0, '', '');
          }
          if (joined == '--prefix keepalive') {
            return ProcessResult(5, 0, '${prefix.path}\n', '');
          }
        }
        if (executable == brewBinary &&
            arguments.single == AppConstants.cliVersionArg) {
          return ProcessResult(6, 0, 'Keep-Alive Version: 1.5.3\n', '');
        }
        if (executable == bundledPath &&
            arguments.single == AppConstants.cliVersionArg) {
          return ProcessResult(7, 0, 'Keep-Alive Version: 1.5.4\n', '');
        }
        return ProcessResult(99, 1, '', 'unexpected command');
      }

      final service = CliDownloadService(
        apiService: GitHubApiService(dio: Dio()),
        dio: Dio()
          ..httpClientAdapter = MockHttpAdapter((_) {
            fail('Homebrew downgrade path should not download archives');
          }),
        appSupportDir: tempDir.path,
        processRunner: processRunner,
        bundledCliLookup: () async => bundledPath,
      );

      // Adopt bundled v1.5.4 first to match the user's real state.
      await service.ensureCliInstalled();
      expect(service.installSource, CliInstallSource.bundled);

      await expectLater(
        () => service.updateLatest(),
        throwsA(
          isA<DownloadException>().having(
            (e) => e.message,
            'message',
            allOf(
              contains('Refusing to downgrade'),
              contains('1.5.4'),
              contains('1.5.3'),
            ),
          ),
        ),
      );

      // Critically: we still point at the bundled binary, not Homebrew's.
      expect(service.installSource, CliInstallSource.bundled);
      expect(await service.binaryPath, bundledPath);
    });
  });

  group('CliDownloadService error handling', () {
    test(
      'throws DownloadException when getLatestRelease returns no assets',
      () async {
        final tempDir = await Directory.systemTemp.createTemp(
          'keepalive_test_',
        );
        try {
          final releaseJson = jsonEncode({'tag_name': 'v1.0.0', 'assets': []});
          final adapter = MockHttpAdapter(
            (_) => responseBodyFromJson(releaseJson),
          );
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
      },
    );
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

List<int> _archiveWithBinary(String assetName) {
  final binaryName = assetName.endsWith('.zip') ? 'keepalive.exe' : 'keepalive';
  final content = utf8.encode('#!/bin/sh\necho "Keep-Alive Version: 9.9.9"\n');
  final archive = Archive()..addFile(ArchiveFile.bytes(binaryName, content));

  if (assetName.endsWith('.zip')) {
    return ZipEncoder().encode(archive);
  }

  final tarBytes = TarEncoder().encode(archive);
  return const GZipEncoder().encode(tarBytes);
}

Future<ProcessResult> _testProcessRunner(
  String executable,
  List<String> arguments, {
  bool runInShell = false,
}) async {
  // Real exec for chmod (used to mark the extracted binary executable) and
  // for the binary itself (used by _parseVersionFromBinary). Everything
  // else stays mocked so unrelated `which`/`brew` lookups don't accidentally
  // hit the host system.
  if (executable == 'chmod' || File(executable).existsSync()) {
    return Process.run(executable, arguments, runInShell: runInShell);
  }
  return ProcessResult(0, 1, '', 'unexpected command');
}
