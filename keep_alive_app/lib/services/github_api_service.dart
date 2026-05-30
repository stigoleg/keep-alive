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
    return '${AppConstants.cliBinaryName}_${osName}_$arch.$ext';
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
}
