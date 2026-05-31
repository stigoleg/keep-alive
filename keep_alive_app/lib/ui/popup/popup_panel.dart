import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import '../../utils/platform_utils.dart';
import '../theme/app_theme.dart';
import 'cli_status_footer.dart';
import 'status_header.dart';
import 'toggle_section.dart';

class PopupPanel extends ConsumerStatefulWidget {
  const PopupPanel({
    super.key,
    this.onOpenSettings,
    this.minWindowHeight,
    this.resizeRevision = 0,
  });

  final VoidCallback? onOpenSettings;
  final double? minWindowHeight;
  final int resizeRevision;

  @override
  ConsumerState<PopupPanel> createState() => _PopupPanelState();
}

class _PopupPanelState extends ConsumerState<PopupPanel> {
  final GlobalKey _contentKey = GlobalKey();
  double? _lastHeight;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _syncWindowSize());
  }

  @override
  void didUpdateWidget(PopupPanel oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.minWindowHeight != widget.minWindowHeight ||
        oldWidget.resizeRevision != widget.resizeRevision) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _syncWindowSize());
    }
  }

  void _syncWindowSize() {
    final renderBox =
        _contentKey.currentContext?.findRenderObject() as RenderBox?;
    if (renderBox == null || !renderBox.hasSize) return;

    final contentHeight = renderBox.size.height;
    if (contentHeight <= 0) return;

    var targetHeight = contentHeight;
    final minWindowHeight = widget.minWindowHeight;
    if (minWindowHeight != null && targetHeight < minWindowHeight) {
      targetHeight = minWindowHeight;
    }
    if (_lastHeight != null && (targetHeight - _lastHeight!).abs() < 0.5) {
      return;
    }
    _lastHeight = targetHeight;

    windowManager
        .getBounds()
        .then((bounds) {
          // Anchor the top edge to the menu bar — only resize, never reposition.
          // setSize on macOS/Win/Linux preserves the top-left origin.
          windowManager.setSize(Size(bounds.size.width, targetHeight));
        })
        .catchError((_) {});
  }

  bool _onSizeChanged(SizeChangedLayoutNotification _) {
    WidgetsBinding.instance.addPostFrameCallback((_) => _syncWindowSize());
    return false;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    // On macOS, keep the surface semi-transparent so the NSVisualEffectView /
    // Liquid Glass underneath shows through (native menu-bar popover feel).
    final surfaceColor = PlatformUtils.isMacOS
        ? theme.colorScheme.surface.withValues(alpha: 0.55)
        : theme.colorScheme.surface;

    return Scaffold(
      backgroundColor: Colors.transparent,
      body: Container(
        decoration: BoxDecoration(
          color: surfaceColor,
          borderRadius: BorderRadius.circular(AppTheme.radiusMedium),
        ),
        clipBehavior: Clip.antiAlias,
        child: SingleChildScrollView(
          child: NotificationListener<SizeChangedLayoutNotification>(
            onNotification: _onSizeChanged,
            child: SizeChangedLayoutNotifier(
              child: Column(
                key: _contentKey,
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  StatusHeader(onOpenSettings: widget.onOpenSettings),
                  _buildDivider(theme),
                  const ToggleSection(),
                  _buildDivider(theme),
                  const CliStatusFooter(),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildDivider(ThemeData theme) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: AppTheme.spacing12),
      child: Divider(
        height: 1,
        thickness: 0.5,
        color: theme.dividerColor.withValues(alpha: 0.3),
      ),
    );
  }
}
