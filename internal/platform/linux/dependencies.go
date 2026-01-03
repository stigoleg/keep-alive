//go:build linux

package linux

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// DependencyInfo contains information about a missing dependency and how to install it.
type DependencyInfo struct {
	Name        string
	WhyNeeded   string
	InstallCmd  string
	Optional    bool
	Available   bool
	Alternative string
}

// getPackageName returns the package name for a tool on a specific distribution.
func getPackageName(tool string) string {
	tool = strings.ToLower(tool)
	switch tool {
	case "ydotool", "xdotool", "wtype", "xprintidle":
		return tool
	default:
		return ""
	}
}

// GenerateInstallCommand generates a distro-specific installation command for the given tool.
func GenerateInstallCommand(tool string, distro DistroInfo) (cmd string, note string) {
	if tool == "" {
		return "", "Tool name is required"
	}

	pkgName := getPackageName(tool)
	if pkgName == "" {
		return "", fmt.Sprintf("Package name not available for tool '%s'", tool)
	}

	switch distro.PkgManager {
	case "apt":
		cmd = fmt.Sprintf("sudo apt update && sudo apt install %s", pkgName)
		if tool == "ydotool" {
			note = "Note: ydotool may not be in default Ubuntu/Debian repos. You may need to build from source or use a PPA."
		}
	case "dnf", "yum":
		cmd = fmt.Sprintf("sudo %s install %s", distro.PkgManager, pkgName)
		if tool == "ydotool" {
			note = "Note: ydotool may not be in default Fedora/RHEL repos. You may need to build from source."
		}
	case "pacman":
		cmd = fmt.Sprintf("sudo pacman -S %s", pkgName)
		if tool == "ydotool" {
			note = "Note: ydotool is available in AUR. Install with: yay -S ydotool (or use your AUR helper)"
		}
	case "zypper":
		cmd = fmt.Sprintf("sudo zypper install %s", pkgName)
	case "apk":
		cmd = fmt.Sprintf("sudo apk add %s", pkgName)
	default:
		cmd = fmt.Sprintf("Install %s using your distribution's package manager", pkgName)
		note = fmt.Sprintf("Package name: %s. Check your distribution's repositories.", pkgName)
	}

	return cmd, note
}

// CheckMissingDependencies checks which dependencies are missing and returns installation information.
func CheckMissingDependencies(caps Capabilities, hasUinput bool) []DependencyInfo {
	var missing []DependencyInfo
	distro := DetectDistribution()

	// Check ydotool (recommended for Wayland, works on X11 too)
	if !caps.YdotoolAvailable {
		installCmd, note := GenerateInstallCommand("ydotool", distro)
		whyNeeded := "Provides reliable mouse simulation on both X11 and Wayland (recommended)"
		if caps.DisplayServer == DisplayServerWayland {
			whyNeeded = "Provides reliable mouse simulation on Wayland display server (highly recommended)"
		}
		alt := "Use uinput instead (requires permissions: sudo usermod -aG input $USER, then logout/login)"
		if !hasUinput {
			alt = "Setup uinput permissions: sudo usermod -aG input $USER (then logout/login)"
		}
		dep := DependencyInfo{
			Name:        "ydotool",
			WhyNeeded:   whyNeeded,
			InstallCmd:  installCmd,
			Optional:    true,
			Available:   true,
			Alternative: alt,
		}
		if note != "" {
			dep.Alternative = note + "\n" + alt
		}
		missing = append(missing, dep)
	}

	// Check xdotool (X11 only)
	if caps.DisplayServer == DisplayServerX11 && !caps.XdotoolAvailable {
		installCmd, _ := GenerateInstallCommand("xdotool", distro)
		alt := "Not needed if using Wayland or if uinput/ydotool is configured"
		if !hasUinput && !caps.YdotoolAvailable {
			alt = "Alternative: Install ydotool (works on both X11 and Wayland) or setup uinput"
		}
		missing = append(missing, DependencyInfo{
			Name:        "xdotool",
			WhyNeeded:   "Provides mouse simulation on X11 display server",
			InstallCmd:  installCmd,
			Optional:    true,
			Available:   true,
			Alternative: alt,
		})
	}

	// Check xprintidle (X11 only, optional - used for idle detection)
	if caps.DisplayServer == DisplayServerX11 && !caps.XprintidleAvailable {
		installCmd, _ := GenerateInstallCommand("xprintidle", distro)
		missing = append(missing, DependencyInfo{
			Name:        "xprintidle",
			WhyNeeded:   "Provides idle time detection on X11 (optional, activity simulation works without it)",
			InstallCmd:  installCmd,
			Optional:    true,
			Available:   true,
			Alternative: "Not needed on Wayland or if you don't need idle detection",
		})
	}

	return missing
}

