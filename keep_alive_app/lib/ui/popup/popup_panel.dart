import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import '../theme/app_theme.dart';
import 'battery_section.dart';
import 'cli_status_footer.dart';
import 'status_header.dart';
import 'toggle_section.dart';

class PopupPanel extends ConsumerStatefulWidget {
  const PopupPanel({super.key, this.onOpenSettings});

  final VoidCallback? onOpenSettings;

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

  void _syncWindowSize() {
    final renderBox =
        _contentKey.currentContext?.findRenderObject() as RenderBox?;
    if (renderBox == null || !renderBox.hasSize) return;

    final contentHeight = renderBox.size.height;
    if (contentHeight <= 0) return;

    final targetHeight = contentHeight + 16;
    if (_lastHeight != null && (targetHeight - _lastHeight!).abs() < 0.5) {
      return;
    }
    _lastHeight = targetHeight;

    windowManager.getBounds().then((bounds) {
      final dyChange = bounds.size.height - targetHeight;
      windowManager.setSize(Size(bounds.size.width, targetHeight));
      windowManager.setPosition(Offset(
        bounds.left,
        bounds.top + dyChange,
      ));
    }).catchError((_) {});
  }

  bool _onSizeChanged(SizeChangedLayoutNotification _) {
    WidgetsBinding.instance.addPostFrameCallback((_) => _syncWindowSize());
    return false;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      backgroundColor: Colors.transparent,
      body: Container(
        decoration: BoxDecoration(
          color: theme.colorScheme.surface,
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
                  const BatterySection(),
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
