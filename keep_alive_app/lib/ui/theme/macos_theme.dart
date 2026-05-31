import 'package:flutter/material.dart';

import 'app_theme.dart';

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
      surface: (isDark ? const Color(0xFF1C1C1E) : const Color(0xFFF6F6F6))
          .withValues(alpha: isDark ? 0.94 : 0.90),
      surfaceContainerHighest:
          (isDark ? const Color(0xFF2C2C2E) : const Color(0xFFE8E8E8))
              .withValues(alpha: 0.88),
      onSurface: isDark ? const Color(0xFFFFFFFF) : const Color(0xFF1D1D1F),
      onSurfaceVariant:
          isDark ? const Color(0xFF98989D) : const Color(0xFF6E6E73),
    );

    return ThemeData(
      useMaterial3: true,
      colorScheme: colorScheme,
      brightness: brightness,
      scaffoldBackgroundColor: Colors.transparent,
      cardTheme: CardThemeData(
        elevation: 0,
        margin: EdgeInsets.zero,
        color: colorScheme.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppTheme.radiusLarge),
        ),
      ),
      switchTheme: SwitchThemeData(
        thumbIcon: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return const Icon(Icons.check, size: 12, color: Colors.white);
          }
          return null;
        }),
        trackOutlineWidth: WidgetStateProperty.all(0),
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
      textTheme: _sfCompactTextTheme(isDark),
      iconTheme: IconThemeData(
        size: AppTheme.iconSmall,
        color: colorScheme.onSurfaceVariant,
      ),
      inputDecorationTheme: InputDecorationTheme(
        isDense: true,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: AppTheme.spacing6,
          vertical: AppTheme.spacing4,
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
        color: colorScheme.onSurface.withValues(alpha: 0.08),
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
          horizontal: AppTheme.spacing6,
          vertical: AppTheme.spacing4,
        ),
      ),
    );
  }

  static TextTheme _sfCompactTextTheme(bool isDark) {
    final color = isDark ? Colors.white : const Color(0xFF1D1D1F);
    final secondary =
        isDark ? const Color(0xFF98989D) : const Color(0xFF6E6E73);

    return TextTheme(
      headlineSmall: TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w600,
        color: color,
        letterSpacing: -0.08,
      ),
      titleMedium: TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w500,
        color: color,
        letterSpacing: -0.05,
      ),
      titleSmall: TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w500,
        color: secondary,
      ),
      bodyMedium: TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w400,
        color: color,
        letterSpacing: -0.05,
      ),
      bodySmall: TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w400,
        color: secondary,
      ),
      labelLarge: TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w600,
        color: color,
      ),
      labelMedium: TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w500,
        color: secondary,
      ),
    );
  }
}
