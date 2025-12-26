//go:build linux

package platform

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// inhibitor defines the common interface for various Linux sleep prevention methods.
type inhibitor interface {
	Name() string
	Activate(ctx context.Context) error
	Deactivate() error
}

// runVerbose executes a command, returns error and combined output (stdout+stderr)
func runVerbose(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// runBestEffort executes a command ignoring any errors (best-effort)
func runBestEffort(name string, args ...string) {
	if out, err := runVerbose(name, args...); err != nil {
		log.Printf("linux: best-effort command %s %s failed: %v (output: %q)", name, strings.Join(args, " "), err, out)
	}
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// detectDesktopEnvironment detects the current desktop environment
// Returns: "cosmic", "gnome", "kde", "xfce", "mate", "unknown"
func detectDesktopEnvironment() string {
	xdgDesktop := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	desktopSession := strings.ToLower(os.Getenv("DESKTOP_SESSION"))

	// Check for Cosmic (Pop OS)
	if strings.Contains(xdgDesktop, "cosmic") || strings.Contains(xdgDesktop, "pop") ||
		strings.Contains(desktopSession, "cosmic") || strings.Contains(desktopSession, "pop") {
		return "cosmic"
	}

	// Check for GNOME
	if strings.Contains(xdgDesktop, "gnome") || strings.Contains(desktopSession, "gnome") {
		return "gnome"
	}

	// Check for KDE
	if strings.Contains(xdgDesktop, "kde") || strings.Contains(desktopSession, "kde") ||
		strings.Contains(xdgDesktop, "plasma") {
		return "kde"
	}

	// Check for XFCE
	if strings.Contains(xdgDesktop, "xfce") || strings.Contains(desktopSession, "xfce") {
		return "xfce"
	}

	// Check for MATE
	if strings.Contains(xdgDesktop, "mate") || strings.Contains(desktopSession, "mate") {
		return "mate"
	}

	return "unknown"
}

// detectDisplayServer detects whether running on Wayland or X11
// Returns: "wayland", "x11", or "unknown"
func detectDisplayServer() string {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return "wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}
	if os.Getenv("XDG_SESSION_TYPE") == "x11" {
		return "x11"
	}
	return "unknown"
}

// detectLinuxDistribution detects the Linux distribution and package manager by parsing /etc/os-release.
// It supports major distributions including Debian/Ubuntu, Fedora/RHEL, Arch, openSUSE, and Alpine.
// Returns: (distribution name, package manager command, error)
// If detection fails, returns "unknown" for distribution and package manager.
func detectLinuxDistribution() (string, string, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "unknown", "", fmt.Errorf("failed to read /etc/os-release: %v", err)
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

	if err := scanner.Err(); err != nil {
		return "unknown", "", fmt.Errorf("failed to parse /etc/os-release: %v", err)
	}

	// Normalize distribution name
	distro := strings.ToLower(id)
	if distro == "" {
		distro = "unknown"
	}

	// Determine package manager based on distribution
	var pkgManager string
	switch {
	case distro == "debian" || distro == "ubuntu" || distro == "pop" || strings.Contains(idLike, "debian") || strings.Contains(idLike, "ubuntu"):
		pkgManager = "apt"
	case distro == "fedora" || distro == "rhel" || distro == "centos" || strings.Contains(idLike, "fedora") || strings.Contains(idLike, "rhel"):
		// Check if dnf is available, fallback to yum
		if hasCommand("dnf") {
			pkgManager = "dnf"
		} else {
			pkgManager = "yum"
		}
	case distro == "arch" || distro == "manjaro" || strings.Contains(idLike, "arch"):
		pkgManager = "pacman"
	case distro == "opensuse" || distro == "opensuse-leap" || distro == "opensuse-tumbleweed" || strings.Contains(idLike, "suse"):
		pkgManager = "zypper"
	case distro == "alpine":
		pkgManager = "apk"
	default:
		// Try to detect package manager by checking which commands are available
		if hasCommand("apt") {
			pkgManager = "apt"
		} else if hasCommand("dnf") {
			pkgManager = "dnf"
		} else if hasCommand("yum") {
			pkgManager = "yum"
		} else if hasCommand("pacman") {
			pkgManager = "pacman"
		} else if hasCommand("zypper") {
			pkgManager = "zypper"
		} else if hasCommand("apk") {
			pkgManager = "apk"
		} else {
			pkgManager = "unknown"
		}
	}

	return distro, pkgManager, nil
}

// getPackageName returns the package name for a tool on a specific distribution.
// Package names are typically consistent across distributions, but availability may vary.
func getPackageName(tool string, distro string) string {
	tool = strings.ToLower(tool)
	// distro parameter is kept for potential future distro-specific variations

	switch tool {
	case "ydotool", "xdotool", "wtype", "xprintidle":
		// Package names are consistent across distributions
		return tool
	default:
		return ""
	}
}

// generateInstallCommand generates a distro-specific installation command for the given tool.
// Returns: (installation command, optional note about package availability)
// If the package manager is unknown, returns a generic instruction.
// If the package name cannot be determined, returns an empty command with an error note.
func generateInstallCommand(tool string, distro string, pkgManager string) (string, string) {
	if tool == "" {
		return "", "Tool name is required"
	}

	pkgName := getPackageName(tool, distro)
	if pkgName == "" {
		return "", fmt.Sprintf("Package name not available for tool '%s' on distribution '%s'", tool, distro)
	}

	var cmd string
	var note string

	switch pkgManager {
	case "apt":
		cmd = fmt.Sprintf("sudo apt update && sudo apt install %s", pkgName)
		if tool == "ydotool" {
			note = "Note: ydotool may not be in default Ubuntu/Debian repos. You may need to build from source or use a PPA."
		}
	case "dnf", "yum":
		cmd = fmt.Sprintf("sudo %s install %s", pkgManager, pkgName)
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

// checkMissingDependencies checks which dependencies are missing and returns installation information.
// It detects the Linux distribution and generates distro-specific installation commands.
// Returns a list of DependencyInfo structs for missing optional dependencies.
func checkMissingDependencies(caps linuxCapabilities, displayServer string, hasUinput bool) []DependencyInfo {
	var missing []DependencyInfo

	distro, pkgManager, err := detectLinuxDistribution()
	if err != nil {
		log.Printf("linux: failed to detect distribution: %v", err)
		distro = "unknown"
		pkgManager = "unknown"
	}

	// Check ydotool (recommended for Wayland, works on X11 too)
	if !caps.ydotoolAvailable {
		installCmd, note := generateInstallCommand("ydotool", distro, pkgManager)
		whyNeeded := "Provides reliable mouse simulation on both X11 and Wayland (recommended)"
		if displayServer == "wayland" {
			whyNeeded = "Provides reliable mouse simulation on Wayland display server (highly recommended)"
		}
		alt := "Use uinput instead (requires permissions: sudo usermod -aG input $USER, then logout/login)"
		if !hasUinput {
			alt = "Setup uinput permissions: sudo usermod -aG input $USER (then logout/login)"
		}
		missing = append(missing, DependencyInfo{
			Name:        "ydotool",
			WhyNeeded:   whyNeeded,
			InstallCmd:  installCmd,
			Optional:    true,
			Available:   true,
			Alternative: alt,
		})
		if note != "" {
			missing[len(missing)-1].Alternative = note + "\n" + alt
		}
	}

	// Check xdotool (X11 only)
	if displayServer == "x11" && !caps.xdotoolAvailable {
		installCmd, _ := generateInstallCommand("xdotool", distro, pkgManager)
		whyNeeded := "Provides mouse simulation on X11 display server"
		alt := "Not needed if using Wayland or if uinput/ydotool is configured"
		if !hasUinput && !caps.ydotoolAvailable {
			alt = "Alternative: Install ydotool (works on both X11 and Wayland) or setup uinput"
		}
		missing = append(missing, DependencyInfo{
			Name:        "xdotool",
			WhyNeeded:   whyNeeded,
			InstallCmd:  installCmd,
			Optional:    true,
			Available:   true,
			Alternative: alt,
		})
	}

	// Check wtype (Wayland only, optional)
	if displayServer == "wayland" && !caps.wtypeAvailable {
		installCmd, note := generateInstallCommand("wtype", distro, pkgManager)
		whyNeeded := "Provides Wayland-native mouse/keyboard simulation (optional, ydotool is preferred)"
		alt := "ydotool is recommended instead, or use uinput"
		missing = append(missing, DependencyInfo{
			Name:        "wtype",
			WhyNeeded:   whyNeeded,
			InstallCmd:  installCmd,
			Optional:    true,
			Available:   note == "", // Available if no special note
			Alternative: alt,
		})
		if note != "" {
			missing[len(missing)-1].Alternative = note + "\n" + alt
		}
	}

	// Check xprintidle (X11 only, optional - used for idle detection)
	if displayServer == "x11" && !caps.xprintidleAvailable {
		installCmd, _ := generateInstallCommand("xprintidle", distro, pkgManager)
		whyNeeded := "Provides idle time detection on X11 (optional, activity simulation works without it)"
		alt := "Not needed on Wayland or if you don't need idle detection"
		missing = append(missing, DependencyInfo{
			Name:        "xprintidle",
			WhyNeeded:   whyNeeded,
			InstallCmd:  installCmd,
			Optional:    true,
			Available:   true,
			Alternative: alt,
		})
	}

	return missing
}

// formatDependencyMessages formats dependency information into user-friendly messages.
// Returns an empty string if no dependencies are missing, otherwise returns a formatted message
// with installation instructions and alternatives.
func formatDependencyMessages(missing []DependencyInfo, displayServer string, hasUinput bool) string {
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

// getInputGroupGID looks up the "input" group GID by parsing /etc/group
// Returns the GID if found, or -1 if not found or on error
func getInputGroupGID() int {
	file, err := os.Open("/etc/group")
	if err != nil {
		return -1
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// /etc/group format: groupname:password:GID:userlist
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[0] == "input" {
			if gid, err := strconv.Atoi(parts[2]); err == nil {
				return gid
			}
		}
	}
	return -1
}

// checkUinputPermissions checks if uinput is accessible and returns user-friendly error message if not
// Returns: (hasAccess bool, errorMessage string)
func checkUinputPermissions() (bool, string) {
	// Check if /dev/uinput exists
	if _, err := os.Stat(uinputDevicePath); os.IsNotExist(err) {
		return false, "uinput device not found: /dev/uinput does not exist. The uinput kernel module may not be loaded. Try: sudo modprobe uinput"
	}

	// Try to open the device to check permissions
	f, err := os.OpenFile(uinputDevicePath, os.O_WRONLY, 0)
	if err != nil {
		// Check if user is in input group by looking up the actual input group GID
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
					msg := "uinput permission denied. Add your user to the 'input' group:\n  sudo usermod -aG input $USER\nThen log out and log back in for changes to take effect.\n\nAlternatively, create a udev rule:\n  echo 'KERNEL==\"uinput\", MODE=\"0664\", GROUP=\"input\"' | sudo tee /etc/udev/rules.d/99-uinput.rules\n  sudo udevadm control --reload-rules\n  sudo udevadm trigger"
					return false, msg
				}
			}
		}
		return false, fmt.Sprintf("uinput permission denied: %v\n\nTo fix:\n1. Add user to input group: sudo usermod -aG input $USER (then logout/login)\n2. Or create udev rule: echo 'KERNEL==\"uinput\", MODE=\"0664\", GROUP=\"input\"' | sudo tee /etc/udev/rules.d/99-uinput.rules", err)
	}
	f.Close()
	return true, ""
}

// --- systemd-inhibit strategy ---

type systemdInhibitor struct {
	cmd *exec.Cmd
}

func (s *systemdInhibitor) Name() string { return "systemd-inhibit" }
func (s *systemdInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("systemd-inhibit") {
		return fmt.Errorf("systemd-inhibit command not found")
	}
	s.cmd = exec.CommandContext(ctx, "systemd-inhibit",
		"--what=idle:sleep:handle-lid-switch",
		"--who=keep-alive",
		"--why=User requested keep-alive",
		"--mode=block",
		"sleep", "infinity")
	return s.cmd.Start()
}
func (s *systemdInhibitor) Deactivate() error {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}

// --- DBus Base strategy ---

type dbusStrategy struct {
	dest   string
	path   string
	iface  string
	method string
	args   []string
	cookie uint32
}

func (d *dbusStrategy) call(method string, args ...string) (string, error) {
	if hasCommand("dbus-send") {
		fullArgs := append([]string{"--print-reply", "--dest=" + d.dest, d.path, d.iface + "." + method}, args...)
		return runVerbose("dbus-send", fullArgs...)
	}
	if hasCommand("gdbus") {
		fullArgs := append([]string{"call", "--session", "--dest", d.dest, "--object-path", d.path, "--method", d.iface + "." + method}, args...)
		return runVerbose("gdbus", fullArgs...)
	}
	return "", fmt.Errorf("no dbus client (dbus-send/gdbus) found")
}

func (d *dbusStrategy) parseCookie(out string) (uint32, error) {
	// Simple parsing for both dbus-send and gdbus output (returns a uint32)
	parts := strings.Fields(out)
	if len(parts) > 0 {
		lastPart := strings.TrimRight(parts[len(parts)-1], ")")
		if val, err := strconv.ParseUint(lastPart, 10, 32); err == nil {
			return uint32(val), nil
		}
	}
	return 0, fmt.Errorf("failed to parse cookie from: %q", out)
}

type dbusInhibitor struct {
	dbusStrategy
	name         string
	unInhibitArg string
}

func (d *dbusInhibitor) Name() string { return d.name }
func (d *dbusInhibitor) Activate(ctx context.Context) error {
	out, err := d.call(d.method, d.args...)
	if err != nil {
		return err
	}
	cookie, err := d.parseCookie(out)
	if err != nil {
		return err
	}
	d.cookie = cookie
	return nil
}

func (d *dbusInhibitor) Deactivate() error {
	if d.cookie == 0 {
		return nil
	}
	_, err := d.call(d.unInhibitArg, "uint32:"+strconv.FormatUint(uint64(d.cookie), 10))
	return err
}

// --- GNOME specific fallback logic ---

type gsettingsInhibitor struct {
	prevSettings map[string]string
}

func (g *gsettingsInhibitor) Name() string { return "gsettings" }
func (g *gsettingsInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("gsettings") {
		return fmt.Errorf("gsettings command not found")
	}
	g.prevSettings = make(map[string]string)
	settings := []struct{ schema, key, value string }{
		{"org.gnome.desktop.session", "idle-delay", "0"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-ac-type", "'nothing'"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-battery-type", "'nothing'"},
	}
	for _, s := range settings {
		if out, err := runVerbose("gsettings", "get", s.schema, s.key); err == nil {
			g.prevSettings[s.schema+" "+s.key] = out
		}
		if out, err := runVerbose("gsettings", "set", s.schema, s.key, s.value); err != nil {
			return fmt.Errorf("gsettings set failed: %v (out: %q)", err, out)
		}
	}
	return nil
}
func (g *gsettingsInhibitor) Deactivate() error {
	for k, v := range g.prevSettings {
		parts := strings.SplitN(k, " ", 2)
		runBestEffort("gsettings", "set", parts[0], parts[1], v)
	}
	return nil
}

// --- X11 strategy ---

type xsetInhibitor struct{}

func (x *xsetInhibitor) Name() string { return "xset" }
func (x *xsetInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("xset") || os.Getenv("DISPLAY") == "" {
		return fmt.Errorf("xset not available or DISPLAY not set")
	}
	runBestEffort("xset", "s", "off")
	runBestEffort("xset", "-dpms")
	return nil
}
func (x *xsetInhibitor) Deactivate() error {
	runBestEffort("xset", "s", "on")
	runBestEffort("xset", "+dpms")
	return nil
}

// getLinuxIdleTime returns the system idle time on Linux using xprintidle (best-effort)
// Note: xprintidle only works on X11, not Wayland
func getLinuxIdleTime() (time.Duration, error) {
	displayServer := detectDisplayServer()
	if displayServer == "wayland" {
		return 0, fmt.Errorf("xprintidle does not work on Wayland (only X11)")
	}
	if !hasCommand("xprintidle") {
		return 0, fmt.Errorf("xprintidle not found")
	}
	out, err := runVerbose("xprintidle")
	if err != nil {
		return 0, err
	}
	millis, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse xprintidle output %q: %v", out, err)
	}
	return time.Duration(millis) * time.Millisecond, nil
}

// --- Native uinput Simulator ---

const (
	uinputDevicePath = "/dev/uinput"
	evSyn            = 0x00
	evRel            = 0x02
	relX             = 0x00
	relY             = 0x01
	uiSetEvbit       = 0x40045564 // _IOW('U', 100, int)
	uiSetRelbit      = 0x40045565 // _IOW('U', 101, int)
	uiDevCreate      = 0x5501     // _IO('U', 1)
	uiDevDestroy     = 0x5502     // _IO('U', 2)
)

type uinputUserDev struct {
	name [80]byte
	id   struct {
		bustype uint16
		vendor  uint16
		product uint16
		version uint16
	}
	ffEffectsMax uint32
	absmax       [64]int32
	absmin       [64]int32
	absfuzz      [64]int32
	absflat      [64]int32
}

type inputEvent struct {
	time  syscall.Timeval
	etype uint16
	code  uint16
	value int32
}

type uinputSimulator struct {
	fd   uintptr
	file *os.File
}

func (u *uinputSimulator) setup() error {
	f, err := os.OpenFile(uinputDevicePath, os.O_WRONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		return err
	}
	u.file = f
	u.fd = f.Fd()

	// Enable relative axes
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetEvbit), uintptr(evRel)); errno != 0 {
		f.Close()
		u.file = nil
		u.fd = 0
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetRelbit), uintptr(relX)); errno != 0 {
		f.Close()
		u.file = nil
		u.fd = 0
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetRelbit), uintptr(relY)); errno != 0 {
		f.Close()
		u.file = nil
		u.fd = 0
		return errno
	}

	// Create device
	var dev uinputUserDev
	copy(dev.name[:], "keep-alive-mouse")
	dev.id.bustype = 0x03 // BUS_USB
	dev.id.vendor = 0x1234
	dev.id.product = 0x5678

	if _, _, errno := syscall.Syscall(syscall.SYS_WRITE, u.fd, uintptr(unsafe.Pointer(&dev)), unsafe.Sizeof(dev)); errno != 0 {
		f.Close()
		u.file = nil
		u.fd = 0
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiDevCreate), 0); errno != 0 {
		f.Close()
		u.file = nil
		u.fd = 0
		return errno
	}

	return nil
}

