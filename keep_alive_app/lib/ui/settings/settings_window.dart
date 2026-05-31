import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/constants.dart';
import '../../core/logger.dart';
import '../../models/download_state.dart';
import '../../platform/platform_interface.dart';
import '../../providers/cli_binary_provider.dart';
import '../../providers/session_provider.dart';
import '../../providers/settings_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/toggle_switch.dart';

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
        ToggleSwitch(
          label: 'Start on Login',
          description: 'Launch automatically when you log in',
          value: settings.autoStart,
          onChanged: (value) => _setAutoStart(value, ref),
        ),
        ToggleSwitch(
          label: 'Start Minimized',
          description: 'Hide to system tray on launch',
          value: settings.startMinimized,
          onChanged: (value) =>
              ref.read(appSettingsProvider.notifier).setStartMinimized(value),
        ),
        ToggleSwitch(
          label: 'Show Countdown in Menu Bar',
          description: 'Display remaining time in the menu bar icon',
          value: settings.showCountdownInMenuBar,
          onChanged: (value) => ref
              .read(appSettingsProvider.notifier)
              .setShowCountdownInMenuBar(value),
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
        '${binaryState.installedVersion ?? 'unknown'} installed',
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

class _LogSection extends ConsumerStatefulWidget {
  const _LogSection();

  @override
  ConsumerState<_LogSection> createState() => _LogSectionState();
}

class _LogSectionState extends ConsumerState<_LogSection> {
  String? _activeFilter;
  final TextEditingController _searchController = TextEditingController();
  String _searchText = '';

  static const _filters = <String, String?>{
    'All': null,
    'Debug': 'FINE',
    'Info': 'INFO',
    'Warning': 'WARNING',
    'Error': 'SEVERE',
  };

  static const int _displayCap = 500;

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  List<String> _resolveLogs() {
    final base = _activeFilter != null
        ? AppLogger.filteredLogs(_activeFilter)
        : AppLogger.recentLogs;
    if (_searchText.isEmpty) return base;
    final needle = _searchText.toLowerCase();
    return base.where((line) => line.toLowerCase().contains(needle)).toList();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final settings = ref.watch(appSettingsProvider);
    final logs = _resolveLogs();
    final displayLogs = logs.length > _displayCap
        ? logs.sublist(logs.length - _displayCap)
        : logs;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Logging',
          style: theme.textTheme.labelMedium?.copyWith(
            color: theme.colorScheme.onSurface.withValues(alpha: 0.6),
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: AppTheme.spacing4),
        ToggleSwitch(
          label: 'Enable Logging',
          description: 'Write debug output to log file',
          value: settings.enableLogging,
          onChanged: (value) {
            ref.read(appSettingsProvider.notifier).setEnableLogging(value);
            ref.read(sessionProvider).applySettingsAndRestart();
          },
        ),
        const SizedBox(height: AppTheme.spacing8),
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
                setState(() {
                  AppLogger.clearLogs();
                });
              },
              icon: const Icon(Icons.delete_outline, size: AppTheme.iconSmall),
              label: const Text('Clear'),
              style: TextButton.styleFrom(
                padding: const EdgeInsets.symmetric(
                  horizontal: AppTheme.spacing8,
                  vertical: AppTheme.spacing4,
                ),
                minimumSize: Size.zero,
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
            TextButton.icon(
              onPressed: () {
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
        const SizedBox(height: AppTheme.spacing6),
        SizedBox(
          height: 32,
          child: TextField(
            controller: _searchController,
            onChanged: (value) => setState(() => _searchText = value),
            style: theme.textTheme.bodySmall,
            decoration: InputDecoration(
              isDense: true,
              hintText: 'Search logs',
              prefixIcon: const Icon(Icons.search, size: AppTheme.iconSmall),
              prefixIconConstraints: const BoxConstraints(
                minWidth: 32,
                minHeight: 32,
              ),
              suffixIcon: _searchText.isEmpty
                  ? null
                  : IconButton(
                      icon: const Icon(Icons.close, size: AppTheme.iconSmall),
                      padding: EdgeInsets.zero,
                      constraints: const BoxConstraints(
                        minWidth: 28,
                        minHeight: 28,
                      ),
                      onPressed: () {
                        _searchController.clear();
                        setState(() => _searchText = '');
                      },
                    ),
              contentPadding: const EdgeInsets.symmetric(
                vertical: AppTheme.spacing4,
              ),
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
              ),
            ),
          ),
        ),
        const SizedBox(height: AppTheme.spacing6),
        Wrap(
          spacing: AppTheme.spacing4,
          children: _filters.entries.map((entry) {
            final selected = _activeFilter == entry.value;
            return SizedBox(
              height: 28,
              child: ChoiceChip(
                label: Text(
                  entry.key,
                  style: const TextStyle(fontSize: 11),
                ),
                selected: selected,
                onSelected: (_) {
                  setState(() => _activeFilter = entry.value);
                },
                selectedColor: theme.colorScheme.primaryContainer,
                labelStyle: TextStyle(
                  color: selected
                      ? theme.colorScheme.onPrimaryContainer
                      : theme.colorScheme.onSurface.withValues(alpha: 0.7),
                  fontWeight: selected ? FontWeight.w600 : FontWeight.normal,
                ),
                padding: const EdgeInsets.symmetric(horizontal: AppTheme.spacing8),
                materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                visualDensity: VisualDensity.compact,
              ),
            );
          }).toList(),
        ),
        const SizedBox(height: AppTheme.spacing8),
        _LogViewer(logs: displayLogs),
      ],
    );
  }
}

