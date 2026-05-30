import 'package:flutter/material.dart';

import 'app_theme.dart';

/// Linux GTK/Adwaita menu bar popup theme.
///
/// Adwaita-inspired color palette, 8px rounded corners, system font,
/// and Adwaita-style wider rounded toggle switches.
class LinuxTheme {
  LinuxTheme._();

  static ThemeData get lightTheme => _build(Brightness.light);
  static ThemeData get darkTheme => _build(Brightness.dark);

  static ThemeData _build(Brightness brightness) {
    final isDark = brightness == Brightness.dark;

    final colorScheme = ColorScheme.fromSeed(
      seedColor: const Color(0xFF3584E4),
      brightness: brightness,
    ).copyWith(
      surface: isDark ? const Color(0xFF242424) : const Color(0xFFFAFAFA),
      surfaceContainerHighest:
          isDark ? const Color(0xFF303030) : const Color(0xFFF0F0F0),
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
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
        ),
      ),
      switchTheme: SwitchThemeData(
        trackOutlineWidth: WidgetStateProperty.all(1.5),
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
      textTheme: _adwaitaTextTheme(isDark),
      iconTheme: IconThemeData(
        size: AppTheme.iconMedium,
        color: colorScheme.onSurface.withValues(alpha: 0.8),
      ),
      inputDecorationTheme: InputDecorationTheme(
        isDense: true,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing10,
          vertical: AppTheme.spacing8,
        ),
        filled: true,
        fillColor: colorScheme.surfaceContainerHighest,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
          borderSide: BorderSide(
            color: colorScheme.outline.withValues(alpha: 0.4),
            width: 1,
          ),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
          borderSide: BorderSide(
            color: colorScheme.outline.withValues(alpha: 0.25),
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
        elevation: 6,
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
          horizontal: AppTheme.spacing10,
          vertical: AppTheme.spacing6,
        ),
      ),
    );
  }

  static TextTheme _adwaitaTextTheme(bool isDark) {
    final base = ThemeData(brightness: isDark ? Brightness.dark : Brightness.light).textTheme;
    final color = isDark ? Colors.white : const Color(0xFF1A1A1A);
    return base.copyWith(
      headlineSmall: base.headlineSmall?.copyWith(
        fontSize: 16,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      titleMedium: base.titleMedium?.copyWith(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      titleSmall: base.titleSmall?.copyWith(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: color.withValues(alpha: 0.7),
      ),
      bodyMedium: base.bodyMedium?.copyWith(
        fontSize: 14,
        color: color,
      ),
      bodySmall: base.bodySmall?.copyWith(
        fontSize: 12,
        color: color.withValues(alpha: 0.55),
      ),
      labelLarge: base.labelLarge?.copyWith(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      labelMedium: base.labelMedium?.copyWith(
        fontSize: 12,
        color: color.withValues(alpha: 0.7),
      ),
    );
  }
}