func (u *uinputSimulator) move(dx, dy int32) error {
	events := []inputEvent{
		{etype: evRel, code: relX, value: dx},
		{etype: evRel, code: relY, value: dy},
		{etype: evSyn, code: 0, value: 0},
	}
	for _, ev := range events {
		_, err := syscall.Write(int(u.fd), (*[unsafe.Sizeof(ev)]byte)(unsafe.Pointer(&ev))[:])
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *uinputSimulator) close() {
	if u.fd != 0 {
		syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiDevDestroy), 0)
	}
	if u.file != nil {
		u.file.Close()
		u.file = nil
	}
	u.fd = 0
}

// --- Platform Implementation ---

// DependencyInfo contains information about a missing dependency and how to install it.
// This struct is used to provide user-friendly installation guidance.
type DependencyInfo struct {
	Name        string // Name of the dependency (e.g., "ydotool", "xdotool")
	WhyNeeded   string // Explanation of why this dependency is needed
	InstallCmd  string // Distro-specific installation command
	Optional    bool   // Whether the dependency is optional (all dependencies are currently optional)
	Available   bool   // Whether the package exists in default repositories
	Alternative string // Alternative installation methods or workarounds
}

type linuxCapabilities struct {
	xdotoolAvailable    bool
	xprintidleAvailable bool
	uinputAvailable     bool
	ydotoolAvailable    bool
	wtypeAvailable      bool
	displayServer       string
	desktopEnvironment  string
}

