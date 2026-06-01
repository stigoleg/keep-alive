@TestOn('!windows')
library;

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/services/process_manager.dart';
import 'package:keep_alive_app/services/stale_cli_sweeper.dart';
// ignore: depend_on_referenced_packages
import 'package:path_provider_platform_interface/path_provider_platform_interface.dart';

class _MemoryPathProvider extends PathProviderPlatform {
  final String dir;
  _MemoryPathProvider(this.dir);

  @override
  Future<String?> getApplicationSupportPath() async => dir;
}

void main() {
  late Directory supportDir;

  setUp(() async {
    supportDir =
        await Directory.systemTemp.createTemp('stale_sweeper_test_');
    PathProviderPlatform.instance = _MemoryPathProvider(supportDir.path);
  });

  tearDown(() async {
    if (supportDir.existsSync()) {
      await supportDir.delete(recursive: true);
    }
  });

  group('StaleCliSweeper', () {
    test('clears pid file when no PID is alive', () async {
      final path = await ProcessManager.resolvePidFilePath();
      await File(path).writeAsString('9999999\n'); // Very unlikely to exist

      await StaleCliSweeper.sweep();

      expect(File(path).existsSync(), isFalse);
    });

    test('leaves unrelated live process untouched and clears pid file',
        () async {
      // Spawn `sleep` so we have an alive PID that is not "keepalive".
      final probe = await Process.start('sleep', ['30']);
      addTearDown(() async {
        try {
          probe.kill(ProcessSignal.sigkill);
          await probe.exitCode.timeout(const Duration(seconds: 2));
        } catch (_) {}
      });

      final path = await ProcessManager.resolvePidFilePath();
      await File(path).writeAsString('${probe.pid}\n');

      await StaleCliSweeper.sweep();

      // PID file removed regardless (we treat unknown-but-alive PIDs as
      // stale ownership), but the unrelated process is still running.
      expect(File(path).existsSync(), isFalse);

      // Check the unrelated process is still alive via kill -0.
      final liveCheck =
          await Process.run('kill', ['-0', probe.pid.toString()]);
      expect(liveCheck.exitCode, 0,
          reason: 'sleep should still be alive — sweeper must not kill it');
    });

    test('handles missing pid file gracefully', () async {
      // Just don't write anything; sweep should be a no-op.
      await StaleCliSweeper.sweep();
      // No exception is success.
    });

    test('handles malformed pid file', () async {
      final path = await ProcessManager.resolvePidFilePath();
      await File(path).writeAsString('not-a-number\n');

      await StaleCliSweeper.sweep();

      expect(File(path).existsSync(), isFalse);
    });
  });
}
