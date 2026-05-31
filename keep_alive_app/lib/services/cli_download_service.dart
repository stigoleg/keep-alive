import 'dart:async';
import 'dart:io';

import 'package:archive/archive_io.dart' as archive_io;
import 'package:dio/dio.dart';
import 'package:path_provider/path_provider.dart';

import '../core/constants.dart';
import '../core/exceptions.dart';
import '../core/logger.dart';
import '../utils/platform_utils.dart';
import 'github_api_service.dart';

class CliDownloadService {
  final GitHubApiService _apiService;
  final Dio _dio;
  final String? _appSupportDirOverride;

  String? _binaryPath;
  String? _versionFilePath;
  String? _urlCachePath;
  bool _usingSystemBinary = false;
  String? _systemBinaryPath;

  CliDownloadService({
    required this._apiService,
    required this._dio,
    String? appSupportDir,
  }) : _appSupportDirOverride = appSupportDir;

  bool get isUsingSystemBinary => _usingSystemBinary;

  Future<Directory> get _appSupportDir async {
    final override = _appSupportDirOverride;
    if (override != null) {
      return Directory(override);
    }
    return getApplicationSupportDirectory();
  }

  Future<String> get binaryPath async {
    if (_binaryPath != null) return _binaryPath!;
    if (_systemBinaryPath != null) return _systemBinaryPath!;
    final dir = await _appSupportDir;
    final name = Platform.isWindows
        ? '${AppConstants.cliBinaryName}.exe'
        : AppConstants.cliBinaryName;
    _binaryPath = '${dir.path}/$name';
    return _binaryPath!;
  }

  Future<String> get versionFilePath async {
    if (_versionFilePath != null) return _versionFilePath!;
    final dir = await _appSupportDir;
    _versionFilePath = '${dir.path}/.version';
    return _versionFilePath!;
  }

  Future<String> get _cachedUrlFilePath async {
    if (_urlCachePath != null) return _urlCachePath!;
    final dir = await _appSupportDir;
    _urlCachePath = '${dir.path}/${AppConstants.downloadUrlCacheFile}';
    return _urlCachePath!;
  }

  Future<bool> isBinaryInstalled() async {
    final path = await binaryPath;
    final file = File(path);
    return file.existsSync();
  }

  Future<String?> getInstalledVersion() async {
    final vPath = await versionFilePath;
    final file = File(vPath);
    if (!file.existsSync()) return null;
    try {
      return file.readAsStringSync().trim();
    } catch (e) {
      AppLogger.warning('Failed to read version file: $e');
      return null;
    }
  }

  Future<bool> isUpdateAvailable() async {
    try {
      final installed = await getInstalledVersion();
      final release = await _apiService.getLatestRelease();
      return installed != release.tagName;
    } catch (_) {
      return false;
    }
  }

  Future<bool> verifyBinary() async {
    final path = await binaryPath;
    final file = File(path);
    if (!file.existsSync()) return false;

    try {
      final result = await Process.run(
        path,
        [AppConstants.cliVersionArg],
        runInShell: true,
      );
      return result.exitCode == 0;
    } catch (_) {
      return false;
    }
  }

  Future<String?> _findBinaryInPath() async {
    final command = Platform.isWindows ? 'where' : 'which';
    try {
      final result = await Process.run(
        command,
        [AppConstants.cliBinaryName],
        runInShell: true,
      );
      if (result.exitCode == 0) {
        final stdout = (result.stdout as String).trim();
        if (stdout.isNotEmpty) {
          final lines = stdout.split('\n');
          final firstPath = lines.first.trim();
          if (firstPath.isNotEmpty) {
            AppLogger.info('Found keepalive in PATH: $firstPath');
            return firstPath;
          }
        }
      }
    } catch (e) {
      AppLogger.debug('$command failed: $e');
    }
    return null;
  }

