import 'package:flutter/material.dart';

import 'ui/theme/linux_theme.dart';
import 'ui/theme/macos_theme.dart';
import 'ui/theme/windows_theme.dart';
import 'utils/platform_utils.dart';

class KeepAliveApp extends StatelessWidget {
  const KeepAliveApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'KeepAlive',
      debugShowCheckedModeBanner: false,
      themeMode: ThemeMode.system,
      theme: _lightTheme,
      darkTheme: _darkTheme,
      home: const Scaffold(
        body: Center(),
      ),
    );
  }
}

ThemeData get _resolveLight {
  if (PlatformUtils.isMacOS) return MacOSTheme.lightTheme;
  if (PlatformUtils.isWindows) return WindowsTheme.lightTheme;
  return LinuxTheme.lightTheme;
}

ThemeData get _resolveDark {
  if (PlatformUtils.isMacOS) return MacOSTheme.darkTheme;
  if (PlatformUtils.isWindows) return WindowsTheme.darkTheme;
  return LinuxTheme.darkTheme;
}

final ThemeData _lightTheme = _resolveLight;
final ThemeData _darkTheme = _resolveDark;
