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

Download the latest binary for your platform from the [GitHub releases page](https://github.com/stigoleg/keep-alive/releases/latest).

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

1. Start the application:
```bash
keepalive
```

2. Use arrow keys (↑/↓) or j/k to navigate the menu
3. Press Enter to select an option
4. When entering minutes, use numbers only (e.g., "150" for 2.5 hours)
5. Press q or Esc to quit

## How It Works

Keep-Alive uses platform-specific APIs to prevent your system from entering sleep mode:

- **macOS**: Uses the `caffeinate` command to prevent system and display sleep
- **Windows**: Uses SetThreadExecutionState to prevent system sleep
- **Linux**: Uses systemd-inhibit to prevent the system from going idle/sleep

The application provides three main options:
1. Keep system awake indefinitely
2. Keep system awake for X minutes (enter the number of minutes)
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

## License

This project is licensed under the MIT License.

## Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - The TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions for terminal applications