class ReleaseAsset {
  final String name;
  final String downloadUrl;
  final int size;

  const ReleaseAsset({
    required this.name,
    required this.downloadUrl,
    required this.size,
  });

  ReleaseAsset copyWith({
    String? name,
    String? downloadUrl,
    int? size,
  }) {
    return ReleaseAsset(
      name: name ?? this.name,
      downloadUrl: downloadUrl ?? this.downloadUrl,
      size: size ?? this.size,
    );
  }

  Map<String, dynamic> toJson() => {
        'name': name,
        'browser_download_url': downloadUrl,
        'size': size,
      };

  factory ReleaseAsset.fromJson(Map<String, dynamic> json) {
    return ReleaseAsset(
      name: json['name'] as String,
      downloadUrl: json['browser_download_url'] as String,
      size: json['size'] as int,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is ReleaseAsset &&
          name == other.name &&
          downloadUrl == other.downloadUrl &&
          size == other.size;

  @override
  int get hashCode => name.hashCode ^ downloadUrl.hashCode ^ size.hashCode;

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

  GitHubRelease copyWith({
    String? tagName,
    List<ReleaseAsset>? assets,
  }) {
    return GitHubRelease(
      tagName: tagName ?? this.tagName,
      assets: assets ?? this.assets,
    );
  }

  Map<String, dynamic> toJson() => {
        'tag_name': tagName,
        'assets': assets.map((a) => a.toJson()).toList(),
      };

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
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is GitHubRelease &&
          tagName == other.tagName &&
          _listEquals(assets, other.assets);

  @override
  int get hashCode => tagName.hashCode ^ Object.hashAll(assets);

  @override
  String toString() => 'GitHubRelease(tagName: $tagName, assets: ${assets.length})';
}

bool _listEquals<T>(List<T>? a, List<T>? b) {
  if (identical(a, b)) return true;
  if (a == null || b == null) return false;
  if (a.length != b.length) return false;
  for (var i = 0; i < a.length; i++) {
    if (a[i] != b[i]) return false;
  }
  return true;
}
