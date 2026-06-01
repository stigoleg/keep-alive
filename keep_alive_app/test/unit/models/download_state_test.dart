import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/download_state.dart';

void main() {
  group('DownloadState', () {
    test('defaults to notInstalled', () {
      const state = DownloadState();
      expect(state.status, DownloadStatus.notInstalled);
      expect(state.isDownloading, isFalse);
      expect(state.progress, 0.0);
      expect(state.installedVersion, isNull);
      expect(state.latestVersion, isNull);
      expect(state.errorMessage, isNull);
    });

    test('isDownloading is true only for downloading status', () {
      const downloading = DownloadState(status: DownloadStatus.downloading);
      expect(downloading.isDownloading, isTrue);

      for (final status in DownloadStatus.values) {
        if (status != DownloadStatus.downloading) {
          final state = DownloadState(status: status);
          expect(state.isDownloading, isFalse);
        }
      }
    });

    group('copyWith', () {
      test('copies all fields unchanged', () {
        const original = DownloadState(
          status: DownloadStatus.installed,
          progress: 1.0,
          installedVersion: 'v1.2.3',
          latestVersion: 'v1.2.3',
          errorMessage: null,
        );
        final copied = original.copyWith();
        expect(copied, original);
      });

      test('updates specific fields', () {
        const original = DownloadState();
        final updated = original.copyWith(
          status: DownloadStatus.downloading,
          progress: 0.5,
        );
        expect(updated.status, DownloadStatus.downloading);
        expect(updated.progress, 0.5);
        expect(updated.installedVersion, isNull);
      });

      test('clears error message with null', () {
        const original = DownloadState(
          status: DownloadStatus.error,
          errorMessage: 'failed',
        );
        final updated = original.copyWith(errorMessage: null);
        expect(updated.errorMessage, isNull);
      });
    });

    group('equality', () {
      test('identical values are equal', () {
        const a = DownloadState(
          status: DownloadStatus.downloading,
          progress: 0.75,
        );
        const b = DownloadState(
          status: DownloadStatus.downloading,
          progress: 0.75,
        );
        expect(a, b);
      });

      test('different values are not equal', () {
        const a = DownloadState(status: DownloadStatus.notInstalled);
        const b = DownloadState(status: DownloadStatus.installed);
        expect(a, isNot(b));
      });

      test('hashCode matches for equal values', () {
        const a = DownloadState(
          status: DownloadStatus.error,
          errorMessage: 'network down',
        );
        const b = DownloadState(
          status: DownloadStatus.error,
          errorMessage: 'network down',
        );
        expect(a.hashCode, b.hashCode);
      });
    });

    group('JSON serialization', () {
      test('roundtrip preserves all fields', () {
        const original = DownloadState(
          status: DownloadStatus.installed,
          progress: 1.0,
          installedVersion: 'v2.0.0',
          latestVersion: 'v2.0.0',
          errorMessage: null,
        );
        final json = original.toJson();
        final restored = DownloadState.fromJson(json);
        expect(restored.status, original.status);
        expect(restored.progress, original.progress);
        expect(restored.installedVersion, original.installedVersion);
        expect(restored.latestVersion, original.latestVersion);
        expect(restored, original);
      });

      test('roundtrip with error state', () {
        const original = DownloadState(
          status: DownloadStatus.error,
          progress: 0.2,
          errorMessage: 'disk full',
        );
        final json = original.toJson();
        final restored = DownloadState.fromJson(json);
        expect(restored, original);
      });

      test('fromJson with missing fields returns defaults', () {
        final restored = DownloadState.fromJson({});
        expect(restored.status, DownloadStatus.notInstalled);
        expect(restored.progress, 0.0);
      });

      test('fromJson with unknown status name falls back to notInstalled', () {
        final restored = DownloadState.fromJson({'status': 'unknown'});
        expect(restored.status, DownloadStatus.notInstalled);
      });
    });

    test('toString produces descriptive string', () {
      const state = DownloadState(
        status: DownloadStatus.downloading,
        progress: 0.42,
        installedVersion: 'v1.0.0',
      );
      final str = state.toString();
      expect(str, contains('DownloadState'));
      expect(str, contains('download'));
      expect(str, contains('v1.0.0'));
    });
  });

  group('DownloadStatus', () {
    test('has correct enum values', () {
      expect(DownloadStatus.values.length, 4);
      expect(DownloadStatus.notInstalled.name, 'notInstalled');
      expect(DownloadStatus.downloading.name, 'downloading');
      expect(DownloadStatus.installed.name, 'installed');
      expect(DownloadStatus.error.name, 'error');
    });
  });
}