type linuxKeepAlive struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	isRunning   bool
	chatAppTick *time.Ticker
	inhibitors  []inhibitor
	uinput      *uinputSimulator

	simulateActivity bool

	// last time we logged that user is active (to avoid spam)
	lastActiveLogNS int64

	// random source and pattern generator for natural mouse movements
	rnd        *rand.Rand
	patternGen *MousePatternGenerator
}

func detectLinuxCapabilities() linuxCapabilities {
	displayServer := detectDisplayServer()
	// xprintidle only works on X11, not Wayland
	xprintidleAvailable := hasCommand("xprintidle") && displayServer == "x11"
	return linuxCapabilities{
		xdotoolAvailable:    hasCommand("xdotool"),
		xprintidleAvailable: xprintidleAvailable,
		uinputAvailable:     true, // Will be tested during setup
		ydotoolAvailable:    hasCommand("ydotool"),
		wtypeAvailable:      hasCommand("wtype"),
		displayServer:       displayServer,
		desktopEnvironment:  detectDesktopEnvironment(),
	}
}

// buildLinuxInhibitors builds a prioritized list of inhibitors based on detected desktop environment
// Priority: systemd-inhibit (always first) → DE-specific DBus → gsettings (GNOME-based) → xset (X11 only)
func buildLinuxInhibitors() []inhibitor {
	de := detectDesktopEnvironment()
	displayServer := detectDisplayServer()
	inhibitors := []inhibitor{}

	// Always try systemd-inhibit first (works on all systems)
	inhibitors = append(inhibitors, &systemdInhibitor{})

	// Add DE-specific inhibitors based on detected desktop
	switch de {
	case "cosmic":
		// Cosmic uses GNOME session manager
		inhibitors = append(inhibitors, &dbusInhibitor{
			name: "dbus-cosmic",
			dbusStrategy: dbusStrategy{
				dest:   "org.gnome.SessionManager",
				path:   "/org/gnome/SessionManager",
				iface:  "org.gnome.SessionManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "uint32:0", "string:User requested keep-alive", "uint32:12"},
			},
			unInhibitArg: "Uninhibit",
		})
		// Cosmic is GNOME-based, so gsettings should work
		inhibitors = append(inhibitors, &gsettingsInhibitor{})
	case "gnome":
		inhibitors = append(inhibitors, &dbusInhibitor{
			name: "dbus-gnome",
			dbusStrategy: dbusStrategy{
				dest:   "org.gnome.SessionManager",
				path:   "/org/gnome/SessionManager",
				iface:  "org.gnome.SessionManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "uint32:0", "string:User requested keep-alive", "uint32:12"},
			},
			unInhibitArg: "Uninhibit",
		})
		inhibitors = append(inhibitors, &gsettingsInhibitor{})
	case "kde":
		inhibitors = append(inhibitors, &dbusInhibitor{
			name: "dbus-kde",
			dbusStrategy: dbusStrategy{
				dest:   "org.freedesktop.PowerManagement.Inhibit",
				path:   "/org/freedesktop/PowerManagement/Inhibit",
				iface:  "org.freedesktop.PowerManagement.Inhibit",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "string:Keep system awake"},
			},
			unInhibitArg: "UnInhibit",
		})
	case "xfce":
		inhibitors = append(inhibitors, &dbusInhibitor{
			name: "dbus-xfce",
			dbusStrategy: dbusStrategy{
				dest:   "org.xfce.PowerManager",
				path:   "/org/xfce/PowerManager",
				iface:  "org.xfce.PowerManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "string:Keep system awake"},
			},
			unInhibitArg: "UnInhibit",
		})
	case "mate":
		inhibitors = append(inhibitors, &dbusInhibitor{
			name: "dbus-mate",
			dbusStrategy: dbusStrategy{
				dest:   "org.mate.SessionManager",
				path:   "/org/mate/SessionManager",
				iface:  "org.mate.SessionManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "uint32:0", "string:Keep system awake", "uint32:12"},
			},
			unInhibitArg: "Uninhibit",
		})
	}

	// Add freedesktop fallback (works on many systems)
	inhibitors = append(inhibitors, &dbusInhibitor{
		name: "dbus-freedesktop",
		dbusStrategy: dbusStrategy{
			dest:   "org.freedesktop.ScreenSaver",
			path:   "/org/freedesktop/ScreenSaver",
			iface:  "org.freedesktop.ScreenSaver",
			method: "Inhibit",
			args:   []string{"string:keep-alive", "string:Keep system awake"},
		},
		unInhibitArg: "UnInhibit",
	})

	// xset only works on X11
	if displayServer == "x11" {
		inhibitors = append(inhibitors, &xsetInhibitor{})
	}

	return inhibitors
}