// FormatDependencyMessages formats dependency information into user-friendly messages.
func FormatDependencyMessages(missing []DependencyInfo, displayServer string, hasUinput bool) string {
	if len(missing) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("═══════════════════════════════════════════════════════════\n")
	b.WriteString("  Missing Optional Dependencies Detected\n")
	b.WriteString("═══════════════════════════════════════════════════════════\n")
	b.WriteString("\n")
	b.WriteString("The following dependencies are recommended for optimal mouse simulation:\n")
	b.WriteString("\n")

	for i, dep := range missing {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, dep.Name))
		b.WriteString(fmt.Sprintf("   Why needed: %s\n", dep.WhyNeeded))
		b.WriteString(fmt.Sprintf("   Install with: %s\n", dep.InstallCmd))
		if dep.Alternative != "" {
			b.WriteString(fmt.Sprintf("   Alternative: %s\n", dep.Alternative))
		}
		b.WriteString("\n")
	}

	b.WriteString("Note: The app will work without these dependencies, but mouse\n")
	b.WriteString("simulation may be limited. DBus simulation will be used as fallback.\n")
	b.WriteString("\n")
	if !hasUinput {
		b.WriteString("Tip: Setting up uinput permissions provides native mouse simulation\n")
		b.WriteString("     without external dependencies. Run: sudo usermod -aG input $USER\n")
		b.WriteString("     (then logout and login again)\n")
		b.WriteString("\n")
	}
	b.WriteString("═══════════════════════════════════════════════════════════\n")

	return b.String()
}

// getInputGroupGID looks up the "input" group GID by parsing /etc/group.
func getInputGroupGID() int {
	file, err := os.Open("/etc/group")
	if err != nil {
		return -1
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[0] == "input" {
			if gid, err := strconv.Atoi(parts[2]); err == nil {
				return gid
			}
		}
	}
	return -1
}

// CheckUinputPermissions checks if uinput is accessible and returns a user-friendly error message if not.
func CheckUinputPermissions() (hasAccess bool, errorMessage string) {
	const uinputDevicePath = "/dev/uinput"

	if _, err := os.Stat(uinputDevicePath); os.IsNotExist(err) {
		return false, "uinput device not found: /dev/uinput does not exist. The uinput kernel module may not be loaded. Try: sudo modprobe uinput"
	}

	f, err := os.OpenFile(uinputDevicePath, os.O_WRONLY, 0)
	if err != nil {
		userGroups, err := os.Getgroups()
		if err == nil {
			inputGID := getInputGroupGID()
			if inputGID != -1 {
				hasInputGroup := false
				for _, gid := range userGroups {
					if gid == inputGID {
						hasInputGroup = true
						break
					}
				}
				if !hasInputGroup {
					return false, "uinput permission denied. Add your user to the 'input' group:\n  sudo usermod -aG input $USER\nThen log out and log back in for changes to take effect.\n\nAlternatively, create a udev rule:\n  echo 'KERNEL==\"uinput\", MODE=\"0664\", GROUP=\"input\"' | sudo tee /etc/udev/rules.d/99-uinput.rules\n  sudo udevadm control --reload-rules\n  sudo udevadm trigger"
				}
			}
		}
		return false, fmt.Sprintf("uinput permission denied: %v\n\nTo fix:\n1. Add user to input group: sudo usermod -aG input $USER (then logout/login)\n2. Or create udev rule: echo 'KERNEL==\"uinput\", MODE=\"0664\", GROUP=\"input\"' | sudo tee /etc/udev/rules.d/99-uinput.rules", err)
	}
	f.Close()
	return true, ""
}

// GetDependencyMessage returns the formatted dependency message if dependencies are missing.
func GetDependencyMessage() string {
	caps := DetectCapabilities()
	hasUinput, _ := CheckUinputPermissions()
	missingDeps := CheckMissingDependencies(caps, hasUinput)
	if len(missingDeps) > 0 {
		msg := FormatDependencyMessages(missingDeps, caps.DisplayServer, hasUinput)
		log.Printf("linux: missing dependencies detected:\n%s", msg)
		return msg
	}
	return ""
}
