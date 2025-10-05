# Keep-Alive

A lightweight, cross-platform utility to prevent your system from going to sleep. Perfect for maintaining active connections, downloads, or any process that requires your system to stay awake.

[![GitHub release (latest by date)](https://img.shields.io/github/v/release/stigoleg/keep-alive)](https://github.com/stigoleg/keep-alive/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/stigoleg/keep-alive)](https://goreportcard.com/report/github.com/stigoleg/keep-alive)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

![Keep-Alive Demo](docs/demo.gif)

## Features

- 🔄 Configurable keep-alive duration
- 💻 Cross-platform support (macOS, Windows, Linux)
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

### Command-Line Options

```
Flags:
    -d, --duration string   Duration to keep system alive (e.g., "2h30m" or "150")
    -c, --clock string     Time to keep system alive until (e.g., "22:00" or "10:00PM")
    -v, --version          Show version information
    -h, --help            Show help message
```


### Man Page

A minimal man page is included as `man/keepalive.1` in the archives:

```bash
sudo mkdir -p /usr/local/share/man/man1
sudo cp man/keepalive.1 /usr/local/share/man/man1/
sudo mandb || true
man keepalive
```

The duration can be specified in two ways:
1. Using the duration flag (-d/--duration):
   - As a time duration (e.g., "2h30m", "1h", "45m")
   - As minutes (e.g., "150" for 2.5 hours)

2. Using the clock flag (-c/--clock):
   - 24-hour format: "HH:MM" (e.g., "22:00", "09:45")
   - 12-hour format: "HH:MM[AM|PM]" (e.g., "11:30PM", "9:45 AM")
   - If the specified time is in the past, it's assumed to be for the next day

Note: You cannot use both duration and clock flags at the same time.

### Examples:
```bash
keepalive                    # Start with interactive TUI
keepalive -d 2h30m          # Keep system awake for 2 hours and 30 minutes
keepalive -d 150            # Keep system awake for 150 minutes
keepalive -c 22:00          # Keep system awake until 10:00 PM
keepalive -c 10:00PM        # Keep system awake until 10:00 PM
keepalive --version         # Show version information
```

### Interactive Mode

1. Start the application without flags to enter interactive mode:
```bash
keepalive
```

2. Use arrow keys (↑/↓) or j/k to navigate the menu
3. Press Enter to select an option
4. Press q or Esc to quit

## How It Works

Keep-Alive uses platform-specific APIs and techniques to prevent your system from entering sleep mode:

### macOS
- Uses the `caffeinate` command with multiple flags to prevent:
  - System sleep (`-s`)
  - Display sleep (`-d`)
  - Disk idle sleep (`-m`)
  - System idle sleep (`-i`)
  - User activity simulation (`-u`)
- Periodically asserts user activity using both `caffeinate -u` and `pmset touch`
- Automatically restores system power settings on exit

### Windows
- Utilizes the Windows `SetThreadExecutionState` API with flags:
  - `ES_CONTINUOUS`: Maintain the current state
  - `ES_SYSTEM_REQUIRED`: Prevent system sleep
  - `ES_DISPLAY_REQUIRED`: Prevent display sleep
- Implements a PowerShell-based fallback mechanism for additional reliability
- Restores default power settings on exit

### Linux
- Primary method: Uses `systemd-inhibit` to prevent:
  - System idle
  - Sleep
  - Lid switch actions
- Fallback methods if systemd is not available:
  - `xset` commands to disable screen saver and DPMS
  - GNOME settings modifications for idle prevention
- Automatically restores all system settings on exit

The application is built with reliability in mind:
1. **Process Monitoring**: Continuously monitors the keep-alive processes and automatically restarts them if they fail
2. **Graceful Cleanup**: Ensures all processes are properly terminated and system settings are restored on exit
3. **Resource Efficiency**: Uses minimal system resources while maintaining effectiveness

The UI provides three main options:
1. Keep system awake indefinitely
2. Keep system awake for a specified duration
3. Quit the application

When running with a timer, the application shows a countdown of the remaining time. You can stop the keep-alive at any time by pressing Enter to return to the menu or q/Esc to quit the application.

## Dependencies

### Runtime Dependencies

- **Linux**:
  - systemd (recommended) or X11
  - A terminal that supports TUI applications

### Build Dependencies

- Go 1.21 or later

## Building from Source

1. Clone the repository:
```bash
git clone https://github.com/stigoleg/keep-alive.git
cd keep-alive
```

2. Build the binary:
```bash
go build -o keepalive ./cmd/keepalive
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - The terminal UI framework that powers the interactive interface
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Provides the beautiful styling for the terminal UI
- [x/sys](https://pkg.go.dev/golang.org/x/sys) - Go packages for making system calls, especially useful for the Windows implementation

This project builds upon these excellent tools and APIs to provide a reliable, cross-platform solution for keeping your system awake.

## License

This project is licensed under the MIT License.