func (k *linuxKeepAlive) activateInhibitors(ctx context.Context) (int, error) {
	allInhibitors := buildLinuxInhibitors()
	activeCount := 0
	var activationErrors []string

	for _, inh := range allInhibitors {
		err := inh.Activate(ctx)
		if err != nil {
			log.Printf("linux: inhibitor %s failed: %v", inh.Name(), err)
			activationErrors = append(activationErrors, fmt.Sprintf("%s: %v", inh.Name(), err))
			continue
		}

		// Verify activation based on inhibitor type
		verified := false
		switch v := inh.(type) {
		case *systemdInhibitor:
			// Verify systemd-inhibit process is running
			if v.cmd != nil && v.cmd.Process != nil {
				// Check if process is still alive
				if err := v.cmd.Process.Signal(syscall.Signal(0)); err == nil {
					verified = true
					log.Printf("linux: verified systemd-inhibit process (pid %d) is running", v.cmd.Process.Pid)
				} else {
					log.Printf("linux: warning: systemd-inhibit process verification failed: %v", err)
				}
			}
		case *dbusInhibitor:
			// Verify DBus cookie was received
			if v.cookie != 0 {
				verified = true
				log.Printf("linux: verified DBus inhibitor %s with cookie %d", v.name, v.cookie)
			} else {
				log.Printf("linux: warning: DBus inhibitor %s activated but no cookie received", v.name)
			}
		case *gsettingsInhibitor:
			// gsettings doesn't return a cookie, but if Activate succeeded, it worked
			verified = true
		case *xsetInhibitor:
			// xset doesn't have verification, but if Activate succeeded, it worked
			verified = true
		}

		if verified {
			k.inhibitors = append(k.inhibitors, inh)
			log.Printf("linux: activated and verified inhibitor: %s", inh.Name())
			activeCount++
		} else {
			log.Printf("linux: warning: inhibitor %s activated but verification failed", inh.Name())
			// Still count it as active if activation succeeded, but log the warning
			k.inhibitors = append(k.inhibitors, inh)
			activeCount++
		}
	}

	if activeCount == 0 {
		errorMsg := "linux: no keep-alive method successfully activated"
		if len(activationErrors) > 0 {
			errorMsg += "\nFailed inhibitors:\n" + strings.Join(activationErrors, "\n")
		}
		return 0, fmt.Errorf("%s", errorMsg)
	}

	log.Printf("linux: successfully activated %d inhibitor(s) out of %d attempted", activeCount, len(allInhibitors))
	return activeCount, nil
}

