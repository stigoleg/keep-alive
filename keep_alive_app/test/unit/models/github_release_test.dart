import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/github_release.dart';

void main() {
  group('ReleaseAsset', () {
    const testAsset = ReleaseAsset(
      name: 'keepalive_linux_amd64.tar.gz',
      downloadUrl: 'https://github.com/stigoleg/keep-alive/releases/download/v1.0.0/keepalive_linux_amd64.tar.gz',
      size: 5_000_000,
    );

    test('fromJson parses correctly', () {
      final json = {
        'name': 'keepalive_darwin_arm64.tar.gz',
        'browser_download_url': 'https://example.com/asset.tar.gz',
        'size': 3_000_000,
      };
      final asset = ReleaseAsset.fromJson(json);
      expect(asset.name, 'keepalive_darwin_arm64.tar.gz');
      expect(asset.downloadUrl, 'https://example.com/asset.tar.gz');
      expect(asset.size, 3_000_000);
    });

    group('toJson', () {
      test('serializes all fields', () {
        final json = testAsset.toJson();
        expect(json['name'], testAsset.name);
        expect(json['browser_download_url'], testAsset.downloadUrl);
        expect(json['size'], testAsset.size);
      });
    });

    group('copyWith', () {
      test('copies all fields unchanged', () {
        final copied = testAsset.copyWith();
        expect(copied, testAsset);
      });

      test('updates specific fields', () {
        final updated = testAsset.copyWith(name: 'new_name.tar.gz');
        expect(updated.name, 'new_name.tar.gz');
        expect(updated.downloadUrl, testAsset.downloadUrl);
        expect(updated.size, testAsset.size);
      });
    });

    group('equality', () {
      test('identical values are equal', () {
        const a = ReleaseAsset(name: 'a', downloadUrl: 'url', size: 1);
        const b = ReleaseAsset(name: 'a', downloadUrl: 'url', size: 1);
        expect(a, equals(b));
      });

      test('different values are not equal', () {
        const a = ReleaseAsset(name: 'a', downloadUrl: 'url', size: 1);
        const b = ReleaseAsset(name: 'b', downloadUrl: 'url', size: 1);
        expect(a, isNot(equals(b)));
      });
    });

    test('toString produces descriptive string', () {
      final str = testAsset.toString();
      expect(str, contains('ReleaseAsset'));
      expect(str, contains(testAsset.name));
      expect(str, contains(testAsset.size.toString()));
    });
  });

  group('GitHubRelease', () {
    final testAssets = [
      const ReleaseAsset(name: 'a', downloadUrl: 'url_a', size: 100),
      const ReleaseAsset(name: 'b', downloadUrl: 'url_b', size: 200),
    ];

    final testRelease = GitHubRelease(
      tagName: 'v1.0.0',
      assets: testAssets,
    );

    test('fromJson parses correctly with assets', () {
      final json = {
        'tag_name': 'v2.0.0',
        'assets': [
          {
            'name': 'asset1.tar.gz',
            'browser_download_url': 'https://example.com/asset1',
            'size': 4_000_000,
          },
          {
            'name': 'asset2.zip',
            'browser_download_url': 'https://example.com/asset2',
            'size': 5_000_000,
          },
        ],
      };
      final release = GitHubRelease.fromJson(json);
      expect(release.tagName, 'v2.0.0');
      expect(release.assets.length, 2);
      expect(release.assets[0].name, 'asset1.tar.gz');
      expect(release.assets[1].name, 'asset2.zip');
    });

    test('fromJson handles null assets', () {
      final json = {
        'tag_name': 'v1.0.0',
      };
      final release = GitHubRelease.fromJson(json);
      expect(release.tagName, 'v1.0.0');
      expect(release.assets, isEmpty);
    });

    test('fromJson handles empty assets list', () {
      final json = {
        'tag_name': 'v1.0.0',
        'assets': [],
      };
      final release = GitHubRelease.fromJson(json);
      expect(release.assets, isEmpty);
    });

    group('toJson', () {
      test('serializes all fields', () {
        final json = testRelease.toJson();
        expect(json['tag_name'], 'v1.0.0');
        expect(json['assets'], isA<List>());
        expect((json['assets'] as List).length, 2);
      });
    });

    group('copyWith', () {
      test('copies all fields unchanged', () {
        final copied = testRelease.copyWith();
        expect(copied, testRelease);
      });

      test('updates specific fields', () {
        final newAssets = [const ReleaseAsset(name: 'c', downloadUrl: 'url_c', size: 300)];
        final updated = testRelease.copyWith(tagName: 'v2.0.0', assets: newAssets);
        expect(updated.tagName, 'v2.0.0');
        expect(updated.assets.length, 1);
        expect(updated.assets.first.name, 'c');
      });
    });

    group('equality', () {
      test('identical values are equal', () {
        const a = GitHubRelease(
          tagName: 'v1.0.0',
          assets: [],
        );
        const b = GitHubRelease(
          tagName: 'v1.0.0',
          assets: [],
        );
        expect(a, b);
      });

      test('different values are not equal', () {
        const a = GitHubRelease(tagName: 'v1.0.0', assets: []);
        const b = GitHubRelease(tagName: 'v2.0.0', assets: []);
        expect(a, isNot(b));
      });

      test('hashCode matches for equal values', () {
        const assets = [ReleaseAsset(name: 'x', downloadUrl: 'y', size: 1)];
        const a = GitHubRelease(tagName: 'v1.0.0', assets: assets);
        const b = GitHubRelease(tagName: 'v1.0.0', assets: assets);
        expect(a.hashCode, b.hashCode);
      });
    });
  });
}
