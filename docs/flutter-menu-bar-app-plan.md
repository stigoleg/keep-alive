# KeepAlive Flutter Menu Bar App — Architecture & Implementation Plan

## 1. Project Overview

**Goal:** Build a native-feeling, cross-platform menu bar / system tray application using Flutter that wraps the existing KeepAlive Go CLI binary. The app lives in the system tray, has minimal resource footprint, and allows users to toggle keep-alive features without a terminal.

**Key principles:**
- **Battery & resource efficient** — runs quietly in the background.
- **Native look & feel** — macOS menu bar app, Windows system tray, Linux system tray (adapting to DE conventions).
- **Self-updating backend** — on first launch (and optionally on demand), downloads the latest KeepAlive Go binary for the current platform from GitHub Releases.
- **Graceful degradation** — the Flutter UI works even without the Go binary installed; features that require the binary are simply disabled with a clear message.
- **Clean lifecycle** — Flutter app exits cleanly, terminating the Go subprocess reliably on quit or OS shutdown.

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────┐
│  Flutter Menu Bar App (Dart)                    │
│                                                  │
│  ┌──────────────┐  ┌──────────────────────────┐ │
│  │  UI Layer     │  │  Service Layer            │ │
│  │  (Widgets)    │  │                           │ │
│  │               │  │  ┌─────────────────────┐  │ │
│  │  - TrayIcon   │  │  │ CliDownloadService  │  │ │
│  │  - TrayMenu   │  │  │  - GitHub Releases  │  │ │
│  │  - PopupPanel │  │  │  - Download archive  │  │ │
│  │  - Settings   │  │  │  - Extract & verify  │  │ │
│  │               │  │  └─────────────────────┘  │ │
│  └──────┬───────┘  │  ┌─────────────────────┐  │ │
│         │          │  │ ProcessManager       │  │ │
│  ┌──────┴───────┐  │  │  - Start/Stop CLI   │  │ │
│  │  State        │  │  │  - Monitor health   │  │ │
│  │  Management   │  │  │  - Signal handling  │  │ │
│  │  (Riverpod)   │  │  │  - Stdout capture   │  │ │
│  │               │  │  └─────────────────────┘  │ │
│  │  - cliState   │  │  ┌─────────────────────┐  │ │
│  │  - settings   │  │  │ SettingsRepository  │  │ │
│  │  - download   │  │  │  - Persist prefs    │  │ │
│  │  - session    │  │  │  - shared_prefs     │  │ │
│  └──────────────┘  │  └─────────────────────┘  │ │
│                     │  ┌─────────────────────┐  │ │
│                     │  │ BatteryMonitor      │  │ │
│                     │  │  - Poll battery %   │  │ │
│                     │  │  - Trigger stop     │  │ │
│                     │  └─────────────────────┘  │ │
│                     └──────────────────────────┘ │
│                                                  │
│  ┌──────────────────────────────────────────┐   │
│  │  Platform Channels (MethodChannel)        │   │
│  │  - Native system tray APIs                │   │
│  │  - Auto-start registration                │   │
│  │  - Platform-specific behavior             │   │
│  └──────────────────────────────────────────┘   │
└─────────────────────┬───────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│  KeepAlive Go CLI Binary (subprocess)           │
│  keepalive [flags] --log                        │
│                                                  │
│  Flags: -d <duration> | -c <clock> |            │
│         -b <battery%> | -a | -l                 │
└─────────────────────────────────────────────────┘
```

### Data Flow

```
User toggles switch in tray menu popup
         │
         ▼
Riverpod state updated (cliStateProvider)
         │
         ├──► ProcessManager restarts CLI with new flags
         │         │
         │         ▼
         │    Go binary spawns as subprocess
         │         │
         │         ├── stdout/stderr → log buffer (ring buffer, max 1000 lines)
         │         └── exit code → cliStateProvider updates status
         │
         └──► UI reactively updates (switch states, status indicators, remaining time)
```

---

## 3. Component Tree & Responsibility

```
App (MaterialApp / CupertinoApp)
└── SystemTrayShell (root — no visible window)
    ├── TrayIconManager          — Platform-specific tray icon & right-click menu
    │   ├── tray icon asset      — @2x, @3x PNG icons per platform
    │   ├── tooltip              — "KeepAlive — System Active" or "KeepAlive — Idle"
    │   └── native context menu  — Show/Hide, Start, Stop, Quit
    │
    ├── PopupPanel (Window)      — The floating menu bar popup
    │   ├── StatusHeader         — Running state, time remaining, battery %
    │   ├── ToggleSection
    │   │   ├── KeepAwakeToggle      — On/Off (maps to CLI start/stop)
    │   │   ├── ActivitySimToggle    — On/Off (maps to --active flag)
    │   │   └── LoggingToggle        — On/Off (maps to --log flag)
    │   ├── TimerSection
    │   │   ├── DurationPicker       — Hours + minutes selector
    │   │   └── ClockTimePicker      — "Until 17:00" type picker
    │   ├── BatterySection
    │   │   └── BatteryThresholdSlider — 1–100% slider
    │   ├── CliStatusFooter       — CLI binary version, health, download status
    │   └── ActionButtons         — Download/Update CLI, Quit
    │
    └── SettingsWindow (separate window)
        ├── AutoStartToggle       — Launch on login
        ├── StartMinimizedToggle  — Start hidden in tray
        └── AboutSection          — Version, licenses, GitHub link