  String? _findLocalBinary() {
    final binaryName = Platform.isWindows
        ? '${AppConstants.cliBinaryName}.exe'
        : AppConstants.cliBinaryName;

    final candidateDirs = <String>[];
    if (Platform.script.path.isNotEmpty) {
      final scriptDir = File(Platform.script.toFilePath()).parent;
      candidateDirs.add(scriptDir.path);
      candidateDirs.add('${scriptDir.path}/build');
      if (scriptDir.path.endsWith('/cmd/keepalive')) {
        candidateDirs.add(scriptDir.parent.parent.path);
      }
    }

    final envOverrides = Platform.environment;
    final keepaliveHome = envOverrides['KEEPALIVE_HOME'];
    if (keepaliveHome != null) {
      candidateDirs.add(keepaliveHome);
    }

    for (final dir in candidateDirs) {
      final path = '$dir/$binaryName';
      final file = File(path);
      if (file.existsSync()) {
        if (Platform.isWindows || _canExecute(file)) {
          AppLogger.info('Found local keepalive binary: $path');
          return path;
        }
      }
    }

    return null;
  }

  bool _canExecute(File file) {
    try {
      if (_hasExecutableBitSet(file)) return true;
      _setExecutableSync(file.path);
      _hasExecutableBitSet(file);
      return true;
    } catch (_) {
      return false;
    }
  }

  bool _hasExecutableBitSet(File file) {
    try {
      final stat = file.statSync();
      final mode = stat.modeString();
      return mode.contains('x');
    } catch (_) {
      return false;
    }
  }

  void _setExecutableSync(String path) {
    try {
      final result = Process.runSync('chmod', ['+x', path]);
      if (result.exitCode != 0) {
        AppLogger.warning('chmod +x failed: ${result.stderr}');
      }
    } catch (e) {
      AppLogger.warning('Failed to set executable bit: $e');
    }
  }

  Future<bool> _tryInstallViaHomebrew() async {
    if (Platform.isWindows) return false;
    try {
      final brewResult = await Process.run(
        'which',
        ['brew'],
        runInShell: true,
      );
      if (brewResult.exitCode != 0) {
        AppLogger.debug('Homebrew not found in PATH');
        return false;
      }

      AppLogger.info('Installing keepalive via Homebrew...');
      final tapResult = await Process.run(
        'brew',
        ['tap', AppConstants.homebrewTapRepo],
        runInShell: true,
      );
      if (tapResult.exitCode != 0) {
        AppLogger.warning(
            'brew tap failed (non-fatal): ${tapResult.stderr}');
      }

      final installResult = await Process.run(
        'brew',
        ['install', AppConstants.homebrewFormula],
        runInShell: true,
      );
      if (installResult.exitCode == 0) {
        final binaryPath = await _findBinaryInPath();
        if (binaryPath != null) {
          _systemBinaryPath = binaryPath;
          _usingSystemBinary = true;
          AppLogger.info('keepalive installed via Homebrew: $binaryPath');
          return true;
        }
      } else {
        AppLogger.warning(
            'brew install failed: ${installResult.stderr}');
      }
    } catch (e) {
      AppLogger.warning('Homebrew install failed: $e');
    }
    return false;
  }

  Future<bool> _tryInstallViaScoop() async {
    if (!Platform.isWindows) return false;
    try {
      final scoopResult = await Process.run(
        'powershell',
        ['-Command', 'Get-Command scoop -ErrorAction SilentlyContinue'],
        runInShell: true,
      );
      if (scoopResult.exitCode != 0) {
        AppLogger.debug('Scoop not found');
        return false;
      }

      AppLogger.info('Installing keepalive via Scoop...');
      final bucketResult = await Process.run(
        'scoop',
        [
          'bucket',
          'add',
          AppConstants.scoopBucketName,
          AppConstants.scoopBucketUrl
        ],
        runInShell: true,
      );
      if (bucketResult.exitCode != 0) {
        AppLogger.warning(
            'scoop bucket add failed (non-fatal): ${bucketResult.stderr}');
      }

      final installResult = await Process.run(
        'scoop',
        ['install', AppConstants.scoopPackage],
        runInShell: true,
      );
      if (installResult.exitCode == 0) {
        final binaryPath = await _findBinaryInPath();
        if (binaryPath != null) {
          _systemBinaryPath = binaryPath;
          _usingSystemBinary = true;
          AppLogger.info('keepalive installed via Scoop: $binaryPath');
          return true;
        }
      } else {
        AppLogger.warning(
            'scoop install failed: ${installResult.stderr}');
      }
    } catch (e) {
      AppLogger.warning('Scoop install failed: $e');
    }
    return false;
  }

