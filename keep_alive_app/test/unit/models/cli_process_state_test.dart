import 'package:flutter_test/flutter_test.dart';
import 'package:keep_alive_app/models/cli_process_state.dart';

void main() {
  group('CliProcessState', () {
    test('defaults to idle', () {
      const state = CliProcessState();
      expect(state.status, CliProcessStatus.idle);
      expect(state.isRunning, isFalse);
      expect(state.pid, isNull);
      expect(state.startTime, isNull);
      expect(state.exitCode, isNull);
      expect(state.errorMessage, isNull);
    });

    test('isRunning is true only for running status', () {
      const running = CliProcessState(status: CliProcessStatus.running);
      expect(running.isRunning, isTrue);

      for (final status in CliProcessStatus.values) {
        if (status != CliProcessStatus.running) {
          final state = CliProcessState(status: status);
          expect(state.isRunning, isFalse);
        }
      }
    });

    group('copyWith', () {
      test('copies all fields unchanged', () {
        final startTime = DateTime(2025, 6, 1, 12, 0);
        final original = CliProcessState(
          status: CliProcessStatus.running,
          pid: 12345,
          startTime: startTime,
          exitCode: 0,
          errorMessage: null,
        );
        final copied = original.copyWith();
        expect(copied, original);
      });

      test('updates specific fields', () {
        const original = CliProcessState();
        final updated = original.copyWith(
          status: CliProcessStatus.starting,
          pid: 9999,
        );
        expect(updated.status, CliProcessStatus.starting);
        expect(updated.pid, 9999);
        expect(updated.startTime, isNull);
      });
    });

    group('equality', () {
      test('identical values are equal', () {
        final startTime = DateTime(2025, 6, 1, 12, 0);
        final a = CliProcessState(
          status: CliProcessStatus.running,
          pid: 42,
          startTime: startTime,
        );
        final b = CliProcessState(
          status: CliProcessStatus.running,
          pid: 42,
          startTime: startTime,
        );
        expect(a, b);
      });

      test('different values are not equal', () {
        const a = CliProcessState(status: CliProcessStatus.idle);
        const b = CliProcessState(status: CliProcessStatus.running);
        expect(a, isNot(b));
      });

      test('hashCode matches for equal values', () {
        const a = CliProcessState(
          status: CliProcessStatus.error,
          errorMessage: 'something wrong',
        );
        const b = CliProcessState(
          status: CliProcessStatus.error,
          errorMessage: 'something wrong',
        );
        expect(a.hashCode, b.hashCode);
      });
    });

    group('JSON serialization', () {
      test('roundtrip preserves all fields', () {
        final startTime = DateTime(2025, 6, 15, 10, 30);
        final original = CliProcessState(
          status: CliProcessStatus.running,
          pid: 54321,
          startTime: startTime,
          exitCode: null,
          errorMessage: null,
        );
        final json = original.toJson();
        final restored = CliProcessState.fromJson(json);
        expect(restored.status, original.status);
        expect(restored.pid, original.pid);
        expect(restored.startTime, original.startTime);
        expect(restored, original);
      });

      test('roundtrip with error state', () {
        const original = CliProcessState(
          status: CliProcessStatus.error,
          exitCode: 1,
          errorMessage: 'binary not found',
        );
        final json = original.toJson();
        final restored = CliProcessState.fromJson(json);
        expect(restored, original);
      });

      test('fromJson with missing fields returns defaults', () {
        final restored = CliProcessState.fromJson({});
        expect(restored.status, CliProcessStatus.idle);
        expect(restored.pid, isNull);
      });

      test('fromJson with unknown status name falls back to idle', () {
        final restored = CliProcessState.fromJson({'status': 'unknown_state'});
        expect(restored.status, CliProcessStatus.idle);
      });
    });

    test('toString produces descriptive string', () {
      const state = CliProcessState(pid: 123, exitCode: 0);
      final str = state.toString();
      expect(str, contains('CliProcessState'));
      expect(str, contains('123'));
    });
  });
}
