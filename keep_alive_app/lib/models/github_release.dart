class ReleaseAsset {
  final String name;
  final String downloadUrl;
  final int size;

  const ReleaseAsset({
    required this.name,
    required this.downloadUrl,
    required this.size,
  });

  factory ReleaseAsset.fromJson(Map<String, dynamic> json) {
    return ReleaseAsset(
      name: json['name'] as String,
      downloadUrl: json['browser_download_url'] as String,
      size: json['size'] as int,
    );
  }

  @override
  String toString() => 'ReleaseAsset(name: $name, size: $size)';
}

class GitHubRelease {
  final String tagName;
  final List<ReleaseAsset> assets;

  const GitHubRelease({
    required this.tagName,
    required this.assets,
  });

  factory GitHubRelease.fromJson(Map<String, dynamic> json) {
    final assetsList = (json['assets'] as List<dynamic>?)
            ?.map((a) => ReleaseAsset.fromJson(a as Map<String, dynamic>))
            .toList() ??
        [];
    return GitHubRelease(
      tagName: json['tag_name'] as String,
      assets: assetsList,
    );
  }

  @override
  String toString() => 'GitHubRelease(tagName: $tagName, assets: ${assets.length})';
}