```

---

## 4. Platform-Specific Design Requirements

### macOS (Menu Bar App)
- **Tray**: `NSStatusBar` via platform channel — icon in menu bar (template image, monochrome).
- **Popup**: `NSPopover`-like behavior — clicking tray icon shows a floating panel anchored to the menu bar. Clicking elsewhere dismisses it.
- **Menu**: Right-click on tray icon shows native `NSMenu` with Quit/Show options.
- **Window**: `LSUIElement = YES` in `Info.plist` (no Dock icon). Accessory window level (`NSFloatingWindowLevel`).
- **Design**: Follow macOS HIG — rounded corners, translucent blur background (`NSVisualEffectView`), compact spacing.
- **Auto-start**: `LSBackgroundOnly` + LaunchAgent plist.

### Windows (System Tray)
- **Tray**: `Shell_NotifyIcon` — icon in system tray notification area.
- **Popup**: Click tray icon → show borderless Flutter window positioned near tray. Dismiss on focus loss.
- **Menu**: Right-click tray icon → native context menu (Show, Exit).
- **Window**: No taskbar entry (`WS_EX_TOOLWINDOW`). Always-on-top while open.
- **Design**: Follow Windows 11 Fluent Design — sharp corners or subtle rounding, acrylic/mica background, Segoe UI font.
- **Auto-start**: Registry key `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.

### Linux (System Tray)
- **Tray**: Freedesktop `StatusNotifierItem` (KDE) with `libappindicator` / `AyatanaAppIndicator` fallback for GNOME/XFCE/MATE.
- **Popup**: Click tray icon → floating window near tray. Dismiss on focus loss.
- **Menu**: Right-click → native context menu (Show, Quit).
- **Window**: `_NET_WM_WINDOW_TYPE_DOCK` or equivalent. Skip taskbar/pager.
- **Design**: Adapt to DE — GNOME/libadwaita styling, KDE/Breeze styling. Use `gtk` theme colors where possible.
- **Auto-start**: `~/.config/autostart/keepalive.desktop` file.

---

## 5. Technology Choices

| Concern | Choice | Rationale |
|---------|--------|-----------|
| **Framework** | Flutter 3.x (stable) | Cross-platform desktop from single codebase |
| **State management** | `flutter_riverpod` | Compile-safe, testable, minimal rebuilds |
| **System tray** | `system_tray` + native channels | Cross-platform tray support with native fallbacks |
| **Window management** | `window_manager` | Control window level, transparency, blur, position |
| **Process management** | `dart:io` Process | Spawn Go CLI, capture stdout/stderr, send signals |
| **Settings persistence** | `shared_preferences` | Lightweight key-value store for toggle states |
| **HTTP client** | `dio` | Download CLI binary with progress tracking |
| **Archive extraction** | `archive` | Extract tar.gz (Linux/macOS) and zip (Windows) |
| **App paths** | `path_provider` | Platform-appropriate data directories |
| **Icons** | `flutter_launcher_icons` | Generate platform-specific app icons |
| **Logging** | `logging` + ring buffer | In-memory log ring for CLI output |
| **Testing** | `flutter_test` + `mockito` | Unit, widget, and integration tests |

---

## 6. State Design (Riverpod Providers)

```dart
// ── CLI Binary State ──────────────────────────────
// Whether the Go binary is installed and its version
@riverpod
class CliBinaryState extends _$CliBinaryState {
  // States: notInstalled, downloading(progress%), installed(version), error(msg)
}

// ── Session State ─────────────────────────────────
// Current keep-alive session configuration
@riverpod
class SessionConfig extends _$SessionConfig {
  // Fields: isRunning, durationMinutes, clockTime, batteryThreshold,
  //         simulateActivity, enableLogging
}

// ── Process State ──────────────────────────────────
// The running Go CLI subprocess status
@riverpod
class CliProcessState extends _$CliProcessState {
  // States: idle, starting, running(pid), stopping, error(msg)
  // Derived: timeRemaining, batteryPercent
}

// ── Settings (persisted) ──────────────────────────
@riverpod
class AppSettings extends _$AppSettings {
  // Fields: autoStart, startMinimized, lastDuration, lastBatteryThreshold
}

// ── Battery State (platform) ──────────────────────
@riverpod
Stream<BatteryInfo> batteryState(BatteryStateRef ref) {
  // Platform channel → native battery API
  // Emits: percentage, isCharging, isPresent
}
```

---

## 7. Detailed Task Breakdown

---

### Task 1: Flutter Project Scaffolding & Tooling Setup

**Description:**
Initialize a new Flutter project inside the repository (monorepo style), configure all target platforms, set up code generation (Riverpod), and establish the linting/formatting toolchain.