func (k *linuxKeepAlive) setupUinput() {
	hasAccess, errMsg := checkUinputPermissions()
	if !hasAccess {
		log.Printf("linux: uinput not available: %s", errMsg)
		k.uinput = nil
		return
	}

	k.uinput = &uinputSimulator{}
	if err := k.uinput.setup(); err != nil {
		log.Printf("linux: uinput setup failed: %v", err)
		if errMsg != "" {
			log.Printf("linux: permission hint: %s", errMsg)
		}
		k.uinput = nil
	} else {
		log.Printf("linux: native uinput mouse simulation activated")
	}
}

func (k *linuxKeepAlive) startInhibitorHealthCheck(ctx context.Context) {
	healthCheckTicker := time.NewTicker(30 * time.Second)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer healthCheckTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-healthCheckTicker.C:
				k.verifyInhibitors()
			}
		}
	}()
}

func (k *linuxKeepAlive) verifyInhibitors() {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isRunning {
		return
	}

	for _, inh := range k.inhibitors {
		switch v := inh.(type) {
		case *systemdInhibitor:
			// Verify systemd-inhibit process is still running
			if v.cmd != nil && v.cmd.Process != nil {
				if err := v.cmd.Process.Signal(syscall.Signal(0)); err != nil {
					log.Printf("linux: warning: systemd-inhibit process (pid %d) is not running: %v", v.cmd.Process.Pid, err)
					// Attempt to reactivate
					if k.ctx != nil {
						if reactivateErr := v.Activate(k.ctx); reactivateErr != nil {
							log.Printf("linux: error: failed to reactivate systemd-inhibit: %v", reactivateErr)
						} else {
							log.Printf("linux: successfully reactivated systemd-inhibit")
						}
					}
				}
			}
		case *dbusInhibitor:
			// DBus inhibitors don't need periodic checks as they're session-based
			// The cookie remains valid until explicitly uninhibited
		case *gsettingsInhibitor:
			// gsettings inhibitors are persistent until deactivated
		case *xsetInhibitor:
			// xset inhibitors are persistent until deactivated
		}
	}
}

