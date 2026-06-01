import 'dart:async';
import 'dart:io';

import 'package:archive/archive_io.dart' as archive_io;
import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

import '../core/constants.dart';
import '../core/exceptions.dart';
import '../core/logger.dart';
import '../platform/platform_interface.dart';
import '../utils/platform_utils.dart';
import '../utils/version_utils.dart';
import 'github_api_service.dart';

/// Resolves a platform-provided bundled-CLI path. Defaults to the host
/// platform channel; tests inject a custom lookup.
typedef BundledCliLookup = Future<String?> Function();

typedef ProcessRunner =
    Future<ProcessResult> Function(
      String executable,
      List<String> arguments, {
      bool runInShell,
    });

enum CliInstallSource { bundled, local, appManaged, path, homebrew, scoop }

class CliDownloadService {
  final GitHubApiService _apiService;
  final Dio _dio;
  final String? _appSupportDirOverride;
  final BundledCliLookup _bundledCliLookup;
  final ProcessRunner _processRunner;

  String? _binaryPath;
  String? _versionFilePath;
  String? _urlCachePath;
  bool _usingSystemBinary = false;
  String? _systemBinaryPath;
  CliInstallSource? _installSource;

  CliDownloadService({
    required this._apiService,
    required this._dio,
    String? appSupportDir,
    BundledCliLookup? bundledCliLookup,
    ProcessRunner? processRunner,
  }) : _appSupportDirOverride = appSupportDir,
       _bundledCliLookup = bundledCliLookup ?? _defaultBundledCliLookup,
       _processRunner = processRunner ?? _defaultProcessRunner;

  static Future<String?> _defaultBundledCliLookup() =>
      KeepAlivePlatform.instance.getBundledCliPath();

  static Future<ProcessResult> _defaultProcessRunner(
    String executable,
    List<String> arguments, {
    bool runInShell = false,
  }) => Process.run(executable, arguments, runInShell: runInShell);

  bool get isUsingSystemBinary => _usingSystemBinary;

  CliInstallSource? get installSource => _installSource;

  bool get isUsingPackageManager =>
      _installSource == CliInstallSource.homebrew ||
      _installSource == CliInstallSource.scoop;

  Future<Directory> get _appSupportDir async {
    final override = _appSupportDirOverride;
    if (override != null) {
      return Directory(override);
    }
    return getApplicationSupportDirectory();
  }

  Future<String> get binaryPath async {
    if (_systemBinaryPath != null) return _systemBinaryPath!;
    if (_binaryPath != null) return _binaryPath!;
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
    if (file.existsSync()) {
      try {
        return file.readAsStringSync().trim();
      } catch (e) {
        AppLogger.warning('Failed to read version file: $e');
      }
    }

    final path = await binaryPath;
    if (path.isNotEmpty) {
      return _parseVersionFromBinary(path);
    }

    return null;
  }

  Future<String?> getSystemBinaryVersion(String path) =>
      _parseVersionFromBinary(path);

