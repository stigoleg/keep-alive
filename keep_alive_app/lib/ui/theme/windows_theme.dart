import 'package:flutter/material.dart';

import 'app_theme.dart';

/// Windows 11 Fluent Design menu bar popup theme.
///
/// Acrylic-tinted surface colors, 8px rounded corners, Segoe UI system font,
/// and Windows 11-style wider toggle switches.
class WindowsTheme {
  WindowsTheme._();

  static ThemeData get lightTheme => _build(Brightness.light);
  static ThemeData get darkTheme => _build(Brightness.dark);

  static ThemeData _build(Brightness brightness) {
    final isDark = brightness == Brightness.dark;

    final colorScheme = ColorScheme.fromSeed(
      seedColor: const Color(0xFF0078D4),
      brightness: brightness,
    ).copyWith(
      surface: (isDark ? const Color(0xFF202020) : const Color(0xFFF3F3F3))
          .withValues(alpha: 0.90),
      surfaceContainerHighest:
          (isDark ? const Color(0xFF2D2D2D) : const Color(0xFFF9F9F9))
              .withValues(alpha: 0.85),
    );

    return ThemeData(
      useMaterial3: true,
      colorScheme: colorScheme,
      brightness: brightness,
      cardTheme: CardThemeData(
        elevation: isDark ? 0 : 1,
        margin: EdgeInsets.zero,
        color: colorScheme.surface,
        shadowColor: isDark ? Colors.transparent : Colors.black.withValues(alpha: 0.04),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
        ),
      ),
      switchTheme: SwitchThemeData(
        trackOutlineWidth: WidgetStateProperty.all(2),
        materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
        splashRadius: 18,
      ),
      sliderTheme: SliderThemeData(
        trackHeight: 4,
        activeTrackColor: colorScheme.primary,
        inactiveTrackColor: colorScheme.surfaceContainerHighest,
        thumbColor: colorScheme.primary,
        overlayColor: colorScheme.primary.withValues(alpha: 0.12),
      ),
      textTheme: _fluentTextTheme(isDark),
      iconTheme: IconThemeData(
        size: AppTheme.iconMedium,
        color: colorScheme.onSurface.withValues(alpha: 0.7),
      ),
      inputDecorationTheme: InputDecorationTheme(
        isDense: true,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing12,
          vertical: AppTheme.spacing10,
        ),
        filled: true,
        fillColor: colorScheme.surfaceContainerHighest,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
          borderSide: BorderSide(
            color: colorScheme.outline.withValues(alpha: 0.3),
            width: 1,
          ),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
          borderSide: BorderSide(
            color: colorScheme.outline.withValues(alpha: 0.2),
            width: 1,
          ),
        ),
      ),
      dividerTheme: DividerThemeData(
        space: 1,
        thickness: 1,
        color: colorScheme.outlineVariant.withValues(alpha: 0.4),
      ),
      popupMenuTheme: PopupMenuThemeData(
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
        ),
        elevation: 8,
        position: PopupMenuPosition.under,
      ),
      tooltipTheme: TooltipThemeData(
        decoration: BoxDecoration(
          color: colorScheme.inverseSurface,
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        ),
        textStyle: TextStyle(
          color: colorScheme.onInverseSurface,
          fontSize: 12,
        ),
        padding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing12,
          vertical: AppTheme.spacing6,
        ),
      ),
    );
  }

  static TextTheme _fluentTextTheme(bool isDark) {
    final base = ThemeData(brightness: isDark ? Brightness.dark : Brightness.light).textTheme;
    final color = isDark ? Colors.white : const Color(0xFF1A1A1A);
    return base.copyWith(
      headlineSmall: base.headlineSmall?.copyWith(
        fontSize: 15,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      titleMedium: base.titleMedium?.copyWith(
        fontSize: 13,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      titleSmall: base.titleSmall?.copyWith(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: color.withValues(alpha: 0.65),
      ),
      bodyMedium: base.bodyMedium?.copyWith(
        fontSize: 13,
        color: color,
      ),
      bodySmall: base.bodySmall?.copyWith(
        fontSize: 12,
        color: color.withValues(alpha: 0.55),
      ),
      labelLarge: base.labelLarge?.copyWith(
        fontSize: 13,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      labelMedium: base.labelMedium?.copyWith(
        fontSize: 12,
        color: color.withValues(alpha: 0.65),
      ),
    );
  }
}