func (k *linuxKeepAlive) startChatAppTickerLocked(ctx context.Context, caps linuxCapabilities) {
	if !k.simulateActivity {
		return
	}

	k.chatAppTick = time.NewTicker(ChatAppActivityInterval)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.chatAppTick.Stop()

		if !caps.xprintidleAvailable {
			log.Printf("linux: xprintidle not found; will simulate activity without idle check")
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-k.chatAppTick.C:
				k.simulateChatAppActivity(ctx, caps)
			}
		}
	}()
}

func (k *linuxKeepAlive) simulateChatAppActivity(ctx context.Context, caps linuxCapabilities) {
	shouldSimulate := true
	var idle time.Duration
	var idleErr error

	if caps.xprintidleAvailable {
		idle, idleErr = getLinuxIdleTime()
		if idleErr == nil && idle <= IdleThreshold {
			shouldSimulate = false
		} else if idleErr != nil {
			log.Printf("linux: idle time check failed: %v (will simulate anyway)", idleErr)
		}
	} else if caps.displayServer == "wayland" {
		// xprintidle doesn't work on Wayland, so we'll simulate anyway
		log.Printf("linux: xprintidle not available on Wayland; simulating activity")
	}

	nowNS := time.Now().UnixNano()
	lastActiveLog := atomic.LoadInt64(&k.lastActiveLogNS)

	if !shouldSimulate {
		// Log occasionally (every 2 minutes) that we're skipping due to active use
		if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > 2*time.Minute {
			atomic.StoreInt64(&k.lastActiveLogNS, nowNS)
			if caps.xprintidleAvailable && idleErr == nil {
				log.Printf("linux: user is active (idle: %v); skipping simulation to avoid interference", idle)
			} else {
				log.Printf("linux: user is active; skipping simulation to avoid interference")
			}
		}
		return
	}

	// User became idle or idle check unavailable - log if we were previously active
	if lastActiveLog != 0 {
		atomic.StoreInt64(&k.lastActiveLogNS, 0)
		if caps.xprintidleAvailable && idleErr == nil {
			log.Printf("linux: user became idle (%v); resuming activity simulation", idle)
		} else if idleErr != nil {
			log.Printf("linux: idle check failed; resuming activity simulation (unable to determine user state)")
		} else {
			log.Printf("linux: resuming activity simulation")
		}
	}

	points := k.patternGen.GenerateShapePoints()
	k.executeMousePattern(points, caps)
}

func (k *linuxKeepAlive) executeMousePattern(points []MousePoint, caps linuxCapabilities) {
	// Execute pattern using available methods based on display server
	// Priority: uinput → ydotool → xdotool (X11 only) → wtype (Wayland only) → DBus fallback
	// Stop after first successful method to avoid redundant execution

	// Try uinput first (works on both X11 and Wayland if permissions allow)
	if k.uinput != nil {
		if k.executePatternUinput(points) {
			return
		}
	}

	// Try ydotool (works on both X11 and Wayland)
	if caps.ydotoolAvailable {
		if k.executePatternYdotool(points) {
			return
		}
	}

	// Try xdotool (X11 only)
	if caps.displayServer == "x11" && caps.xdotoolAvailable {
		if k.executePatternXdotool(points) {
			return
		}
	}

	// Try wtype (Wayland-native, but limited mouse support)
	if caps.displayServer == "wayland" && caps.wtypeAvailable {
		if k.executePatternWtype(points) {
			return
		}
	}

	// Soft simulation via DBus (works on both, but less effective) - only if no other method worked
	runBestEffort("dbus-send", "--dest=org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.SimulateUserActivity")
	runBestEffort("dbus-send", "--dest=org.gnome.ScreenSaver", "/org/gnome/ScreenSaver", "org.gnome.ScreenSaver.SimulateUserActivity")

	if caps.displayServer == "wayland" {
		log.Printf("linux: warning: no Wayland-compatible mouse simulation method available. Install ydotool: sudo apt install ydotool (or equivalent for your distribution)")
	}
}

func (k *linuxKeepAlive) executePatternUinput(points []MousePoint) bool {
	if k.uinput == nil {
		return false
	}

	// Execute pattern with natural timing
	for i, pt := range points {
		dx := int32(pt.X)
		dy := int32(pt.Y)
		if err := k.uinput.move(dx, dy); err != nil {
			log.Printf("linux: uinput move failed: %v", err)
			return false
		}

		distance := SegmentDistance(points, i)
		delay := k.patternGen.MovementDelay(distance)
		time.Sleep(delay)

		if k.patternGen.ShouldPause() {
			time.Sleep(k.patternGen.PauseDelay())
		}

		if k.patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := k.patternGen.IntermediatePoint(points, i, delay)
			if err := k.uinput.move(int32(midPt.X), int32(midPt.Y)); err != nil {
				log.Printf("linux: uinput move failed: %v", err)
				return false
			}
			time.Sleep(midDelay)
		}
	}

	// Return to origin
	lastPt := points[len(points)-1]
	returnDelay := k.patternGen.ReturnDelay()
	if err := k.uinput.move(-int32(lastPt.X), -int32(lastPt.Y)); err != nil {
		log.Printf("linux: uinput move failed: %v", err)
		return false
	}
	time.Sleep(returnDelay)
	return true
}

