import 'package:flutter/material.dart';

import 'app_theme.dart';

/// macOS menu bar popup theme.
///
/// Compact layout with translucent surface colors, SF-style typography,
/// 12px rounded corners, and macOS-style small capsule switches.
class MacOSTheme {
  MacOSTheme._();

  static ThemeData get lightTheme => _build(Brightness.light);
  static ThemeData get darkTheme => _build(Brightness.dark);

  static ThemeData _build(Brightness brightness) {
    final isDark = brightness == Brightness.dark;

    final colorScheme = ColorScheme.fromSeed(
      seedColor: Colors.blueGrey,
      brightness: brightness,
    ).copyWith(
      surface: (isDark ? const Color(0xFF1E1E20) : const Color(0xFFF5F5F5))
          .withValues(alpha: 0.92),
      surfaceContainerHighest:
          (isDark ? const Color(0xFF2C2C2E) : Colors.white).withValues(alpha: 0.88),
    );

    return ThemeData(
      useMaterial3: true,
      colorScheme: colorScheme,
      brightness: brightness,
      cardTheme: CardThemeData(
        elevation: 0,
        margin: EdgeInsets.zero,
        color: colorScheme.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusLarge),
        ),
      ),
      switchTheme: SwitchThemeData(
        trackOutlineWidth: WidgetStateProperty.all(1.5),
        materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
        splashRadius: 14,
      ),
      sliderTheme: SliderThemeData(
        trackHeight: 3,
        activeTrackColor: colorScheme.primary,
        inactiveTrackColor: colorScheme.surfaceContainerHighest,
        thumbColor: colorScheme.primary,
        overlayColor: colorScheme.primary.withValues(alpha: 0.12),
      ),
      textTheme: _compactTextTheme(isDark),
      iconTheme: IconThemeData(
        size: AppTheme.iconMedium,
        color: colorScheme.onSurface.withValues(alpha: 0.8),
      ),
      inputDecorationTheme: InputDecorationTheme(
        isDense: true,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing8,
          vertical: AppTheme.spacing6,
        ),
        filled: true,
        fillColor: colorScheme.surfaceContainerHighest,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
          borderSide: BorderSide.none,
        ),
      ),
      dividerTheme: DividerThemeData(
        space: 1,
        thickness: 0.5,
        color: colorScheme.surfaceContainerHighest,
      ),
      popupMenuTheme: PopupMenuThemeData(
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
        ),
        elevation: 3,
        position: PopupMenuPosition.under,
      ),
      tooltipTheme: TooltipThemeData(
        decoration: BoxDecoration(
          color: colorScheme.inverseSurface,
          borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        ),
        textStyle: TextStyle(
          color: colorScheme.onInverseSurface,
          fontSize: 11,
        ),
        padding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing8,
          vertical: AppTheme.spacing4,
        ),
      ),
    );
  }

  static TextTheme _compactTextTheme(bool isDark) {
    final base = ThemeData(brightness: isDark ? Brightness.dark : Brightness.light).textTheme;
    final color = isDark ? Colors.white : Colors.black87;
    return base.copyWith(
      headlineSmall: base.headlineSmall?.copyWith(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      titleMedium: base.titleMedium?.copyWith(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      titleSmall: base.titleSmall?.copyWith(
        fontSize: 11,
        fontWeight: FontWeight.w600,
        color: color.withValues(alpha: 0.7),
      ),
      bodyMedium: base.bodyMedium?.copyWith(
        fontSize: 13,
        color: color,
      ),
      bodySmall: base.bodySmall?.copyWith(
        fontSize: 11,
        color: color.withValues(alpha: 0.6),
      ),
      labelLarge: base.labelLarge?.copyWith(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      labelMedium: base.labelMedium?.copyWith(
        fontSize: 11,
        color: color.withValues(alpha: 0.7),
      ),
    );
  }
}