**Steps:**
1. Run `flutter create --org com.stigoleg --platforms macos,windows,linux keep_alive_app` in repo root.
2. Add dependencies to `pubspec.yaml`:
   ```yaml
   dependencies:
     flutter_riverpod: ^2.x
     riverpod_annotation: ^2.x
     window_manager: ^0.4.x
     system_tray: ^2.x
     shared_preferences: ^2.x
     path_provider: ^2.x
     dio: ^5.x
     archive: ^3.x  # or ^4.x
     logging: ^1.x
     flutter_launcher_icons: ^0.14.x

   dev_dependencies:
     flutter_test:
       sdk: flutter
     riverpod_generator: ^2.x
     build_runner: ^2.x
     mockito: ^5.x
     flutter_lints: ^5.x
   ```
3. Configure `build_runner` for Riverpod code generation.
4. Set up `analysis_options.yaml` with strict lint rules.
5. Configure `flutter_launcher_icons` in `pubspec.yaml` for all platforms.
6. Create project directory structure:
   ```
   keep_alive_app/
   ├── lib/
   │   ├── main.dart
   │   ├── app.dart
   │   ├── core/
   │   │   ├── constants.dart        # App constants
   │   │   ├── exceptions.dart       # Custom exceptions
   │   │   └── logger.dart           # Logging setup
   │   ├── models/
   │   │   ├── cli_flags.dart        # CLI flag model
   │   │   ├── battery_info.dart     # Battery info model
   │   │   └── github_release.dart   # GitHub API models
   │   ├── services/
   │   │   ├── cli_download_service.dart
   │   │   ├── process_manager.dart
   │   │   ├── battery_monitor.dart
   │   │   └── github_api_service.dart
   │   ├── repositories/
   │   │   └── settings_repository.dart
   │   ├── providers/
   │   │   ├── cli_binary_provider.dart
   │   │   ├── session_provider.dart
   │   │   ├── process_provider.dart
   │   │   ├── settings_provider.dart
   │   │   └── battery_provider.dart
   │   ├── platform/
   │   │   ├── platform_interface.dart
   │   │   ├── platform_macos.dart
   │   │   ├── platform_windows.dart
   │   │   └── platform_linux.dart
   │   ├── ui/
   │   │   ├── theme/
   │   │   │   ├── app_theme.dart
   │   │   │   ├── macos_theme.dart
   │   │   │   ├── windows_theme.dart
   │   │   │   └── linux_theme.dart
   │   │   ├── tray/
   │   │   │   ├── tray_manager.dart
   │   │   │   └── tray_menu.dart
   │   │   ├── popup/
   │   │   │   ├── popup_panel.dart
   │   │   │   ├── status_header.dart
   │   │   │   ├── toggle_section.dart
   │   │   │   ├── timer_section.dart
   │   │   │   ├── battery_section.dart
   │   │   │   └── cli_status_footer.dart
   │   │   ├── settings/
   │   │   │   └── settings_window.dart
   │   │   └── widgets/
   │   │       ├── toggle_switch.dart
   │   │       ├── duration_picker.dart
   │   │       └── battery_slider.dart
   │   └── utils/
   │       ├── platform_utils.dart
   │       └── format_utils.dart
   ├── assets/
   │   ├── icons/
   │   │   ├── tray_icon.png
   │   │   ├── tray_icon_active.png
   │   │   └── tray_icon@2x.png
   │   └── images/
   ├── test/
   │   ├── unit/
   │   │   ├── services/
   │   │   └── providers/
   │   ├── widget/
   │   └── integration/
   ├── macos/
   ├── windows/
   ├── linux/
   └── pubspec.yaml
   ```
7. Verify `flutter analyze` passes clean.
8. Verify `flutter test` passes (initial default tests).

**Desired result:** A compilable Flutter project scaffold with all dependencies, tooling, and directory structure in place. Riverpod codegen produces expected `.g.dart` files.

---

### Task 2: Core Models & Constants

**Description:**
Define all data models shared across the app, plus app-wide constants (CLI binary name, GitHub repo URL, download paths, etc.).

**Deliverables:**
- `lib/core/constants.dart` — `githubRepo`, `cliBinaryName`, `cliDownloadBaseUrl`, update check interval, battery poll interval.
- `lib/models/cli_flags.dart` — Immutable model representing all CLI flags the Go binary accepts:
  ```dart
  class CliFlags {
    final int? durationMinutes;       // --duration
    final DateTime? clockTime;        // --clock
    final int? batteryThreshold;      // --battery
    final bool simulateActivity;      // --active
    final bool enableLogging;         // --log
    // Method: List<String> toArgs() => builds CLI argument list
  }
  ```
- `lib/models/battery_info.dart` — `BatteryInfo(percentage: double, isCharging: bool, isPresent: bool)`.
- `lib/models/github_release.dart` — `GitHubRelease(tagName, assets: [ReleaseAsset(name, downloadUrl, size)])`.
- `lib/models/cli_process_state.dart` — `enum CliProcessState { idle, starting, running, stopping, error }` and a wrapper class with PID, start time, exit code.
- `lib/models/download_state.dart` — `enum DownloadState { notInstalled, downloading, installed, error }` with progress percentage.