  Future<void> ensureCliInstalled({
    void Function(double progress)? onProgress,
  }) async {
    final installed = await isBinaryInstalled();
    final version = await getInstalledVersion();

    if (installed && version != null) {
      final binaryOk = await verifyBinary();
      if (binaryOk) {
        AppLogger.info('CLI binary already installed: $version');
        return;
      }
      AppLogger.warning(
        'Installed binary failed verification, re-downloading',
      );
    }

    final pathBinary = await _findBinaryInPath();
    if (pathBinary != null) {
      final verified = await _verifyBinaryAt(pathBinary);
      if (verified) {
        _systemBinaryPath = pathBinary;
        _usingSystemBinary = true;
        AppLogger.info('Using keepalive from PATH: $pathBinary');
        return;
      }
      AppLogger.warning(
        'keepalive found in PATH at $pathBinary but verification failed',
      );
    }

    final localBinary = _findLocalBinary();
    if (localBinary != null) {
      final verified = await _verifyBinaryAt(localBinary);
      if (verified) {
        _systemBinaryPath = localBinary;
        _usingSystemBinary = true;
        AppLogger.info('Using keepalive from local filesystem: $localBinary');
        return;
      }
      AppLogger.warning(
        'keepalive found locally at $localBinary but verification failed',
      );
    }

    if (PlatformUtils.isMacOS || PlatformUtils.isLinux) {
      final brewInstalled = await _tryInstallViaHomebrew();
      if (brewInstalled) return;
    }

    if (PlatformUtils.isWindows) {
      final scoopInstalled = await _tryInstallViaScoop();
      if (scoopInstalled) return;
    }

    await _downloadAndInstall(onProgress: onProgress);
  }

  Future<bool> _verifyBinaryAt(String path) async {
    final file = File(path);
    if (!file.existsSync()) return false;
    try {
      final result = await Process.run(
        path,
        [AppConstants.cliVersionArg],
        runInShell: true,
      );
      return result.exitCode == 0;
    } catch (_) {
      return false;
    }
  }

  Future<void> downloadLatest({
    void Function(double progress)? onProgress,
  }) async {
    await _downloadAndInstall(onProgress: onProgress);
  }

  Future<String?> _getCachedDownloadUrl() async {
    final path = await _cachedUrlFilePath;
    final file = File(path);
    if (!file.existsSync()) return null;
    try {
      final url = file.readAsStringSync().trim();
      if (url.isNotEmpty) return url;
    } catch (_) {}
    return null;
  }

  Future<void> _cacheDownloadUrl(String url) async {
    try {
      final path = await _cachedUrlFilePath;
      await File(path).writeAsString(url);
    } catch (e) {
      AppLogger.warning('Failed to cache download URL: $e');
    }
  }

  Future<String> _resolveAssetUrl() async {
    try {
      final release = await _apiService.getLatestRelease();
      final assetUrl = _apiService.findPlatformAssetUrl(release);
      if (assetUrl == null) {
        final assetName = _apiService.getAssetNameForCurrentPlatform();
        throw DownloadException(
            'No binary available for current platform ($assetName)');
      }
      await _cacheDownloadUrl(assetUrl);
      return assetUrl;
    } on DownloadException {
      rethrow;
    } catch (e) {
      final cached = await _getCachedDownloadUrl();
      if (cached != null) {
        AppLogger.warning(
          'Failed to fetch latest release ($e), using cached download URL',
        );
        return cached;
      }
      throw DownloadException(
        'Failed to resolve download URL and no cached URL available: $e',
        underlying: e,
      );
    }
  }

