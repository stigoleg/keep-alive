import 'package:flutter/material.dart';

/// Base theme shared across all platforms.
class AppTheme {
  AppTheme._();

  static ThemeData get baseTheme => ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.blueGrey),
      );
}
