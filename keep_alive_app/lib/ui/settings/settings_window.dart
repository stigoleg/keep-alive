import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/constants.dart';
import '../../core/logger.dart';
import '../../models/download_state.dart';
import '../../platform/platform_interface.dart';
import '../../providers/cli_binary_provider.dart';
import '../../providers/settings_provider.dart';
import '../theme/app_theme.dart';

class SettingsDialog extends ConsumerWidget {
  const SettingsDialog({super.key, required this.onClose});

  final VoidCallback onClose;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(appSettingsProvider);
    final binaryState = ref.watch(cliBinaryProvider);
    final theme = Theme.of(context);

    return Dialog(
      backgroundColor: theme.colorScheme.surface,
      insetPadding: const EdgeInsets.all(AppTheme.spacing16),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppTheme.radiusLarge),
      ),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420, maxHeight: 520),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            _Header(onClose: onClose),
            Flexible(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(
                  horizontal: AppTheme.spacing16,
                  vertical: AppTheme.spacing12,
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    _StartupSection(settings: settings, ref: ref),
                    _buildDivider(theme),
                    _UpdatesSection(binaryState: binaryState, ref: ref),
                    _buildDivider(theme),
                    const _AboutSection(),
                    _buildDivider(theme),
                    const _LogSection(),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildDivider(ThemeData theme) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: AppTheme.spacing8),
      child: Divider(
        height: 1,
        thickness: 0.5,
        color: theme.dividerColor.withValues(alpha: 0.3),
      ),
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.onClose});

  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing16,
        vertical: AppTheme.spacing8,
      ),
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(
            color: theme.dividerColor.withValues(alpha: 0.2),
            width: 0.5,
          ),
        ),
      ),
      child: Row(
        children: [
          const Icon(Icons.settings, size: AppTheme.iconMedium),
          const SizedBox(width: AppTheme.spacing8),
          Expanded(
            child: Text(
              'Settings',
              style: theme.textTheme.titleMedium,
            ),
          ),
          IconButton(
            onPressed: onClose,
            icon: const Icon(Icons.close, size: AppTheme.iconMedium),
            padding: EdgeInsets.zero,
            constraints: const BoxConstraints(
              minWidth: 32,
              minHeight: 32,
            ),
            style: IconButton.styleFrom(
              tapTargetSize: MaterialTapTargetSize.shrinkWrap,
            ),
          ),
        ],
      ),
    );
  }
}

class _StartupSection extends StatelessWidget {
  const _StartupSection({required this.settings, required this.ref});

  final AppSettingsState settings;
  final WidgetRef ref;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Startup',
          style: theme.textTheme.labelMedium?.copyWith(
            color: theme.colorScheme.onSurface.withValues(alpha: 0.6),
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: AppTheme.spacing8),
        _SettingRow(
          label: 'Start on Login',
          subtitle: 'Launch automatically when you log in',
          child: Switch(
            value: settings.autoStart,
            onChanged: (value) => _setAutoStart(value, ref),
          ),
        ),
        const SizedBox(height: AppTheme.spacing4),
        _SettingRow(
          label: 'Start Minimized',
          subtitle: 'Hide to system tray on launch',
          child: Switch(
            value: settings.startMinimized,
            onChanged: (value) =>
                ref.read(appSettingsProvider.notifier).setStartMinimized(value),
          ),
        ),
      ],
    );
  }

  Future<void> _setAutoStart(bool value, WidgetRef ref) async {
    try {
      await KeepAlivePlatform.instance.setAutoStart(value);
    } catch (e) {
      AppLogger.error('Failed to set auto-start', e);
    }
    await ref.read(appSettingsProvider.notifier).setAutoStart(value);
  }
}

class _UpdatesSection extends StatelessWidget {
  const _UpdatesSection({required this.binaryState, required this.ref});

  final DownloadState binaryState;
  final WidgetRef ref;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Updates',
          style: theme.textTheme.labelMedium?.copyWith(
            color: theme.colorScheme.onSurface.withValues(alpha: 0.6),
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: AppTheme.spacing8),
        _SettingRow(
          label: 'CLI Binary',
          subtitle: _subtitle,
          child: _actionButton(context, theme),
        ),
      ],
    );
  }

  String get _subtitle {
    return switch (binaryState.status) {
      DownloadStatus.installed =>
        'v${binaryState.installedVersion ?? 'unknown'} installed',
      DownloadStatus.downloading => 'Downloading\u2026',
      DownloadStatus.notInstalled => 'Not installed',
      DownloadStatus.error => 'Error: ${binaryState.errorMessage ?? 'unknown'}',
    };
  }

  Widget _actionButton(BuildContext context, ThemeData theme) {
    return switch (binaryState.status) {
      DownloadStatus.installed => TextButton(
          onPressed: () async {
            final notifier = ref.read(cliBinaryProvider.notifier);
            if (await notifier.checkForUpdate()) {
              if (context.mounted) notifier.downloadLatest();
            }
          },
          style: TextButton.styleFrom(
            padding: const EdgeInsets.symmetric(
              horizontal: AppTheme.spacing10,
              vertical: AppTheme.spacing4,
            ),
            minimumSize: Size.zero,
            tapTargetSize: MaterialTapTargetSize.shrinkWrap,
          ),
          child: const Text('Check'),
        ),
      DownloadStatus.notInstalled || DownloadStatus.error => TextButton(
          onPressed: () =>
              ref.read(cliBinaryProvider.notifier).downloadLatest(),
          style: TextButton.styleFrom(
            padding: const EdgeInsets.symmetric(
              horizontal: AppTheme.spacing10,
              vertical: AppTheme.spacing4,
            ),
            minimumSize: Size.zero,
            tapTargetSize: MaterialTapTargetSize.shrinkWrap,
          ),
          child: const Text('Download'),
        ),
      DownloadStatus.downloading => SizedBox(
          width: AppTheme.iconMedium,
          height: AppTheme.iconMedium,
          child: CircularProgressIndicator(
            strokeWidth: 2,
            value:
                binaryState.progress > 0 ? binaryState.progress : null,
          ),
        ),
    };
  }
}

