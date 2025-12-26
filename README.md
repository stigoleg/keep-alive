# Keep-Alive

A lightweight, cross-platform utility to prevent your system from going to sleep. Perfect for maintaining active connections, downloads, or any process that requires your system to stay awake.

[![GitHub release (latest by date)](https://img.shields.io/github/v/release/stigoleg/keep-alive)](https://github.com/stigoleg/keep-alive/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/stigoleg/keep-alive)](https://goreportcard.com/report/github.com/stigoleg/keep-alive)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

![Keep-Alive Demo](docs/demo.gif)

## Features

- 🔄 Configurable keep-alive duration
- 💻 Cross-platform support (macOS, Windows, Linux)
- 💬 **Active Status Simulation** (optional, for Slack/Teams)
- ⚡ Lightweight and efficient
- 🎯 Simple and intuitive to use
- 🛠 Zero configuration required

## Installation

Download the latest binary for your platform from the [GitHub releases page](https://github.com/stigoleg/keep-alive/releases/latest), or install via package managers below.

### Homebrew (macOS/Linux)

```bash
brew tap stigoleg/homebrew-tap
brew install keepalive
```

### Scoop (Windows)

```powershell
scoop bucket add stigoleg https://github.com/stigoleg/scoop-bucket.git
scoop install keepalive
```

### macOS and Linux

1. Download the archive for your platform:
```bash
# For macOS:
curl -LO https://github.com/stigoleg/keep-alive/releases/latest/download/keep-alive_Darwin_x86_64.tar.gz

# For Linux:
curl -LO https://github.com/stigoleg/keep-alive/releases/latest/download/keep-alive_Linux_x86_64.tar.gz
```

2. Extract the archive:
```bash
tar xzf keep-alive_*_x86_64.tar.gz
```

3. Move the binary to a location in your PATH:
```bash
sudo mv keepalive /usr/local/bin/
```

### Windows

1. Download the Windows archive from the [releases page](https://github.com/stigoleg/keep-alive/releases/latest)
2. Extract the archive
3. Move `keepalive.exe` to your desired location
4. (Optional) Add the location to your PATH environment variable

## Usage

### Interactive Mode

1. Start the application without major flags to enter interactive mode:
```bash
keepalive
```

2. Use arrow keys (↑/↓) or j/k to navigate the menu.
3. **Toggle Active Status**: Press `a` to toggle activity simulation (Slack/Teams).
4. Press Enter to select an option.
5. Press q or Esc to quit.

### Command-Line Options

```
Flags:
    -d, --duration string   Duration to keep system alive (e.g., "2h30m" or "150")
    -c, --clock string     Time to keep system alive until (e.g., "22:00" or "10:00PM")
    -a, --active           Keep chat apps (Slack/Teams) active by simulating activity
    -v, --version          Show version information
    -h, --help            Show help message
```

### Examples:
```bash
keepalive                    # Start with interactive TUI
keepalive --active           # Start with active status simulation
keepalive -d 2h30m --active  # Keep system/Slack awake for 2.5 hours
keepalive -c 17:00           # Keep system awake until 5 PM
```

## How It Works

Keep-Alive uses platform-specific APIs and techniques to prevent your system from entering sleep mode:

### macOS
- Uses the `caffeinate` command with multiple flags (`-s`, `-d`, `-m`, `-i`, `-u`).
- Periodically asserts user activity using `pmset touch`.
- **Active Status**: Optionally jitters the mouse by 1 pixel via native scripting to maintain application-level activity.

### Windows
- Utilizes the Windows `SetThreadExecutionState` API.
- **Active Status**: Optionally uses the native `SendInput` API to simulate tiny, non-intrusive mouse movements.
- Restores default power settings on exit.

### Linux
Keep-Alive uses a multi-layered approach:
- **Systemd**: Uses `systemd-inhibit` (preferred, works on all systems).
- **Desktop DBus**: Native inhibition for Cosmic (Pop OS), GNOME, KDE, XFCE, and MATE.
- **gsettings**: For GNOME-based desktops (including Cosmic).
- **Active Status**: Uses multiple methods with automatic fallback:
  - **uinput** (native, works on both X11 and Wayland, requires permissions)
  - **ydotool** (recommended for Wayland, works on X11 too)
  - **wtype** (Wayland-native, limited mouse support)
  - **xdotool** (X11 only)

## Dependencies

### Runtime Dependencies

- **Linux**:
  - `dbus-send` or `gdbus` (typically pre-installed)
  - `systemd-inhibit` (typically pre-installed on systemd-based systems)
  - **For mouse simulation (`--active` flag)**:
    - `ydotool` (recommended, works on both X11 and Wayland): `sudo apt install ydotool` (Debian/Ubuntu) or equivalent
    - `xdotool` (X11 only): `sudo apt install xdotool` (Debian/Ubuntu) or equivalent
    - `wtype` (Wayland only, limited support): Install from your distribution's repository
    - Native uinput (requires proper permissions, see Troubleshooting)
  - A terminal that supports TUI applications

### Build Dependencies

- Go 1.25 or later
- **Library Dependencies:**
  - [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
  - [Bubbles](https://github.com/charmbracelet/bubbles) - UI components (textinput, timer, progress, help)
  - [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
  - [Testify](https://github.com/stretchr/testify) - Testing assertions
  - [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) - Windows syscall interop

## Troubleshooting

### Linux

#### Sleep Prevention Not Working

**Pop OS Cosmic / GNOME-based desktops:**
- Ensure `systemd-inhibit` is available: `which systemd-inhibit`
- Check DBus services: `dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames | grep -i session`
- For Cosmic, the application automatically detects and uses the GNOME session manager

**General Linux:**
- Check system logs: `journalctl -xe | grep keep-alive`
- Verify inhibitors are active: Check the application's debug log (`debug.log`)
- If using systemd, ensure the service is running: `systemctl status`

#### Mouse Simulation Not Working

**Permission Issues (uinput):**
If you see permission errors for `/dev/uinput`, you have two options:

1. **Add user to input group** (recommended):
   ```bash
   sudo usermod -aG input $USER
   ```
   Then **log out and log back in** for changes to take effect.

2. **Create udev rule**:
   ```bash
   echo 'KERNEL=="uinput", MODE="0664", GROUP="input"' | sudo tee /etc/udev/rules.d/99-uinput.rules
   sudo udevadm control --reload-rules
   sudo udevadm trigger
   ```

**Wayland vs X11:**
- **Wayland**: Install `ydotool` for best compatibility: `sudo apt install ydotool` (Debian/Ubuntu) or equivalent
- **X11**: `xdotool` works: `sudo apt install xdotool` (Debian/Ubuntu) or equivalent
- Check your display server: `echo $XDG_SESSION_TYPE` or `echo $WAYLAND_DISPLAY`
- If on Wayland without `ydotool`, the application will fall back to DBus simulation (less effective)

**Missing Dependencies:**
- The application will log warnings if required tools are missing
- Check `debug.log` for specific dependency recommendations
- Install missing tools based on your display server (see Dependencies section)

#### Pop OS Cosmic Specific Notes

- Cosmic is automatically detected and uses GNOME session manager
- Works with both Wayland and X11 sessions
- For best mouse simulation on Wayland, install `ydotool`
- If sleep prevention fails, check that `org.gnome.SessionManager` is available via DBus

#### Debugging

- Check the debug log: `cat debug.log` or `tail -f debug.log`
- Look for diagnostic messages starting with `linux: === Startup Diagnostics ===`
- Verify detected desktop environment and display server match your system
- Check which inhibitors and mouse simulation methods are active

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License.
