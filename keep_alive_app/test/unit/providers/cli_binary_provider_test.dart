import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/download_state.dart';
import 'package:keep_alive_app/providers/cli_binary_provider.dart';

void main() {
  group('CliBinaryNotifier', () {
    late ProviderContainer container;

    setUp(() {
      container = ProviderContainer();
    });

    tearDown(() {
      container.dispose();
    });

    test('initial state is notInstalled', () {
      final state = container.read(cliBinaryProvider);
      expect(state.status, DownloadStatus.notInstalled);
      expect(state.installedVersion, isNull);
      expect(state.errorMessage, isNull);
    });

    test('checkForUpdate returns false when service throws', () async {
      final notifier = container.read(cliBinaryProvider.notifier);
      final result = await notifier.checkForUpdate();
      expect(result, isFalse);
    });

    test('state has correct default progress', () {
      final state = container.read(cliBinaryProvider);
      expect(state.progress, 0.0);
    });

    test('downloadLatest sets error state on failure', () async {
      final notifier = container.read(cliBinaryProvider.notifier);
      await notifier.downloadLatest();

      final state = container.read(cliBinaryProvider);
      expect(state.status, DownloadStatus.error);
      expect(state.errorMessage, isNotNull);
    });

    test('clearError restores installed state when a version is known', () {
      final notifier = container.read(cliBinaryProvider.notifier);
      notifier.state = const DownloadState(
        status: DownloadStatus.error,
        installedVersion: 'v1.0.0',
        latestVersion: 'v1.0.1',
        errorMessage: 'failed',
      );

      notifier.clearError();

      final state = container.read(cliBinaryProvider);
      expect(state.status, DownloadStatus.installed);
      expect(state.errorMessage, isNull);
      expect(state.installedVersion, 'v1.0.0');
    });

    test('clearError restores notInstalled state without a version', () {
      final notifier = container.read(cliBinaryProvider.notifier);
      notifier.state = const DownloadState(
        status: DownloadStatus.error,
        errorMessage: 'failed',
      );

      notifier.clearError();

      final state = container.read(cliBinaryProvider);
      expect(state.status, DownloadStatus.notInstalled);
      expect(state.errorMessage, isNull);
    });
  });
}
