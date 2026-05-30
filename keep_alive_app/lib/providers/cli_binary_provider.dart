import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/logger.dart';
import '../models/download_state.dart';
import '../services/cli_download_service.dart';
import '../services/github_api_service.dart';

final cliBinaryProvider =
    NotifierProvider<CliBinaryNotifier, DownloadState>(
  CliBinaryNotifier.new,
);

class CliBinaryNotifier extends Notifier<DownloadState> {
  late final GitHubApiService _apiService;
  late final CliDownloadService _downloadService;

  @override
  DownloadState build() {
    _apiService = GitHubApiService(dio: Dio());
    _downloadService = CliDownloadService(
      apiService: _apiService,
      dio: Dio(),
    );
    return const DownloadState();
  }

  Future<void> checkAndInstall() async {
    try {
      state = state.copyWith(
        status: DownloadStatus.downloading,
        progress: 0.0,
        errorMessage: null,
      );

      await _downloadService.ensureCliInstalled(
        onProgress: (progress) {
          state = state.copyWith(progress: progress);
        },
      );

      final version = await _downloadService.getInstalledVersion();
      state = DownloadState(
        status: DownloadStatus.installed,
        installedVersion: version,
        latestVersion: version,
      );

      AppLogger.info('CLI binary ready: $version');
    } catch (e) {
      AppLogger.error('Failed to install CLI binary', e);
      state = state.copyWith(
        status: DownloadStatus.error,
        errorMessage: e.toString(),
      );
    }
  }

  Future<void> downloadLatest() async {
    try {
      state = state.copyWith(
        status: DownloadStatus.downloading,
        progress: 0.0,
        errorMessage: null,
      );

      await _downloadService.downloadLatest(
        onProgress: (progress) {
          state = state.copyWith(progress: progress);
        },
      );

      final version = await _downloadService.getInstalledVersion();
      state = DownloadState(
        status: DownloadStatus.installed,
        installedVersion: version,
        latestVersion: version,
      );

      AppLogger.info('CLI updated to $version');
    } catch (e) {
      AppLogger.error('Failed to download latest CLI', e);
      state = state.copyWith(
        status: DownloadStatus.error,
        errorMessage: e.toString(),
      );
    }
  }

  Future<bool> checkForUpdate() async {
    try {
      final needsUpdate = await _downloadService.isUpdateAvailable();
      if (needsUpdate) {
        final release = await _apiService.getLatestRelease();
        state = state.copyWith(
          latestVersion: release.tagName,
          status: state.status,
        );
      }
      return needsUpdate;
    } catch (e) {
      AppLogger.error('Update check failed', e);
      return false;
    }
  }
}
