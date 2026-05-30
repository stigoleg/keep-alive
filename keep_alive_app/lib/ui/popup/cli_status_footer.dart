import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/download_state.dart';
import '../../providers/cli_binary_provider.dart';
import '../theme/app_theme.dart';

class CliStatusFooter extends ConsumerWidget {
  const CliStatusFooter({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final binaryState = ref.watch(cliBinaryProvider);
    final theme = Theme.of(context);

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppTheme.spacing12,
        vertical: AppTheme.spacing8,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              _versionWidget(binaryState, theme),
              const Spacer(),
              _actionWidget(context, binaryState, ref, theme),
            ],
          ),
          if (binaryState.status == DownloadStatus.downloading) ...[
            const SizedBox(height: AppTheme.spacing6),
            LinearProgressIndicator(
              value: binaryState.progress > 0 ? binaryState.progress : null,
            ),
          ],
          if (binaryState.status == DownloadStatus.error &&
              binaryState.errorMessage != null) ...[
            const SizedBox(height: AppTheme.spacing4),
            Text(
              binaryState.errorMessage!,
              style: theme.textTheme.bodySmall?.copyWith(
                color: AppTheme.errorColor,
              ),
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
            ),
          ],
        ],
      ),
    );
  }

  Widget _versionWidget(DownloadState state, ThemeData theme) {
    final version = state.installedVersion;

    Widget icon;
    String text;

    switch (state.status) {
      case DownloadStatus.installed:
        icon = const Icon(Icons.check_circle,
            size: AppTheme.iconSmall, color: AppTheme.activeColor);
        text = 'CLI v$version';
        break;
      case DownloadStatus.downloading:
        icon = SizedBox(
          width: AppTheme.iconSmall,
          height: AppTheme.iconSmall,
          child: CircularProgressIndicator(
            strokeWidth: 2,
            value: state.progress > 0 ? state.progress : null,
          ),
        );
        text = 'Downloading\u2026';
        break;
      case DownloadStatus.error:
        icon = const Icon(Icons.error,
            size: AppTheme.iconSmall, color: AppTheme.errorColor);
        text = 'CLI error';
        break;
      case DownloadStatus.notInstalled:
        icon = Icon(Icons.cloud_download,
            size: AppTheme.iconSmall,
            color: theme.colorScheme.onSurface.withValues(alpha: 0.4));
        text = 'CLI not installed';
        break;
    }

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        icon,
        const SizedBox(width: AppTheme.spacing6),
        Text(
          text,
          style: theme.textTheme.bodySmall?.copyWith(
            color: theme.colorScheme.onSurface.withValues(alpha: 0.7),
          ),
        ),
      ],
    );
  }

  Widget _actionWidget(
      BuildContext context, DownloadState state, WidgetRef ref, ThemeData theme) {
    switch (state.status) {
      case DownloadStatus.notInstalled:
      case DownloadStatus.error:
        return TextButton.icon(
          onPressed: () => ref.read(cliBinaryProvider.notifier).downloadLatest(),
          icon: const Icon(Icons.download, size: AppTheme.iconSmall),
          label: Text(
            state.status == DownloadStatus.error ? 'Retry' : 'Download CLI',
            style: theme.textTheme.labelMedium,
          ),
          style: TextButton.styleFrom(
            padding: const EdgeInsets.symmetric(
              horizontal: AppTheme.spacing8,
              vertical: AppTheme.spacing4,
            ),
            minimumSize: Size.zero,
            tapTargetSize: MaterialTapTargetSize.shrinkWrap,
          ),
        );
      case DownloadStatus.downloading:
        return Text(
          '${(state.progress * 100).round()}%',
          style: theme.textTheme.bodySmall,
        );
      case DownloadStatus.installed:
        return TextButton.icon(
          onPressed: () async {
            final hasUpdate = await ref
                .read(cliBinaryProvider.notifier)
                .checkForUpdate();
            if (hasUpdate) {
              if (context.mounted) {
                ref.read(cliBinaryProvider.notifier).downloadLatest();
              }
            }
          },
          icon: const Icon(Icons.refresh, size: AppTheme.iconSmall),
          label: Text(
            state.latestVersion != state.installedVersion
                ? 'Update'
                : 'Reinstall',
            style: theme.textTheme.labelMedium,
          ),
          style: TextButton.styleFrom(
            padding: const EdgeInsets.symmetric(
              horizontal: AppTheme.spacing8,
              vertical: AppTheme.spacing4,
            ),
            minimumSize: Size.zero,
            tapTargetSize: MaterialTapTargetSize.shrinkWrap,
          ),
        );
    }
  }
}