func (k *linuxKeepAlive) executePatternXdotool(points []MousePoint) bool {
	for i, pt := range points {
		dx := int(pt.X)
		dy := int(pt.Y)
		_, err := runVerbose("xdotool", "mousemove_relative", "--", fmt.Sprintf("%d", dx), fmt.Sprintf("%d", dy))
		if err != nil {
			log.Printf("linux: xdotool move failed: %v", err)
			return false
		}

		distance := SegmentDistance(points, i)
		delay := k.patternGen.MovementDelay(distance)
		time.Sleep(delay)

		if k.patternGen.ShouldPause() {
			time.Sleep(k.patternGen.PauseDelay())
		}

		if k.patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := k.patternGen.IntermediatePoint(points, i, delay)
			_, err := runVerbose("xdotool", "mousemove_relative", "--", fmt.Sprintf("%d", int(midPt.X)), fmt.Sprintf("%d", int(midPt.Y)))
			if err != nil {
				log.Printf("linux: xdotool move failed: %v", err)
				return false
			}
			time.Sleep(midDelay)
		}
	}

	// Return to origin
	lastPt := points[len(points)-1]
	returnDelay := k.patternGen.ReturnDelay()
	_, err := runVerbose("xdotool", "mousemove_relative", "--", fmt.Sprintf("%d", -int(lastPt.X)), fmt.Sprintf("%d", -int(lastPt.Y)))
	if err != nil {
		log.Printf("linux: xdotool move failed: %v", err)
		return false
	}
	time.Sleep(returnDelay)
	return true
}

// executePatternYdotool executes mouse pattern using ydotool (works on both X11 and Wayland)
func (k *linuxKeepAlive) executePatternYdotool(points []MousePoint) bool {
	for i, pt := range points {
		dx := int(pt.X)
		dy := int(pt.Y)
		// ydotool mousemove with -- separator for relative movement
		// Format: ydotool mousemove -- <dx> <dy>
		_, err := runVerbose("ydotool", "mousemove", "--", fmt.Sprintf("%d", dx), fmt.Sprintf("%d", dy))
		if err != nil {
			log.Printf("linux: ydotool move failed: %v", err)
			return false
		}

		distance := SegmentDistance(points, i)
		delay := k.patternGen.MovementDelay(distance)
		time.Sleep(delay)

		if k.patternGen.ShouldPause() {
			time.Sleep(k.patternGen.PauseDelay())
		}

		if k.patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := k.patternGen.IntermediatePoint(points, i, delay)
			_, err := runVerbose("ydotool", "mousemove", "--", fmt.Sprintf("%d", int(midPt.X)), fmt.Sprintf("%d", int(midPt.Y)))
			if err != nil {
				log.Printf("linux: ydotool move failed: %v", err)
				return false
			}
			time.Sleep(midDelay)
		}
	}

	// Return to origin
	lastPt := points[len(points)-1]
	returnDelay := k.patternGen.ReturnDelay()
	_, err := runVerbose("ydotool", "mousemove", "--", fmt.Sprintf("%d", -int(lastPt.X)), fmt.Sprintf("%d", -int(lastPt.Y)))
	if err != nil {
		log.Printf("linux: ydotool move failed: %v", err)
		return false
	}
	time.Sleep(returnDelay)
	return true
}

// executePatternWtype executes mouse pattern using wtype (Wayland-native)
// Note: wtype doesn't support relative mouse movement directly, so we use absolute coordinates
// This is a simplified implementation - wtype may need different approach
func (k *linuxKeepAlive) executePatternWtype(points []MousePoint) bool {
	// wtype doesn't have direct mouse movement commands in the same way
	// We'll use a workaround: simulate small keyboard events or use wlrctl if available
	// For now, log that wtype is not fully supported for mouse movement
	log.Printf("linux: wtype mouse movement not fully implemented (wtype focuses on keyboard simulation)")
	// Fall back to DBus simulation which works on Wayland
	_, err := runVerbose("dbus-send", "--dest=org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.SimulateUserActivity")
	if err != nil {
		log.Printf("linux: wtype/DBus simulation failed: %v", err)
		return false
	}
	return true
}

