import 'dart:io' show Platform;

/// Utilities for platform detection and OS-specific helpers.
class PlatformUtils {
  PlatformUtils._();

  static bool get isMacOS => Platform.isMacOS;
  static bool get isWindows => Platform.isWindows;
  static bool get isLinux => Platform.isLinux;
}