**Desired result:** All models defined with `==` / `hashCode` overrides, `copyWith`, JSON serialization, and `toString`. No business logic in models.

---

### Task 3: Settings Repository & Persistence

**Description:**
Implement persistent storage for user preferences using `shared_preferences`. This stores toggle states, last-used values, and auto-start preference so they survive app restarts.

**Deliverables:**
- `lib/repositories/settings_repository.dart`:
  - `Future<void> setKeepAwake(bool value)`
  - `Future<bool> getKeepAwake()`
  - `Future<void> setSimulateActivity(bool value)`
  - `Future<bool> getSimulateActivity()`
  - `Future<void> setEnableLogging(bool value)`
  - `Future<bool> getEnableLogging()`
  - `Future<void> setBatteryThreshold(int? value)`
  - `Future<int?> getBatteryThreshold()`
  - `Future<void> setDurationMinutes(int? value)`
  - `Future<int?> getDurationMinutes()`
  - `Future<void> setAutoStart(bool value)`
  - `Future<bool> getAutoStart()`
  - `Future<void> setStartMinimized(bool value)`
  - `Future<bool> getStartMinimized()`
- `lib/providers/settings_provider.dart` — Riverpod `Notifier` that wraps `SettingsRepository`, exposes reactive settings.
- Unit tests verifying save/load roundtrip.

**Desired result:** Settings persist across app restarts. Riverpod provider notifies listeners on changes.

---

### Task 4: GitHub API Service & CLI Download Manager

**Description:**
Implement the service that communicates with the GitHub Releases API to discover, download, extract, and verify the latest KeepAlive Go binary for the current platform.