class _LogViewer extends StatefulWidget {
  const _LogViewer({required this.logs});

  final List<String> logs;

  @override
  State<_LogViewer> createState() => _LogViewerState();
}

class _LogViewerState extends State<_LogViewer> {
  final ScrollController _controller = ScrollController();
  bool _autoTail = true;

  @override
  void initState() {
    super.initState();
    _controller.addListener(_onScroll);
    WidgetsBinding.instance.addPostFrameCallback((_) => _scrollToEndIfTailing());
  }

  @override
  void didUpdateWidget(_LogViewer old) {
    super.didUpdateWidget(old);
    if (old.logs.length != widget.logs.length) {
      WidgetsBinding.instance
          .addPostFrameCallback((_) => _scrollToEndIfTailing());
    }
  }

  @override
  void dispose() {
    _controller.removeListener(_onScroll);
    _controller.dispose();
    super.dispose();
  }

  void _onScroll() {
    if (!_controller.hasClients) return;
    final atBottom = _controller.position.pixels >=
        _controller.position.maxScrollExtent - 12;
    if (atBottom != _autoTail) {
      setState(() => _autoTail = atBottom);
    }
  }

  void _scrollToEndIfTailing() {
    if (!_autoTail || !_controller.hasClients) return;
    _controller.jumpTo(_controller.position.maxScrollExtent);
  }

  Color _colorForLine(ThemeData theme, String line) {
    if (line.contains('[SEVERE]')) return AppTheme.errorColor;
    if (line.contains('[WARNING]')) return AppTheme.warningColor;
    if (line.contains('[INFO]')) {
      return theme.colorScheme.onSurface.withValues(alpha: 0.85);
    }
    return theme.colorScheme.onSurface.withValues(alpha: 0.55);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Container(
      width: double.infinity,
      height: 240,
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(AppTheme.radiusSmall),
        border: Border.all(
          color: theme.dividerColor.withValues(alpha: 0.2),
          width: 0.5,
        ),
      ),
      padding: const EdgeInsets.all(AppTheme.spacing8),
      child: widget.logs.isEmpty
          ? Center(
              child: Text(
                'No log entries match',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurface.withValues(alpha: 0.4),
                ),
              ),
            )
          : Scrollbar(
              controller: _controller,
              child: SingleChildScrollView(
                controller: _controller,
                child: SelectableText.rich(
                  TextSpan(
                    style: const TextStyle(
                      fontFamily: 'monospace',
                      fontSize: 12,
                      height: 1.4,
                    ),
                    children: [
                      for (var i = 0; i < widget.logs.length; i++) ...[
                        TextSpan(
                          text: widget.logs[i],
                          style: TextStyle(
                            color: _colorForLine(theme, widget.logs[i]),
                          ),
                        ),
                        if (i < widget.logs.length - 1)
                          const TextSpan(text: '\n'),
                      ],
                    ],
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
