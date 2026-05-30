import 'dart:io' show Platform;

import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/platform/platform_interface.dart';

void main() {
  group('KeepAlivePlatform', () {
    test('instance is not null', () {
      final platform = KeepAlivePlatform.instance;
      expect(platform, isNotNull);
    });

    test('instance is correct type for current platform', () {
      final platform = KeepAlivePlatform.instance;
      if (Platform.isMacOS) {
        expect(platform.runtimeType.toString(), contains('MacOS'));
      } else if (Platform.isWindows) {
        expect(platform.runtimeType.toString(), contains('Windows'));
      } else if (Platform.isLinux) {
        expect(platform.runtimeType.toString(), contains('Linux'));
      }
    });

    test('singleton instance is stable across calls', () {
      final a = KeepAlivePlatform.instance;
      final b = KeepAlivePlatform.instance;
      expect(identical(a, b), isTrue);
    });
  });
}
