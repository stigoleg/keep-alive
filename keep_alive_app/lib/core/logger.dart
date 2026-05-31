import 'dart:io' show Platform, stderr;

import 'package:flutter/foundation.dart';
import 'package:logging/logging.dart';

import 'constants.dart';

class AppLogger {
  AppLogger._();

  static final Logger _logger = Logger('KeepAlive');
  static final List<String> _ringBuffer = [];
  static final String? _homeDir = _resolveHome();

  static String? _resolveHome() {
    final env = Platform.environment;
    return env['HOME'] ?? env['USERPROFILE'];
  }

  /// Replaces the user's home directory prefix with `~` in release builds
  /// to avoid leaking absolute paths into shipped logs / bug reports.
  /// In debug builds the original path is returned unchanged.
  static String scrubPath(String path) {
    if (kDebugMode) return path;
    final home = _homeDir;
    if (home == null || home.isEmpty) return path;
    if (path.startsWith(home)) {
      return '~${path.substring(home.length)}';
    }
    return path;
  }

  static List<String> get recentLogs => List.unmodifiable(_ringBuffer);

  static void clearLogs() {
    _ringBuffer.clear();
  }

  static List<String> filteredLogs(String? levelFilter) {
    if (levelFilter == null) return recentLogs;
    final upper = levelFilter.toUpperCase();
    return _ringBuffer.where((line) => line.contains('[$upper]')).toList();
  }

  static void init() {
    Logger.root.level = Level.ALL;
    Logger.root.onRecord.listen(_onRecord);
  }

  static void _onRecord(LogRecord record) {
    final buffer = StringBuffer();
    buffer.write('${record.time} [${record.level.name}] '
        '${record.loggerName}: ${record.message}');
    if (record.error != null) {
      buffer.write(' | error: ${record.error}');
    }
    if (record.stackTrace != null) {
      buffer.write('\n${record.stackTrace}');
    }
    final line = buffer.toString();

    stderr.writeln(line);

    _ringBuffer.add(line);
    if (_ringBuffer.length > AppConstants.maxLogLines) {
      _ringBuffer.removeRange(
        0,
        _ringBuffer.length - AppConstants.maxLogLines,
      );
    }
  }

  static void debug(String message) => _logger.fine(message);
  static void info(String message) => _logger.info(message);
  static void warning(String message) => _logger.warning(message);
  static void error(String message, [Object? error, StackTrace? stack]) =>
      _logger.severe(message, error, stack);
}
