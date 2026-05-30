import 'package:flutter/material.dart';

/// Shared design tokens and helpers for platform-adaptive theming.
class AppTheme {
  AppTheme._();

  static const Color activeColor = Color(0xFF4CAF50);
  static const Color inactiveColor = Color(0xFF9E9E9E);
  static const Color warningColor = Color(0xFFFFC107);
  static const Color errorColor = Color(0xFFEF5350);

  static const double spacing4 = 4;
  static const double spacing6 = 6;
  static const double spacing8 = 8;
  static const double spacing10 = 10;
  static const double spacing12 = 12;
  static const double spacing16 = 16;
  static const double spacing20 = 20;
  static const double spacing24 = 24;
  static const double spacing32 = 32;

  static const double radiusSmall = 6;
  static const double radiusMedium = 8;
  static const double radiusLarge = 12;

  static const double iconSmall = 14;
  static const double iconMedium = 18;
  static const double iconLarge = 22;

  static const double popupWidthMacOS = 300;
  static const double popupWidthWindows = 320;
  static const double popupWidthLinux = 320;
}