func (k *linuxKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	k.ctx, k.cancel = context.WithCancel(ctx)

	// Initialize random source and pattern generator
	k.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	k.patternGen = NewMousePatternGenerator(k.rnd)

	// Detect capabilities and log diagnostics
	caps := detectLinuxCapabilities()
	log.Printf("linux: === Startup Diagnostics ===")
	log.Printf("linux: Desktop Environment: %s", caps.desktopEnvironment)
	log.Printf("linux: Display Server: %s", caps.displayServer)
	log.Printf("linux: Available tools: xdotool=%v, ydotool=%v, wtype=%v, xprintidle=%v",
		caps.xdotoolAvailable, caps.ydotoolAvailable, caps.wtypeAvailable, caps.xprintidleAvailable)

	// Check uinput permissions and log status
	hasUinputAccess, uinputErrMsg := checkUinputPermissions()
	log.Printf("linux: uinput access: %v", hasUinputAccess)
	if !hasUinputAccess && uinputErrMsg != "" {
		log.Printf("linux: uinput permission issue: %s", uinputErrMsg)
	}

	// Activate inhibitors
	activeCount, err := k.activateInhibitors(k.ctx)
	if err != nil {
		k.cancel()
		// Enhance error message with suggestions
		enhancedErr := fmt.Errorf("%v\n\nTroubleshooting:\n- Ensure systemd-inhibit is available: which systemd-inhibit\n- Check DBus services: dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames\n- For Cosmic/GNOME: ensure org.gnome.SessionManager is available", err)
		return enhancedErr
	}

	// Setup uinput if available
	k.setupUinput()

	hasUinput := k.uinput != nil
	if k.uinput != nil {
		caps.uinputAvailable = true
		log.Printf("linux: uinput mouse simulation: enabled")
	} else {
		log.Printf("linux: uinput mouse simulation: disabled (permissions or unavailable)")
	}

	// Check for missing dependencies and log messages
	missingDeps := checkMissingDependencies(caps, caps.displayServer, hasUinput)
	if len(missingDeps) > 0 {
		depMessage := formatDependencyMessages(missingDeps, caps.displayServer, hasUinput)
		log.Printf("linux: missing dependencies detected:\n%s", depMessage)
	}

	// Log mouse simulation capabilities
	mouseMethods := []string{}
	if k.uinput != nil {
		mouseMethods = append(mouseMethods, "uinput")
	}
	if caps.ydotoolAvailable {
		mouseMethods = append(mouseMethods, "ydotool")
	}
	if caps.wtypeAvailable && caps.displayServer == "wayland" {
		mouseMethods = append(mouseMethods, "wtype")
	}
	if caps.xdotoolAvailable && caps.displayServer == "x11" {
		mouseMethods = append(mouseMethods, "xdotool")
	}
	if len(mouseMethods) == 0 {
		log.Printf("linux: warning: no mouse simulation methods available")
	} else {
		log.Printf("linux: mouse simulation methods: %s", strings.Join(mouseMethods, ", "))
	}

	log.Printf("linux: === End Diagnostics ===")
	log.Printf("linux: started successfully; active inhibitors: %d", activeCount)

	// Start periodic inhibitor health checks
	k.startInhibitorHealthCheck(k.ctx)

	// Activity ticker removed - inhibitors maintain state without periodic refresh
	k.startChatAppTickerLocked(k.ctx, caps)

	k.isRunning = true
	return nil
}

func (k *linuxKeepAlive) Stop() error {
	k.mu.Lock()
	if !k.isRunning {
		k.mu.Unlock()
		return nil
	}

	if k.cancel != nil {
		k.cancel()
	}

	// Stop tickers first to prevent new operations
	if k.chatAppTick != nil {
		k.chatAppTick.Stop()
		k.chatAppTick = nil
	}

	// Deactivate all inhibitors in reverse order, tracking failures
	var deactivateErrors []error
	inhibitors := make([]inhibitor, len(k.inhibitors))
	copy(inhibitors, k.inhibitors)

	k.mu.Unlock()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		k.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("linux: all goroutines completed")
	case <-time.After(2 * time.Second):
		log.Printf("linux: warning: some goroutines did not complete within timeout")
	}

	// Deactivate inhibitors (best effort - continue even if some fail)
	for i := len(inhibitors) - 1; i >= 0; i-- {
		inh := inhibitors[i]
		if err := inh.Deactivate(); err != nil {
			log.Printf("linux: error deactivating inhibitor %s: %v", inh.Name(), err)
			deactivateErrors = append(deactivateErrors, err)
		} else {
			log.Printf("linux: deactivated inhibitor %s", inh.Name())
		}
	}

	k.mu.Lock()

	// Cleanup uinput device
	if k.uinput != nil {
		fdBeforeClose := k.uinput.fd
		k.uinput.close()
		// Verify uinput is closed by checking if fd was non-zero before close
		if fdBeforeClose != 0 {
			if k.uinput.fd == 0 {
				log.Printf("linux: uinput device closed successfully")
			} else {
				log.Printf("linux: warning: uinput device may not have closed properly (fd=%d)", k.uinput.fd)
			}
		}
		k.uinput = nil
	}

	k.inhibitors = nil
	k.isRunning = false
	k.ctx = nil
	k.cancel = nil
	k.mu.Unlock()

	if len(deactivateErrors) > 0 {
		log.Printf("linux: stopped with %d inhibitor deactivation errors", len(deactivateErrors))
		return fmt.Errorf("linux: %d inhibitors failed to deactivate", len(deactivateErrors))
	}

	log.Printf("linux: stopped; cleanup complete")
	return nil
}

func (k *linuxKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.simulateActivity = simulate

	if !k.isRunning {
		return
	}

	if simulate {
		// Start chat app ticker if not already running
		if k.chatAppTick == nil {
			caps := detectLinuxCapabilities()
			if k.uinput != nil {
				caps.uinputAvailable = true
			}
			k.startChatAppTickerLocked(k.ctx, caps)
		}
	} else {
		// Stop chat app ticker
		if k.chatAppTick != nil {
			k.chatAppTick.Stop()
			k.chatAppTick = nil
		}
	}
}

// GetDependencyMessage returns the formatted dependency message if dependencies are missing.
// This function is called before Start() to display dependency information to the user.
// It performs a fresh detection to ensure accuracy at startup time.
func GetDependencyMessage() string {
	caps := detectLinuxCapabilities()
	hasUinput, _ := checkUinputPermissions()
	missingDeps := checkMissingDependencies(caps, caps.displayServer, hasUinput)
	if len(missingDeps) > 0 {
		return formatDependencyMessages(missingDeps, caps.displayServer, hasUinput)
	}
	return ""
}

func NewKeepAlive() (KeepAlive, error) {
	return &linuxKeepAlive{}, nil
}
