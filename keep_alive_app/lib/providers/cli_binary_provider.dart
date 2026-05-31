import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/exceptions.dart';
import '../core/logger.dart';
import '../models/download_state.dart';
import '../services/cli_download_service.dart';
import '../services/github_api_service.dart';

final githubApiServiceProvider = Provider<GitHubApiService>((ref) {
  return GitHubApiService(dio: Dio());
});

final cliDownloadServiceProvider = Provider<CliDownloadService>((ref) {
  return CliDownloadService(
    apiService: ref.watch(githubApiServiceProvider),
    dio: Dio(),
  );
});

final cliBinaryProvider =
    NotifierProvider<CliBinaryNotifier, DownloadState>(
  CliBinaryNotifier.new,
);

class CliBinaryNotifier extends Notifier<DownloadState> {
  late final GitHubApiService _apiService;
  late final CliDownloadService _downloadService;
  Completer<void>? _readyCompleter;

  @override
  DownloadState build() {
    _apiService = ref.watch(githubApiServiceProvider);
    _downloadService = ref.watch(cliDownloadServiceProvider);
    return const DownloadState();
  }

  Future<void> waitUntilReady() async {
    if (state.status == DownloadStatus.installed) return;
    if (state.status == DownloadStatus.downloading) {
      await (_readyCompleter?.future ?? Future<void>.value());
      return;
    }
    _readyCompleter = Completer<void>();
    try {
      await checkAndInstall();
    } finally {
      _readyCompleter?.complete();
      _readyCompleter = null;
    }
  }

  bool get isReady => state.status == DownloadStatus.installed;

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
    } on DownloadException catch (e) {
      AppLogger.error('Failed to install CLI binary (DownloadException)', e);
      state = state.copyWith(
        status: DownloadStatus.error,
        errorMessage: e.message,
      );
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
      final prevCompleter = _readyCompleter;
      _readyCompleter = Completer<void>();

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
      prevCompleter?.complete();
      _readyCompleter?.complete();
    } on DownloadException catch (e) {
      AppLogger.error('Failed to download latest CLI (DownloadException)', e);
      state = state.copyWith(
        status: DownloadStatus.error,
        errorMessage: e.message,
      );
      _readyCompleter?.complete();
    } catch (e) {
      AppLogger.error('Failed to download latest CLI', e);
      state = state.copyWith(
        status: DownloadStatus.error,
        errorMessage: e.toString(),
      );
      _readyCompleter?.complete();
    } finally {
      _readyCompleter = null;
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