**Key behaviors:**
- On first launch: check if CLI binary exists in app data dir → if not, download latest release.
- "Update CLI" button: check GitHub for newer version → download if newer.
- Map platform/arch to correct release asset name (e.g., `keep-alive_Darwin_arm64.tar.gz`).
- Extract archive, place binary in app data directory (`path_provider`'s `getApplicationSupportDirectory()`).
- Set executable permissions on macOS/Linux (`chmod +x`).
- Verify binary works by running `keepalive --version` and parsing output.

**Deliverables:**
- `lib/services/github_api_service.dart`:
  - `Future<GitHubRelease> getLatestRelease()` — calls `https://api.github.com/repos/stigoleg/keep-alive/releases/latest`.
  - `String getAssetNameForCurrentPlatform()` — maps Dart's `Platform.operatingSystem` + arch to release asset name.
- `lib/services/cli_download_service.dart`:
  - `Future<String> ensureCliInstalled()` — checks existence, downloads if missing.
  - `Future<void> downloadLatest(String assetUrl, {void Function(double progress)? onProgress})`.
  - `Future<void> extractArchive(String archivePath, String targetDir)` — handles tar.gz and zip.
  - `Future<String?> getInstalledVersion()` — parses `--version` output.
  - `Future<bool> isUpdateAvailable()` — compares installed vs latest GitHub version.
- `lib/providers/cli_binary_provider.dart` — Riverpod `Notifier` managing download state, progress, installed version.
- Unit tests with mocked HTTP client.
- Error handling: network failures, disk full, corrupt archive, permission denied.

**Desired result:** On app startup, the latest CLI binary is automatically downloaded and verified. Download progress is observable via Riverpod. Errors are surfaced gracefully in the UI.

---

### Task 5: Process Manager (Go CLI Lifecycle)

**Description:**
Implement the service that spawns, monitors, and terminates the KeepAlive Go CLI binary as a subprocess. This is the core bridge between Flutter UI and the backend.

**Key behaviors:**
- Build argument list from `CliFlags.toArgs()` and spawn `keepalive [args] --log`.
- The CLI is always started with `--log` so debug output can be captured.
- Monitor stdout/stderr via line-by-line streaming (ring buffer, max 1000 lines).
- Detect process exit (normal, error, killed) and update state.
- Handle platform-specific process signals: `SIGTERM` (Unix), `taskkill` (Windows).
- On stop: send graceful termination signal, wait up to 5 seconds, force kill if still alive.
- Validate CLI binary path exists and is executable before spawning.
- Prevent spawning multiple instances (check if already running).
- Rebuild and restart the CLI process when `CliFlags` change (stop old, start new).

**Deliverables:**
- `lib/services/process_manager.dart`:
  - `Future<void> start(CliFlags flags)` — spawns subprocess.
  - `Future<void> stop()` — graceful termination.
  - `Future<void> restart(CliFlags flags)` — stop + start.
  - `Stream<String> get stdoutStream` — captured output.
  - `Stream<String> get stderrStream` — captured errors.
  - `bool get isRunning`.
  - `int? get pid`.
  - `void dispose()` — cleanup on app exit.
- `lib/providers/process_provider.dart` — Riverpod provider bridging process state to UI.
- Unit tests (mocking `Process`). Integration test (actual binary).
- Signal handling:
  - macOS/Linux: Process sends `SIGTERM` (15) → wait 5s → `SIGKILL` (9).
  - Windows: `taskkill /PID <pid>` → wait 5s → `taskkill /F /PID <pid>`.

**Desired result:** CLI subprocess lifecycle is fully managed. UI reflects running/stopped state reactively. Clean shutdown on app exit.

---

### Task 6: Platform Channels — Native System Tray APIs

**Description:**
Implement the platform channel layer that provides native system tray functionality. Each platform has its own implementation that Flutter communicates with via `MethodChannel`.

**macOS platform channel (`lib/platform/platform_macos.dart`):**
- `setTrayIcon(String iconPath)` → sets `NSStatusBar` button image (template).
- `setTrayTooltip(String tooltip)` → sets tooltip text.
- `showContextMenu(List<String> items)` → right-click menu with callbacks.
- `showPopover(Offset position)` / `hidePopover()`.
- `setAutoStart(bool enabled)` → creates/removes LaunchAgent plist in `~/Library/LaunchAgents/com.stigoleg.keepalive.plist`.
- Native implementation in `macos/Runner/AppDelegate.swift` using Cocoa APIs.
- Window: set `LSUIElement = true` in `Info.plist`, configure `NSFloatingWindowLevel` + `NSVisualEffectView` for popup.

**Windows platform channel (`lib/platform/platform_windows.dart`):**
- `setTrayIcon(String iconPath)` → `Shell_NotifyIcon(NIM_ADD)`.
- `setTrayTooltip(String tooltip)` → `NOTIFYICONDATA.szTip`.
- `showContextMenu(List<String> items)` → `TrackPopupMenu`.
- `setAutoStart(bool enabled)` → Registry `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.
- Native implementation in `windows/runner/win32_window.cpp` using Win32 APIs.
- Window: `WS_EX_TOOLWINDOW`, hide from taskbar when minimized to tray.

**Linux platform channel (`lib/platform/platform_linux.dart`):**
- `setTrayIcon(String iconPath)` → `StatusNotifierItem` + `libappindicator` fallback.
- `setTrayTooltip(String tooltip)`.
- `showContextMenu(List<String> items)`.
- `setAutoStart(bool enabled)` → writes `~/.config/autostart/keepalive.desktop`.
- Native implementation via D-Bus (`StatusNotifierItem` protocol) in C/C++.
- Window: set `_NET_WM_WINDOW_TYPE` to `_NET_WM_WINDOW_TYPE_UTILITY`.

**Desired result:** System tray icon appears on all platforms. Click shows popup, right-click shows native context menu. Auto-start works on all platforms.

---

### Task 7: Battery Monitor (Platform Channel)

**Description:**
Implement a platform channel that reads battery status natively. This is used both for the battery threshold feature (stop when battery drops to X%) and to display battery info in the UI.

**Deliverables:**
- `lib/services/battery_monitor.dart`:
  - `Stream<BatteryInfo> get batteryStream` — emits every 30 seconds.
  - `Future<BatteryInfo> getCurrentBattery()` — one-shot read.
- Platform channel `com.stigoleg.keepalive/battery`:
  - macOS: `IOKit` / `IORegistryEntryCreateCFProperty` for `CurrentCapacity` / `MaxCapacity`.
  - Windows: `GetSystemPowerStatus` syscall → `BATTERY_STATUS`.
  - Linux: Parse `/sys/class/power_supply/BAT*/capacity` and `status` files.
- `lib/providers/battery_provider.dart` — Riverpod stream provider.
- Unit tests with mocked platform channel.

**Desired result:** Battery percentage and charging state polled live. UI updates reactively. Used to honor battery threshold stop condition.

---

### Task 8: App Theme & Platform-Adaptive Styling

**Description:**
Implement the design system for the app with platform-adaptive theming. macOS looks like a native Mac menu bar popup, Windows follows Fluent Design, Linux adapts to current DE theme.

**Deliverables:**
- `lib/ui/theme/app_theme.dart` — base `ThemeData` with shared tokens (colors, typography, spacing).
- `lib/ui/theme/macos_theme.dart`:
  - Translucent background (`NSVisualEffectView` vibe via `BackdropFilter` blur).
  - Rounded corners (12px radius).
  - SF Pro / system font.
  - Compact spacing (8px padding).
  - Dark/Light mode support (follows system).
  - ~300px wide, auto-height popup.
- `lib/ui/theme/windows_theme.dart`:
  - Acrylic/mica background effect (via `window_manager`).
  - Slightly rounded corners (8px).
  - Segoe UI font.
  - `--toggle` styled as Windows 11 toggle switches.
  - ~320px wide.
- `lib/ui/theme/linux_theme.dart`:
  - Follows GTK theme colors (parsed from system or use `libadwaita` colors).
  - Adwaita-style toggle switches.
  - ~320px wide.

**Desired result:** The popup panel looks and feels native on each platform. Colors, fonts, and control styles match OS conventions.

---

### Task 9: System Tray Integration (Dart Side)

**Description:**
Wire up the system tray from the Dart side using the `system_tray` package and our platform channels. Manage icon states (idle vs active), handle click events, build the popup window.

**Deliverables:**
- `lib/ui/tray/tray_manager.dart`:
  - `Future<void> initialize()` — set up tray icon, register click handlers.
  - `void setActiveState(bool isActive)` — switch icon between idle/active assets.
  - `void updateTooltip(String text)`.
  - `void onLeftClick()` → toggle popup window visibility.
  - `void onRightClick()` → show native context menu.
- `lib/ui/tray/tray_menu.dart`:
  - Build context menu items: "Show KeepAlive", separator, "Quit".
  - Handle "Quit" → cleanup + `exit(0)`.
- Tray icon assets:
  - `assets/icons/tray_icon.png` — idle state (grayscale).
  - `assets/icons/tray_icon_active.png` — active state (colored indicator).
  - Include @2x and @3x variants for HiDPI.
- Platform-specific icon requirements:
  - macOS: Template image (PDF or black PNG with alpha, system tints it).
  - Windows: 16x16 and 32x32 ICO/PNG.
  - Linux: 22x22 and 48x48 PNG.

**Desired result:** Tray icon appears in OS menu bar / system tray. Left click shows popup. Right click shows native menu. Icon changes to indicate active state.

---

### Task 10: Popup Panel UI — Main Interface

**Description:**
Build the popup panel that appears when the user clicks the tray icon. This is the primary user interface for controlling KeepAlive features.

**Deliverables:**
- `lib/ui/popup/popup_panel.dart` — root widget for the popup window, handles dismiss-on-focus-loss.
- `lib/ui/popup/status_header.dart`:
  - Shows current state: "Idle" / "Active (2h 15m remaining)".
  - Battery percentage indicator with icon.
  - Animated pulse indicator when active.
- `lib/ui/popup/toggle_section.dart`:
  - **Keep System Awake** toggle switch — starts/stops the CLI process.
  - **Simulate Activity** toggle switch — toggles `--active` flag (restarts CLI if running).
  - **Enable Logging** toggle switch — toggles `--log` flag.
  - Each toggle shows its current state, has a label + description subtitle.
- `lib/ui/popup/timer_section.dart`:
  - Radio group: Indefinite / Duration / Clock Time.
  - Duration picker: increment/decrement hour/minute buttons (or compact `TimePicker`).
  - Clock time picker: input field with time validation + AM/PM selector.
  - Visible only when "Keep System Awake" is ON.
- `lib/ui/popup/battery_section.dart`:
  - Battery threshold slider (1–100%) or quick-select chips (20%, 30%, 50%).
  - "Stop when battery drops to X%" label.
  - Disabled state when current battery is below threshold (with warning).
- `lib/ui/popup/cli_status_footer.dart`:
  - Shows CLI binary version.
  - "Download CLI" / "Update CLI" button with progress indicator.
  - "CLI not installed" warning when binary is missing.
  - Link to log file with copy-path button.
- `lib/ui/widgets/toggle_switch.dart` — reusable styled toggle (platform-adaptive: macOS switch, Windows 11 toggle, Linux GTK switch).
- `lib/ui/widgets/duration_picker.dart` — compact hours + minutes selector.
- `lib/ui/widgets/battery_slider.dart` — styled slider with percentage label.

**Behavior rules:**
- Changing the "Keep Awake" toggle from ON→OFF stops the CLI and resets session state.
- Changing timer/battery settings while running causes a CLI restart with updated flags.
- If CLI is not installed, the "Keep Awake" toggle is disabled with a tooltip.
- The popup closes when clicking outside (focus loss on all platforms).
- Popup remembers its last configuration on re-open.

**Desired result:** A polished, native-feeling popup panel. All controls are responsive and properly wired to state. The UX matches platform conventions.

---

### Task 11: Settings Window

**Description:**
A secondary window for app-wide settings (auto-start, behavior preferences, about info).

**Deliverables:**
- `lib/ui/settings/settings_window.dart`:
  - **Start on Login** toggle — wired to platform channel auto-start.
  - **Start Minimized** toggle — launch hidden in tray.
  - **Check for Updates** button — triggers CLI download check.
  - **About section** — app version, Go CLI version, licenses, GitHub link.
  - **Log viewer** — scrollable text area showing last 100 lines of CLI output.
  - Opens from tray context menu "Preferences..." / "Settings".

**Desired result:** Clean settings window accessible from tray context menu.

---

### Task 12: App Lifecycle & Cleanup

**Description:**
Handle app startup sequence, graceful shutdown, and OS-level events (sleep, wake, shutdown). Ensure the Go subprocess is reliably terminated.

**Deliverables:**
- `lib/app.dart` — `App` widget that orchestrates startup:
  1. Initialize settings (load persisted prefs).
  2. Check for CLI binary (download if missing).
  3. Set up system tray.
  4. If `startMinimized`, don't show any window.
  5. If `autoStart` + last session was active, optionally auto-start the CLI.
  6. Register `WindowListener` for close events.
- Cleanup sequence (triggered on `WM_CLOSE`, `applicationShouldTerminate`, or `Quit` menu item):
  1. Send stop signal to CLI process (graceful → force kill).
  2. Wait for process exit (max 5 seconds).
  3. Remove system tray icon.
  4. Save any unsaved settings.
  5. Call `exit(0)`.
- `lib/main.dart`:
  - `WidgetsFlutterBinding.ensureInitialized()`.
  - Initialize `window_manager` and set window to hidden on start.
  - Run `ProviderScope(child: KeepAliveApp())`.
- OS event handling:
  - **macOS**: `applicationShouldTerminateAfterLastWindowClosed` → `false` (app stays in menu bar).
  - **Windows**: `WM_QUERYENDSESSION` → gracefully stop CLI, return `TRUE`.
  - **Linux**: `SIGTERM` handling → gracefully stop CLI.

**Desired result:** App starts silently in tray. Exiting the app (via Quit menu, Cmd+Q, or OS shutdown) cleanly terminates the Go CLI process. No zombie processes left behind.

---

### Task 13: Error Handling & Resilience

**Description:**
Implement comprehensive error handling across all services. Ensure the app degrades gracefully when the Go binary is unavailable, network is down, or platform features are unsupported.

**Deliverables:**
- Centralized error boundary widget wrapping the popup panel.
- `lib/core/exceptions.dart` — custom exception hierarchy:
  - `CliBinaryException` (not found, permission denied, wrong arch).
  - `CliProcessException` (failed to start, crash, timeout).
  - `DownloadException` (network error, checksum mismatch, disk full).
  - `PlatformException` (unsupported feature).
- Snackbar / inline error display in popup panel:
  - Red indicator dot on tray icon when CLI process crashes.
  - Error message in footer area with "Retry" button.
  - "CLI not available" banner when binary is missing (disables start toggle).
- Logger service (`lib/core/logger.dart`):
  - In-memory ring buffer (last 500 lines).
  - Optional file logging to app data directory.
  - Log levels: debug, info, warning, error.
- Network resilience:
  - CLI download with retry (3 attempts, exponential backoff).
  - Cache last successful download URL.
  - Offline mode: use cached binary, skip update check.

**Desired result:** App never crashes unexpectedly. All error states are communicated to the user with actionable recovery options. The tray icon and basic menu remain functional even when backend features are unavailable.

---

### Task 14: Unit & Widget Tests

**Description:**
Write comprehensive tests for all business logic and UI components.

**Deliverables:**
- **Unit tests** (`test/unit/`):
  - `cli_flags_test.dart` — `toArgs()` generates correct CLI arguments for all flag combinations.
  - `settings_repository_test.dart` — save/load roundtrip with mocked `SharedPreferences`.
  - `cli_download_service_test.dart` — mock HTTP, test asset name mapping, extraction, version parsing.
  - `process_manager_test.dart` — mock `Process`, test start/stop/restart lifecycle, timeout behavior.
  - `github_api_service_test.dart` — mock API responses, test parsing, error handling.
  - `battery_monitor_test.dart` — mock platform channel, test battery parsing.
- **Widget tests** (`test/widget/`):
  - `toggle_switch_test.dart` — tap toggles state, disabled state rendering.
  - `popup_panel_test.dart` — renders all sections, responds to provider state changes.
  - `status_header_test.dart` — displays correct status text and timer.
  - `battery_slider_test.dart` — drag updates value, renders percentage label.
- **Provider tests** (`test/providers/`):
  - Verify state transitions for `cliBinaryProvider`, `sessionProvider`, `processProvider`.
  - Test edge cases: double-start, stop-when-already-stopped, missing binary.
- **Integration tests** (`test/integration/`):
  - Full flow: download mock binary → start session → change flags → stop → verify cleanup.
  - Platform channel communication (with mock native side).
- Test coverage target: ≥80% for `lib/services/` and `lib/providers/`.

**Desired result:** All tests pass. CI pipeline runs `flutter test` on PR. Core logic is well-covered.

---

### Task 15: Build Configuration & CI/CD

**Description:**
Configure platform-specific build settings and set up CI for building release artifacts for all three platforms (macOS, Windows, Linux).

**Deliverables:**
- **macOS build config** (`macos/Runner/`):
  - `Info.plist`: `LSUIElement = YES`, bundle identifier, version, copyright.
  - `AppDelegate.swift`: system tray initialization, quit handling.
  - Entitlements: network client (for download), disable sandbox or add appropriate exceptions.
  - Code signing configuration for distribution (optional — notarization if desired).
  - DMG packaging via `flutter build macos` + `create-dmg`.
- **Windows build config** (`windows/runner/`):
  - `main.cpp`: hide console window, register WM messages.
  - `Runner.exe.manifest`: DPI awareness.
  - MSIX or NSIS installer packaging.
  - Build: `flutter build windows --release`.
- **Linux build config** (`linux/`):
  - CMake configuration.
  - `.desktop` file for launcher.
  - AppStream metadata.
  - Build: `flutter build linux --release`.
  - Package as AppImage / flatpak / deb.
- **CI workflow** (`.github/workflows/flutter-ci.yml`):
  - Triggers on PR to main, push to main.
  - Runs `flutter analyze`, `flutter test`, `flutter build <platform>` matrix.
  - Caches Flutter SDK and pub dependencies.
  - Uploads build artifacts.
- **Release workflow** (`.github/workflows/flutter-release.yml`):
  - Triggers on tag `flutter-v*`.
  - Builds all platforms.
  - Creates GitHub Release with platform-specific archives.
  - Optionally publishes to stores (App Store, Microsoft Store, Snap Store) — stretch goal.

**Desired result:** One command to build for any platform. CI validates every PR. Releases are automated.

---

### Task 16: Documentation

**Description:**
Write developer and user documentation for the Flutter menu bar app.

**Deliverables:**
- `keep_alive_app/README.md`:
  - App description and screenshots.
  - Installation instructions per platform.
  - Usage guide (how to use the menu bar app, all features).
  - How it works (wraps Go CLI, downloads automatically).
  - Troubleshooting (CLI not downloading, permissions, Linux tray issues).
  - Developer setup (Flutter SDK version, `flutter pub get`, `flutter run`).
- Inline code documentation:
  - All public classes/methods with `///` doc comments.
  - `/// {@template}` / `/// {@endtemplate}` for reusable docs.
- Architecture decision records (`docs/adr/`):
  - ADR-001: Why Flutter for desktop menu bar app.
  - ADR-002: Why Riverpod over BLoC/Provider.
  - ADR-003: Why spawn Go CLI as subprocess vs embedded Go via FFI.

**Desired result:** New developers can understand and contribute within 30 minutes. Users can install and use the app without confusion.

---

## 8. Implementation Order (Dependency Graph)

```
Phase 1 — Foundation
  Task 1  (Project Scaffolding)
  Task 2  (Core Models)
  Task 3  (Settings Repository)
         │
Phase 2 — Backend Bridge
  Task 4  (CLI Download Service)  ── depends on Task 2
  Task 5  (Process Manager)       ── depends on Task 2, 4
         │
Phase 3 — Platform Integration
  Task 6  (Platform Channels)     ── depends on Task 1
  Task 7  (Battery Monitor)       ── depends on Task 6
         │
Phase 4 — UI
  Task 8  (Theming)               ── depends on Task 1
  Task 9  (Tray Integration)      ── depends on Task 6, 8
  Task 10 (Popup Panel)           ── depends on Task 3, 8, 9
  Task 11 (Settings Window)       ── depends on Task 3, 8
         │
Phase 5 — Polish & Ship
  Task 12 (App Lifecycle)         ── depends on Task 5, 9, 10
  Task 13 (Error Handling)        ── depends on all above
  Task 14 (Testing)               ── depends on all above
  Task 15 (Build & CI/CD)         ── depends on Task 1
  Task 16 (Documentation)         ── depends on all above
```

---

## 9. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `system_tray` package limitations on Linux | Tray icon doesn't appear on some DEs | Have `libappindicator` and raw DBus fallbacks via platform channel |
| Flutter desktop still maturing on Linux | Rendering bugs, window manager issues | Test on GNOME, KDE, and XFCE. Use stable Flutter channel. Pin versions. |
| Go binary subprocess orphaned on crash | Battery drain, resource leak | Double-kill strategy (SIGTERM → SIGKILL). OS-level process group cleanup. Watchdog timer in Flutter. |
| GitHub API rate limiting | CLI download fails | Cache ETag, use conditional requests, bundle a minimal fallback binary, show manual download link. |
| macOS notarization for distribution | App shows "unidentified developer" warning | Set up notarization in CI (reuse existing GoReleaser notarization setup). |
| Large Flutter binary size (~50MB+) | Slow downloads, disk usage concerns | Use `--split-debug-info`, tree-shake icons, compress assets. Acceptable for a desktop app. |

---

## 10. Open Questions for the User

1. **Icon/Design assets**: Do you have a designer who will create tray icons and UI assets, or should I plan for placeholder icons to be replaced later?

2. **Application name**: Should the Flutter app be called "KeepAlive" (same as CLI) or have a distinct name like "KeepAlive Menu" or "KeepAlive Bar"?

3. **Go binary bundling vs download-only**: Should the app optionally bundle a fallback Go binary (increasing app size) for offline use, or strictly download-on-first-launch?

4. **Notification support**: Should the app show OS notifications when the keep-alive session ends (e.g., "KeepAlive: System sleep prevention stopped — 2h timer finished")?

5. **Multi-language support (i18n)**: Is internationalization needed in V1, or is English-only acceptable initially?

6. **Auto-update for the Flutter app itself**: Should the Flutter wrapper auto-update itself (e.g., via Sparkle on macOS, WinSparkle on Windows), or is manual download acceptable for V1?

7. **Repository structure**: Should the Flutter app live in this same repo (`keep-alive/keep_alive_app/` — monorepo) or in a separate repository?

8. **Target Flutter version**: Which Flutter version / channel should we target? Stable (3.x) or master?
