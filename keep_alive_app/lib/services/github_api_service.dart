import 'dart:ffi';

import 'package:dio/dio.dart';

import '../core/constants.dart';
import '../core/exceptions.dart';
import '../models/github_release.dart';

class GitHubApiService {
  final Dio _dio;
  final String _baseUrl;
  final String _releasesPath;

  GitHubApiService({
    required this._dio,
    this._baseUrl = AppConstants.githubApiBaseUrl,
    this._releasesPath = AppConstants.githubReleasesPath,
  });

  Future<GitHubRelease> getLatestRelease() async {
    try {
      final response = await _dio.get('$_baseUrl$_releasesPath/latest');
      return GitHubRelease.fromJson(response.data as Map<String, dynamic>);
    } on DioException catch (e) {
      throw DownloadException(
        'Failed to fetch latest release: ${e.message}',
        underlying: e,
      );
    }
  }

  String getAssetNameForCurrentPlatform() {
    final os = Abi.current();

    String osName;
    String arch;

    switch (os) {
      case Abi.macosArm64:
        osName = 'Darwin';
        arch = 'arm64';
      case Abi.macosX64:
        osName = 'Darwin';
        arch = 'x86_64';
      case Abi.linuxArm64:
        osName = 'Linux';
        arch = 'arm64';
      case Abi.linuxX64:
        osName = 'Linux';
        arch = 'x86_64';
      case Abi.windowsArm64:
        osName = 'Windows';
        arch = 'arm64';
      case Abi.windowsX64:
        osName = 'Windows';
        arch = 'x86_64';
      default:
        throw PlatformException('Unsupported platform: $os');
    }

    final ext = osName == 'Windows' ? 'zip' : 'tar.gz';
    return '${AppConstants.cliReleaseBaseName}_${osName}_$arch.$ext';
  }

  String? findPlatformAssetUrl(GitHubRelease release) {
    final assetName = getAssetNameForCurrentPlatform();
    for (final asset in release.assets) {
      if (asset.name == assetName) {
        return asset.downloadUrl;
      }
    }
    return null;
  }

  /// Locates the GoReleaser-published `*_checksums.txt` (case-insensitive) so
  /// the downloader can verify the SHA256 of the platform archive before
  /// extracting it. Returns null when no checksum asset is present so the
  /// caller can decide to fail-closed or proceed (we proceed with a warning
  /// today — see [CliDownloadService]).
  String? findChecksumAssetUrl(GitHubRelease release) {
    for (final asset in release.assets) {
      final lower = asset.name.toLowerCase();
      if (lower.endsWith('checksums.txt') || lower == 'sha256sums.txt') {
        return asset.downloadUrl;
      }
    }
    return null;
  }
}