class _AboutSection extends StatelessWidget {
  const _AboutSection();

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'About',
          style: theme.textTheme.labelMedium?.copyWith(
            color: theme.colorScheme.onSurface.withValues(alpha: 0.6),
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: AppTheme.spacing8),
        const _SettingRow(
          label: AppConstants.appName,
          subtitle: 'Version ${AppConstants.appVersion}',
          child: SizedBox.shrink(),
        ),
        const SizedBox(height: AppTheme.spacing8),
        Row(
          children: [
            TextButton.icon(
              onPressed: () => showLicensePage(
                context: context,
                applicationName: AppConstants.appName,
                applicationVersion: AppConstants.appVersion,
              ),
              icon: const Icon(Icons.description, size: AppTheme.iconSmall),
              label: const Text('View Licenses'),
              style: TextButton.styleFrom(
                padding: const EdgeInsets.symmetric(
                  horizontal: AppTheme.spacing10,
                  vertical: AppTheme.spacing4,
                ),
                minimumSize: Size.zero,
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
            const Spacer(),
            TextButton.icon(
              onPressed: () {
                Clipboard.setData(
                    const ClipboardData(text: 'https://github.com/${AppConstants.githubRepo}'));
              },
              icon: const Icon(Icons.open_in_new, size: AppTheme.iconSmall),
              label: const Text('GitHub'),
              style: TextButton.styleFrom(
                padding: const EdgeInsets.symmetric(
                  horizontal: AppTheme.spacing10,
                  vertical: AppTheme.spacing4,
                ),
                minimumSize: Size.zero,
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
          ],
        ),
      ],
    );
  }
}

class _LogSection extends ConsumerWidget {
  const _LogSection();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                'Log Viewer',
                style: theme.textTheme.labelMedium?.copyWith(
                  color: theme.colorScheme.onSurface.withValues(alpha: 0.6),
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            TextButton.icon(
              onPressed: () {
                final logs = AppLogger.recentLogs;
                if (logs.isNotEmpty) {
                  Clipboard.setData(ClipboardData(text: logs.join('\n')));
                }
              },
              icon: const Icon(Icons.copy, size: AppTheme.iconSmall),
              label: const Text('Copy'),
              style: TextButton.styleFrom(
                padding: const EdgeInsets.symmetric(
                  horizontal: AppTheme.spacing8,
                  vertical: AppTheme.spacing4,
                ),
                minimumSize: Size.zero,
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
          ],
        ),
        const SizedBox(height: AppTheme.spacing8),
        _LogViewer(logs: AppLogger.recentLogs),
      ],
    );
  }
}

class _LogViewer extends StatelessWidget {
  const _LogViewer({required this.logs});

  final List<String> logs;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final displayLogs =
        logs.length > 100 ? logs.sublist(logs.length - 100) : logs;

    return Container(
      width: double.infinity,
      height: 120,
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        border: Border.all(
          color: theme.dividerColor.withValues(alpha: 0.2),
          width: 0.5,
        ),
      ),
      padding: const EdgeInsets.all(AppTheme.spacing8),
      child: displayLogs.isEmpty
          ? Center(
              child: Text(
                'No log entries yet',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurface.withValues(alpha: 0.4),
                ),
              ),
            )
          : Scrollbar(
              child: SingleChildScrollView(
                child: SelectableText(
                  displayLogs.join('\n'),
                  style: TextStyle(
                    fontFamily: 'monospace',
                    fontSize: 11,
                    color: theme.colorScheme.onSurface
                        .withValues(alpha: 0.8),
                    height: 1.4,
                  ),
                ),
              ),
            ),
    );
  }
}

class _SettingRow extends StatelessWidget {
  const _SettingRow({
    required this.label,
    required this.subtitle,
    required this.child,
  });

  final String label;
  final String subtitle;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Row(
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: theme.textTheme.bodyMedium,
              ),
              Text(
                subtitle,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurface.withValues(alpha: 0.5),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: AppTheme.spacing8),
        child,
      ],
    );
  }
}
