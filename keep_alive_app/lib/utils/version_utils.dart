/// Semantic-version helpers for comparing CLI version strings like `v1.5.4`.
class VersionUtils {
  VersionUtils._();

  /// Returns the parsed (major, minor, patch) tuple for [version], or null
  /// when [version] does not contain a recognisable `X.Y.Z` number. A leading
  /// `v` is tolerated; trailing pre-release/build metadata is ignored.
  static List<int>? parse(String? version) {
    if (version == null) return null;
    final match = RegExp(r'(\d+)\.(\d+)\.(\d+)').firstMatch(version);
    if (match == null) return null;
    return [
      int.parse(match.group(1)!),
      int.parse(match.group(2)!),
      int.parse(match.group(3)!),
    ];
  }

  /// Returns true when [actual] is greater than or equal to [minimum].
  /// Unparseable versions are treated as not meeting the minimum so callers
  /// can reject unknown binaries instead of silently accepting them.
  static bool meetsMinimum(String? actual, String minimum) {
    final a = parse(actual);
    final m = parse(minimum);
    if (a == null || m == null) return false;
    for (var i = 0; i < 3; i++) {
      if (a[i] > m[i]) return true;
      if (a[i] < m[i]) return false;
    }
    return true;
  }
}
