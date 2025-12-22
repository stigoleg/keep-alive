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
- **Systemd**: Uses `systemd-inhibit` (preferred).
- **Desktop DBus**: Native inhibition for GNOME, KDE, XFCE, and MATE.
- **Active Status**: Optionally uses `xdotool` to simulate periodic mouse movement.

## Dependencies

### Runtime Dependencies

- **Linux**:
  - `dbus-send` or `gdbus` (typically pre-installed)
  - `xdotool` (optional, for `--active` simulation)
  - A terminal that supports TUI applications

### Build Dependencies

- Go 1.25 or later

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License.