  Future<void> _downloadWithRetry(
    String url,
    String archivePath,
    void Function(double progress)? onProgress,
  ) async {
    int attempt = 0;

    while (true) {
      attempt++;
      try {
        AppLogger.info(
            'Download attempt $attempt/${AppConstants.downloadMaxRetries}: $url');
        await _dio.download(
          url,
          archivePath,
          onReceiveProgress: (received, total) {
            if (total > 0 && onProgress != null) {
              onProgress(received / total);
            }
          },
        );
        return;
      } on DioException catch (e) {
        if (attempt >= AppConstants.downloadMaxRetries) {
          throw DownloadException(
            'Download failed after $attempt attempts: $e',
            underlying: e,
          );
        }
        final delayMs =
            AppConstants.downloadRetryBaseDelayMs * (1 << (attempt - 1));
        AppLogger.warning(
          'Download attempt $attempt failed, retrying in ${delayMs}ms: $e',
        );
        await Future<void>.delayed(Duration(milliseconds: delayMs));
      }
    }
  }

  Future<void> _downloadAndInstall({
    void Function(double progress)? onProgress,
  }) async {
    final assetUrl = await _resolveAssetUrl();
    final assetName = _apiService.getAssetNameForCurrentPlatform();

    final tempDir = await Directory.systemTemp.createTemp('keepalive_dl_');
    final archivePath = '${tempDir.path}/$assetName';

    try {
      await _downloadWithRetry(assetUrl, archivePath, onProgress);

      AppLogger.info('Extracting $assetName');
      final extractDir = '${tempDir.path}/extract';
      await Directory(extractDir).create();

      archive_io.extractFileToDisk(archivePath, extractDir);

      final targetPath = await binaryPath;
      final extractedBinary = _findBinaryInDir(extractDir);
      if (extractedBinary == null) {
        throw const DownloadException('Binary not found in extracted archive');
      }

      final targetFile = File(targetPath);
      if (targetFile.existsSync()) {
        await targetFile.delete();
      }
      await File(extractedBinary).copy(targetPath);

      if (!Platform.isWindows) {
        await _setExecutable(targetPath);
      }

      final release = await _resolveReleaseTagForVersion();
      await _writeVersionFile(release);

      AppLogger.info('CLI $release installed to $targetPath');
    } on DownloadException {
      rethrow;
    } catch (e) {
      throw DownloadException('Failed to install CLI: $e', underlying: e);
    } finally {
      try {
        await tempDir.delete(recursive: true);
      } catch (e) {
        AppLogger.warning('Failed to clean up temp dir: $e');
      }
    }
  }

  Future<String> _resolveReleaseTagForVersion() async {
    try {
      final release = await _apiService.getLatestRelease();
      return release.tagName;
    } catch (_) {
      return 'unknown';
    }
  }

  String? _findBinaryInDir(String dir) {
    final directory = Directory(dir);
    if (!directory.existsSync()) return null;

    final binaryName = Platform.isWindows
        ? '${AppConstants.cliBinaryName}.exe'
        : AppConstants.cliBinaryName;

    for (final entity in directory.listSync(recursive: true)) {
      if (entity is File) {
        final name = entity.uri.pathSegments.last;
        if (name == binaryName) {
          return entity.path;
        }
      }
    }
    return null;
  }

  Future<void> _setExecutable(String path) async {
    try {
      final result = await Process.run('chmod', ['+x', path]);
      if (result.exitCode != 0) {
        AppLogger.warning('chmod +x failed: ${result.stderr}');
      }
    } catch (e) {
      AppLogger.warning('Failed to set executable bit: $e');
    }
  }

  Future<void> _writeVersionFile(String version) async {
    final vPath = await versionFilePath;
    await File(vPath).writeAsString('$version\n');
  }
}