  Future<String?> _parseVersionFromBinary(String path) async {
    final file = File(path);
    if (!file.existsSync()) return null;

    try {
      final result = await _processRunner(path, [
        AppConstants.cliVersionArg,
      ], runInShell: true);
      if (result.exitCode == 0) {
        final output = (result.stdout as String).trim();
        final regex = RegExp(r'(\d+\.\d+\.\d+)');
        final match = regex.firstMatch(output);
        if (match != null) {
          return 'v${match.group(1)}';
        }
        AppLogger.debug('Could not parse version from output: $output');
      }
    } catch (e) {
      AppLogger.warning('Failed to query version from binary: $e');
    }
    return null;
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
      final result = await _processRunner(path, [
        AppConstants.cliVersionArg,
      ], runInShell: true);
      return result.exitCode == 0;
    } catch (_) {
      return false;
    }
  }

  Future<String?> _findBinaryInPath() async {
    final command = Platform.isWindows ? 'where' : 'which';
    try {
      final result = await _processRunner(command, [
        AppConstants.cliBinaryName,
      ], runInShell: true);
      if (result.exitCode == 0) {
        final stdout = (result.stdout as String).trim();
        if (stdout.isNotEmpty) {
          final lines = stdout.split('\n');
          final firstPath = lines.first.trim();
          if (firstPath.isNotEmpty) {
            AppLogger.info(
              'Found keepalive in PATH: ${AppLogger.scrubPath(firstPath)}',
            );
            return firstPath;
          }
        }
      }
    } catch (e) {
      AppLogger.debug('$command failed: $e');
    }
    return null;
  }

  Future<String?> _findLocalBinary() async {
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
        if (Platform.isWindows || await _canExecute(file)) {
          AppLogger.info(
            'Found local keepalive binary: ${AppLogger.scrubPath(path)}',
          );
          return path;
        }
      }
    }

    return null;
  }

  Future<bool> _canExecute(File file) async {
    try {
      if (_hasExecutableBitSet(file)) return true;
      await _setExecutable(file.path);
      return _hasExecutableBitSet(file);
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

  Future<String?> _findHomebrewExecutable() async {
    if (Platform.isWindows) return null;

    try {
      final result = await _processRunner('which', ['brew'], runInShell: true);
      if (result.exitCode == 0) {
        final path = (result.stdout as String).trim().split('\n').first.trim();
        if (path.isNotEmpty) return path;
      }
    } catch (e) {
      AppLogger.debug('which brew failed: $e');
    }

    for (final path in const [
      '/opt/homebrew/bin/brew',
      '/usr/local/bin/brew',
      '/home/linuxbrew/.linuxbrew/bin/brew',
    ]) {
      if (File(path).existsSync()) return path;
    }

    return null;
  }

  Future<bool> _isHomebrewFormulaInstalled(String brew) async {
    final result = await _processRunner(brew, [
      'list',
      '--formula',
      AppConstants.homebrewFormula,
    ], runInShell: true);
    return result.exitCode == 0;
  }

  Future<String?> _findHomebrewBinary(String brew) async {
    try {
      final prefixResult = await _processRunner(brew, [
        '--prefix',
        AppConstants.homebrewFormula,
      ], runInShell: true);
      if (prefixResult.exitCode == 0) {
        final prefix = (prefixResult.stdout as String)
            .trim()
            .split('\n')
            .first
            .trim();
        final candidate = p.join(prefix, 'bin', AppConstants.cliBinaryName);
        if (File(candidate).existsSync()) return candidate;
      }
    } catch (e) {
      AppLogger.debug('brew --prefix failed: $e');
    }
    return _findBinaryInPath();
  }

  Future<bool> _adoptExistingHomebrewInstall({
    bool requireMinVersion = true,
  }) async {
    final brew = await _findHomebrewExecutable();
    if (brew == null) {
      AppLogger.debug('Homebrew not found');
      return false;
    }

    try {
      if (!await _isHomebrewFormulaInstalled(brew)) return false;
      final binaryPath = await _findHomebrewBinary(brew);
      if (binaryPath == null) return false;
      return _adoptBinary(
        binaryPath,
        'Homebrew',
        installSource: CliInstallSource.homebrew,
        requireMinVersion: requireMinVersion,
      );
    } catch (e) {
      AppLogger.warning('Homebrew detection failed: $e');
      return false;
    }
  }

  Future<bool> _tryInstallViaHomebrew() async {
    if (Platform.isWindows) return false;
    try {
      final brew = await _findHomebrewExecutable();
      if (brew == null) {
        AppLogger.debug('Homebrew not found in PATH');
        return false;
      }

      if (await _isHomebrewFormulaInstalled(brew)) {
        if (await _adoptExistingHomebrewInstall()) {
          return true;
        }
        final upgraded = await _upgradeHomebrew(brew);
        if (upgraded) return true;
        return false;
      }

      AppLogger.info('Installing keepalive via Homebrew...');
      final tapResult = await _runWithTimeout(brew, [
        'tap',
        AppConstants.homebrewTapRepo,
      ]);
      if (tapResult == null) {
        AppLogger.warning('brew tap timed out (non-fatal)');
      } else if (tapResult.exitCode != 0) {
        AppLogger.warning('brew tap failed (non-fatal): ${tapResult.stderr}');
      }

      final installResult = await _runWithTimeout(brew, [
        'install',
        AppConstants.homebrewFormula,
      ]);
      if (installResult == null) {
        AppLogger.warning(
          'brew install timed out after ${AppConstants.packageManagerInstallTimeoutSeconds}s, falling back to direct download',
        );
        return false;
      }
      if (installResult.exitCode == 0) {
        final binaryPath = await _findHomebrewBinary(brew);
        if (binaryPath != null &&
            await _adoptBinary(
              binaryPath,
              'Homebrew',
              installSource: CliInstallSource.homebrew,
              requireMinVersion: true,
            )) {
          return true;
        }
      } else {
        AppLogger.warning('brew install failed: ${installResult.stderr}');
      }
    } catch (e) {
      AppLogger.warning('Homebrew install failed: $e');
    }
    return false;
  }

  Future<bool> _upgradeHomebrew(String brew) async {
    AppLogger.info('Updating keepalive via Homebrew...');
    final tapResult = await _runWithTimeout(brew, [
      'tap',
      AppConstants.homebrewTapRepo,
    ]);
    if (tapResult == null) {
      AppLogger.warning('brew tap timed out (non-fatal)');
    } else if (tapResult.exitCode != 0) {
      AppLogger.warning('brew tap failed (non-fatal): ${tapResult.stderr}');
    }

    final result = await _runWithTimeout(brew, [
      'upgrade',
      AppConstants.homebrewFormula,
    ]);
    if (result == null) {
      throw const DownloadException(
        'brew upgrade timed out after '
        '${AppConstants.packageManagerInstallTimeoutSeconds}s',
      );
    }
    if (result.exitCode != 0) {
      throw DownloadException('brew upgrade failed: ${result.stderr}');
    }

    final binaryPath = await _findHomebrewBinary(brew);
    if (binaryPath == null ||
        !await _adoptBinary(
          binaryPath,
          'Homebrew',
          installSource: CliInstallSource.homebrew,
          requireMinVersion: true,
        )) {
      throw const DownloadException(
        'Homebrew updated but keepalive was not usable',
      );
    }
    return true;
  }

  Future<ProcessResult?> _runWithTimeout(
    String executable,
    List<String> args,
  ) async {
    try {
      return await _processRunner(executable, args, runInShell: true).timeout(
        const Duration(
          seconds: AppConstants.packageManagerInstallTimeoutSeconds,
        ),
      );
    } on TimeoutException {
      return null;
    }
  }

  Future<bool> _tryInstallViaScoop() async {
    if (!Platform.isWindows) return false;
    try {
      if (!await _isScoopAvailable()) {
        AppLogger.debug('Scoop not found');
        return false;
      }

      if (await _isScoopPackageInstalled()) {
        final binaryPath = await _findScoopBinary();
        if (binaryPath != null &&
            await _adoptBinary(
              binaryPath,
              'Scoop',
              installSource: CliInstallSource.scoop,
              requireMinVersion: true,
            )) {
          return true;
        }
        final updated = await _updateScoop();
        if (updated) return true;
        return false;
      }

      AppLogger.info('Installing keepalive via Scoop...');
      final bucketResult = await _runWithTimeout('scoop', [
        'bucket',
        'add',
        AppConstants.scoopBucketName,
        AppConstants.scoopBucketUrl,
      ]);
      if (bucketResult == null) {
        AppLogger.warning('scoop bucket add timed out (non-fatal)');
      } else if (bucketResult.exitCode != 0) {
        AppLogger.warning(
          'scoop bucket add failed (non-fatal): ${bucketResult.stderr}',
        );
      }

      final installResult = await _runWithTimeout('scoop', [
        'install',
        AppConstants.scoopPackage,
      ]);
      if (installResult == null) {
        AppLogger.warning(
          'scoop install timed out after ${AppConstants.packageManagerInstallTimeoutSeconds}s, falling back to direct download',
        );
        return false;
      }
      if (installResult.exitCode == 0) {
        final binaryPath = await _findScoopBinary();
        if (binaryPath != null &&
            await _adoptBinary(
              binaryPath,
              'Scoop',
              installSource: CliInstallSource.scoop,
              requireMinVersion: true,
            )) {
          return true;
        }
      } else {
        AppLogger.warning('scoop install failed: ${installResult.stderr}');
      }
    } catch (e) {
      AppLogger.warning('Scoop install failed: $e');
    }
    return false;
  }

  Future<bool> _isScoopAvailable() async {
    final scoopResult = await _processRunner('powershell', [
      '-Command',
      'Get-Command scoop -ErrorAction SilentlyContinue',
    ], runInShell: true);
    return scoopResult.exitCode == 0;
  }

  Future<bool> _isScoopPackageInstalled() async {
    final result = await _processRunner('scoop', [
      'prefix',
      AppConstants.scoopPackage,
    ], runInShell: true);
    return result.exitCode == 0;
  }

  Future<String?> _findScoopBinary() async {
    try {
      final prefixResult = await _processRunner('scoop', [
        'prefix',
        AppConstants.scoopPackage,
      ], runInShell: true);
      if (prefixResult.exitCode == 0) {
        final prefix = (prefixResult.stdout as String)
            .trim()
            .split('\n')
            .first
            .trim();
        final candidate = p.join(prefix, '${AppConstants.cliBinaryName}.exe');
        if (File(candidate).existsSync()) return candidate;
      }
    } catch (e) {
      AppLogger.debug('scoop prefix failed: $e');
    }
    return _findBinaryInPath();
  }

  Future<bool> _updateScoop() async {
    AppLogger.info('Updating keepalive via Scoop...');
    final bucketResult = await _runWithTimeout('scoop', [
      'bucket',
      'add',
      AppConstants.scoopBucketName,
      AppConstants.scoopBucketUrl,
    ]);
    if (bucketResult == null) {
      AppLogger.warning('scoop bucket add timed out (non-fatal)');
    } else if (bucketResult.exitCode != 0) {
      AppLogger.warning(
        'scoop bucket add failed (non-fatal): ${bucketResult.stderr}',
      );
    }

    final result = await _runWithTimeout('scoop', [
      'update',
      AppConstants.scoopPackage,
    ]);
    if (result == null) {
      throw const DownloadException(
        'scoop update timed out after '
        '${AppConstants.packageManagerInstallTimeoutSeconds}s',
      );
    }
    if (result.exitCode != 0) {
      throw DownloadException('scoop update failed: ${result.stderr}');
    }

    final binaryPath = await _findScoopBinary();
    if (binaryPath == null ||
        !await _adoptBinary(
          binaryPath,
          'Scoop',
          installSource: CliInstallSource.scoop,
          requireMinVersion: true,
        )) {
      throw const DownloadException(
        'Scoop updated but keepalive was not usable',
      );
    }
    return true;
  }

  Future<void> ensureCliInstalled({
    void Function(double progress)? onProgress,
  }) async {
    // 1. Prefer the CLI bundled inside the host app — it matches the GUI's
    //    build and avoids stale PATH installs (e.g. an older Homebrew copy).
    final bundled = await _findBundledBinary();
    if (bundled != null &&
        await _adoptBinary(
          bundled,
          'bundled app resource',
          installSource: CliInstallSource.bundled,
        )) {
      return;
    }

    // 2. Explicit dev override: KEEPALIVE_HOME or a sibling `keepalive` next
    //    to the running Dart entrypoint. Useful during local development.
    final localBinary = await _findLocalBinary();
    if (localBinary != null &&
        await _adoptBinary(
          localBinary,
          'local filesystem',
          installSource: CliInstallSource.local,
        )) {
      return;
    }

    // 3. App-managed binary previously downloaded into Application Support.
    final installed = await isBinaryInstalled();
    if (installed) {
      final managedPath = await binaryPath;
      final managedVersion = await getInstalledVersion();
      if (await _verifyBinaryAt(managedPath) && _meetsMinimum(managedVersion)) {
        _installSource = CliInstallSource.appManaged;
        AppLogger.info(
          'Using app-managed CLI: ${AppLogger.scrubPath(managedPath)} ($managedVersion)',
        );
        return;
      }
      AppLogger.warning(
        'App-managed CLI at ${AppLogger.scrubPath(managedPath)} '
        'failed verification or below minimum ${AppConstants.minimumCliVersion}, re-downloading',
      );
    }

    // 4. Existing package-manager installs. Tracking this source lets the
    //    update button keep using the same package manager later.
    if (PlatformUtils.isMacOS || PlatformUtils.isLinux) {
      final brewInstalled = await _adoptExistingHomebrewInstall();
      if (brewInstalled) return;
    }

    if (PlatformUtils.isWindows &&
        await _isScoopAvailable() &&
        await _isScoopPackageInstalled()) {
      final scoopBinary = await _findScoopBinary();
      if (scoopBinary != null &&
          await _adoptBinary(
            scoopBinary,
            'Scoop',
            installSource: CliInstallSource.scoop,
            requireMinVersion: true,
          )) {
        return;
      }
    }

    // 5. Generic system PATH (manual installs). Only used as fallback so a
    //    stale system install cannot mask the fixed CLI.
    final pathBinary = await _findBinaryInPath();
    if (pathBinary != null &&
        await _adoptBinary(
          pathBinary,
          'PATH',
          installSource: CliInstallSource.path,
          requireMinVersion: true,
        )) {
      return;
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

  Future<String?> _findBundledBinary() async {
    try {
      final path = await _bundledCliLookup();
      if (path == null || path.isEmpty) return null;
      final file = File(path);
      if (!file.existsSync()) return null;
      AppLogger.info('Found bundled keepalive: ${AppLogger.scrubPath(path)}');
      return path;
    } catch (e) {
      AppLogger.debug('Bundled CLI lookup failed: $e');
      return null;
    }
  }

  /// Verifies [path], parses its version, optionally enforces the minimum
  /// supported version, and records it as the active system binary on success.
  Future<bool> _adoptBinary(
    String path,
    String source, {
    required CliInstallSource installSource,
    bool requireMinVersion = false,
  }) async {
    final verified = await _verifyBinaryAt(path);
    if (!verified) {
      AppLogger.warning(
        'keepalive from $source at ${AppLogger.scrubPath(path)} failed verification',
      );
      return false;
    }

    final version = await _parseVersionFromBinary(path);
    if (requireMinVersion && !_meetsMinimum(version)) {
      AppLogger.warning(
        'Ignoring $source keepalive at ${AppLogger.scrubPath(path)}: '
        'version ${version ?? "unknown"} is older than required '
        '${AppConstants.minimumCliVersion}',
      );
      return false;
    }

    _systemBinaryPath = path;
    _usingSystemBinary = true;
    _installSource = installSource;
    AppLogger.info(
      'Using keepalive from $source: ${AppLogger.scrubPath(path)} (${version ?? "version unknown"})',
    );
    return true;
  }

  bool _meetsMinimum(String? version) =>
      VersionUtils.meetsMinimum(version, AppConstants.minimumCliVersion);

  @visibleForTesting
  Future<bool> tryAdoptForTest(String path, {bool requireMin = false}) =>
      _adoptBinary(
        path,
        'test',
        installSource: CliInstallSource.path,
        requireMinVersion: requireMin,
      );

  Future<bool> _verifyBinaryAt(String path) async {
    final file = File(path);
    if (!file.existsSync()) return false;
    try {
      final result = await _processRunner(path, [
        AppConstants.cliVersionArg,
      ], runInShell: true);
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

  Future<void> updateLatest({
    void Function(double progress)? onProgress,
  }) async {
    if (PlatformUtils.isMacOS || PlatformUtils.isLinux) {
      final brew = await _findHomebrewExecutable();
      if (brew != null && await _isHomebrewFormulaInstalled(brew)) {
        await _upgradeHomebrew(brew);
        return;
      }
    }

    if (PlatformUtils.isWindows &&
        await _isScoopAvailable() &&
        await _isScoopPackageInstalled()) {
      await _updateScoop();
      return;
    }

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
          'No binary available for current platform ($assetName)',
        );
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
          'Download attempt $attempt/${AppConstants.downloadMaxRetries}: $url',
        );
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

      await _verifyArchiveChecksum(archivePath, assetName);

      AppLogger.info('Extracting $assetName');
      final extractDir = '${tempDir.path}/extract';
      await Directory(extractDir).create();

      _assertSafeArchiveEntries(archivePath, assetName, extractDir);
      await archive_io.extractFileToDisk(archivePath, extractDir);

      final targetPath = await binaryPath;
      final extractedBinary = _findBinaryInDir(extractDir);
      if (extractedBinary == null) {
        throw DownloadException(
          'Binary not found in extracted archive ($assetName)',
        );
      }

      final targetFile = File(targetPath);
      final targetParent = targetFile.parent;
      if (!targetParent.existsSync()) {
        await targetParent.create(recursive: true);
      }
      if (targetFile.existsSync()) {
        await targetFile.delete();
      }
      await File(extractedBinary).copy(targetPath);

      if (!Platform.isWindows) {
        await _setExecutable(targetPath);
      }

      final release = await _resolveReleaseTagForVersion();
      await _writeVersionFile(release);
      _systemBinaryPath = null;
      _usingSystemBinary = false;
      _installSource = CliInstallSource.appManaged;

      AppLogger.info(
        'CLI $release installed to ${AppLogger.scrubPath(targetPath)}',
      );
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
    if (!directory.existsSync()) {
      AppLogger.warning('Extract directory does not exist: $dir');
      return null;
    }

    final primaryName = Platform.isWindows
        ? '${AppConstants.cliBinaryName}.exe'
        : AppConstants.cliBinaryName;

    final alternateNames = <String>{
      primaryName,
      if (Platform.isWindows) AppConstants.cliBinaryName,
      AppConstants.cliReleaseBaseName,
    };

    AppLogger.debug(
      'Searching for binary in $dir (looking for: ${alternateNames.join(', ')})',
    );

    for (final entity in directory.listSync(recursive: true)) {
      if (entity is File) {
        final name = entity.path.split(Platform.pathSeparator).last;
        AppLogger.debug('  found file: $name');
        if (alternateNames.contains(name)) {
          AppLogger.info('Found binary in archive: ${entity.path}');
          return entity.path;
        }
      }
    }

    AppLogger.warning('Binary not found in $dir. Files present:');
    try {
      for (final entity in directory.listSync(recursive: false)) {
        AppLogger.warning('  ${entity.path}');
      }
    } catch (_) {}

    return null;
  }

  Future<void> _setExecutable(String path) async {
    try {
      final result = await _processRunner('chmod', ['+x', path]);
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

  /// Fetches the GoReleaser-published checksums file for the current release
  /// and verifies the SHA256 of the downloaded archive against it. Missing
  /// checksums file is treated as a warning, not a hard failure, to keep
  /// install flowing when the release pipeline lags behind the API; a
  /// mismatch always fails closed.
  Future<void> _verifyArchiveChecksum(
    String archivePath,
    String assetName,
  ) async {
    String? checksumUrl;
    try {
      final release = await _apiService.getLatestRelease();
      checksumUrl = _apiService.findChecksumAssetUrl(release);
    } catch (e) {
      AppLogger.warning(
        'Could not look up release checksums asset ($e); skipping verify',
      );
      return;
    }
    if (checksumUrl == null) {
      AppLogger.warning(
        'Release has no checksums.txt asset; skipping integrity check',
      );
      return;
    }

    String body;
    try {
      final response = await _dio.get<String>(
        checksumUrl,
        options: Options(
          responseType: ResponseType.plain,
          headers: const {'Accept': 'text/plain'},
        ),
      );
      body = response.data ?? '';
    } catch (e) {
      AppLogger.warning(
        'Failed to fetch checksums.txt ($e); skipping integrity check',
      );
      return;
    }

    final expected = _parseChecksumFor(body, assetName);
    if (expected == null) {
      AppLogger.warning(
        'checksums.txt does not contain an entry for $assetName; skipping',
      );
      return;
    }

    final bytes = await File(archivePath).readAsBytes();
    final actual = sha256.convert(bytes).toString();
    if (actual.toLowerCase() != expected.toLowerCase()) {
      throw DownloadException(
        'Checksum mismatch for $assetName '
        '(expected $expected, actual $actual)',
      );
    }
    AppLogger.info('Verified SHA256 for $assetName');
  }

  static String? _parseChecksumFor(String body, String assetName) {
    for (final raw in body.split('\n')) {
      final line = raw.trim();
      if (line.isEmpty || line.startsWith('#')) continue;
      // GoReleaser default format: `<hex>  <filename>`. We accept any
      // whitespace separator (`  `, single space, tab).
      final parts = line.split(RegExp(r'\s+'));
      if (parts.length < 2) continue;
      final hash = parts.first.trim();
      final name = parts.skip(1).join(' ').trim();
      if (name == assetName ||
          name == './$assetName' ||
          name == '*$assetName') {
        return hash;
      }
    }
    return null;
  }

  /// Pre-scans the archive entries and rejects any whose extracted path
  /// would escape [extractDir] (zip-slip / tarbomb). Runs before extraction
  /// so a malicious archive never touches disk.
  void _assertSafeArchiveEntries(
    String archivePath,
    String assetName,
    String extractDir,
  ) {
    final isZip = assetName.toLowerCase().endsWith('.zip');
    final bytes = File(archivePath).readAsBytesSync();
    final archive = isZip
        ? archive_io.ZipDecoder().decodeBytes(bytes)
        : archive_io.TarDecoder().decodeBytes(
            const archive_io.GZipDecoder().decodeBytes(bytes),
          );
    final root = p.canonicalize(extractDir);
    for (final entry in archive.files) {
      final dest = p.canonicalize(p.join(extractDir, entry.name));
      if (dest != root && !p.isWithin(root, dest)) {
        throw DownloadException('Unsafe archive entry: ${entry.name}');
      }
    }
  }
}
