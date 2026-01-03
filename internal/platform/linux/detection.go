//go:build linux

// Package linux provides the Linux-specific implementation of the keep-alive functionality.
package linux

import (
	"bufio"
	"os"
	"strings"
)

// Display server types.
const (
	DisplayServerWayland = "wayland"
	DisplayServerX11     = "x11"
	DisplayServerUnknown = "unknown"
)

// Desktop environment types.
const (
	DesktopCosmic  = "cosmic"
	DesktopGNOME   = "gnome"
	DesktopKDE     = "kde"
	DesktopXFCE    = "xfce"
	DesktopMATE    = "mate"
	DesktopUnknown = "unknown"
)

// Capabilities tracks available tools and system information for the Linux platform.
type Capabilities struct {
	XdotoolAvailable    bool
	XprintidleAvailable bool
	UinputAvailable     bool
	YdotoolAvailable    bool
	WtypeAvailable      bool
	DisplayServer       string
	DesktopEnvironment  string
}

// DetectCapabilities detects available tools and system configuration.
func DetectCapabilities() Capabilities {
	displayServer := DetectDisplayServer()
	// xprintidle only works on X11, not Wayland
	xprintidleAvailable := hasCommand("xprintidle") && displayServer == DisplayServerX11
	return Capabilities{
		XdotoolAvailable:    hasCommand("xdotool"),
		XprintidleAvailable: xprintidleAvailable,
		UinputAvailable:     true, // Will be tested during setup
		YdotoolAvailable:    hasCommand("ydotool"),
		WtypeAvailable:      hasCommand("wtype"),
		DisplayServer:       displayServer,
		DesktopEnvironment:  DetectDesktopEnvironment(),
	}
}

// DetectDesktopEnvironment detects the current desktop environment.
func DetectDesktopEnvironment() string {
	xdgDesktop := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	desktopSession := strings.ToLower(os.Getenv("DESKTOP_SESSION"))

	// Check for Cosmic (Pop OS)
	if strings.Contains(xdgDesktop, DesktopCosmic) || strings.Contains(xdgDesktop, "pop") ||
		strings.Contains(desktopSession, DesktopCosmic) || strings.Contains(desktopSession, "pop") {
		return DesktopCosmic
	}

	// Check for GNOME
	if strings.Contains(xdgDesktop, DesktopGNOME) || strings.Contains(desktopSession, DesktopGNOME) {
		return DesktopGNOME
	}

	// Check for KDE
	if strings.Contains(xdgDesktop, DesktopKDE) || strings.Contains(desktopSession, DesktopKDE) ||
		strings.Contains(xdgDesktop, "plasma") {
		return DesktopKDE
	}

	// Check for XFCE
	if strings.Contains(xdgDesktop, DesktopXFCE) || strings.Contains(desktopSession, DesktopXFCE) {
		return DesktopXFCE
	}

	// Check for MATE
	if strings.Contains(xdgDesktop, DesktopMATE) || strings.Contains(desktopSession, DesktopMATE) {
		return DesktopMATE
	}

	return DesktopUnknown
}

// DetectDisplayServer detects whether running on Wayland or X11.
func DetectDisplayServer() string {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return DisplayServerWayland
	}
	if os.Getenv("XDG_SESSION_TYPE") == DisplayServerWayland {
		return DisplayServerWayland
	}
	if os.Getenv("DISPLAY") != "" {
		return DisplayServerX11
	}
	if os.Getenv("XDG_SESSION_TYPE") == DisplayServerX11 {
		return DisplayServerX11
	}
	return DisplayServerUnknown
}

// DistroInfo contains information about the detected Linux distribution.
type DistroInfo struct {
	Name       string
	PkgManager string
}

// DetectDistribution detects the Linux distribution and package manager.
func DetectDistribution() DistroInfo {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return DistroInfo{Name: "unknown", PkgManager: detectPackageManager()}
	}
	defer file.Close()

	var id, idLike string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "ID_LIKE=") {
			idLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), "\"")
		}
	}

	distro := strings.ToLower(id)
	if distro == "" {
		distro = "unknown"
	}

	pkgManager := detectPackageManagerForDistro(distro, idLike)
	return DistroInfo{Name: distro, PkgManager: pkgManager}
}

func detectPackageManagerForDistro(distro, idLike string) string {
	switch {
	case distro == "debian" || distro == "ubuntu" || distro == "pop" ||
		strings.Contains(idLike, "debian") || strings.Contains(idLike, "ubuntu"):
		return "apt"
	case distro == "fedora" || distro == "rhel" || distro == "centos" ||
		strings.Contains(idLike, "fedora") || strings.Contains(idLike, "rhel"):
		if hasCommand("dnf") {
			return "dnf"
		}
		return "yum"
	case distro == "arch" || distro == "manjaro" || strings.Contains(idLike, "arch"):
		return "pacman"
	case distro == "opensuse" || distro == "opensuse-leap" || distro == "opensuse-tumbleweed" ||
		strings.Contains(idLike, "suse"):
		return "zypper"
	case distro == "alpine":
		return "apk"
	default:
		return detectPackageManager()
	}
}

func detectPackageManager() string {
	managers := []string{"apt", "dnf", "yum", "pacman", "zypper", "apk"}
	for _, m := range managers {
		if hasCommand(m) {
			return m
		}
	}
	return "unknown"
}
