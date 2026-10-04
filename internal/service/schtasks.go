package service

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode/utf16"
)

// schtasks manages a Task Scheduler task that starts at the user's logon.
type schtasks struct {
	run     runner
	user    string // DOMAIN\user, from os/user
	tempDir string
}

const schtasksHint = `inspect the task with "schtasks /Query /TN keepalive /V /FO LIST"`

func (m *schtasks) Name() string { return "Task Scheduler" }

func (m *schtasks) Install(spec Spec) error {
	if err := spec.validate(windowsAbs); err != nil {
		return err
	}
	if st, _ := m.Status(); st.Running {
		m.run.Run("schtasks", "/End", "/TN", TaskName) // IgnoreNew would keep the old instance
	}
	f, err := os.CreateTemp(m.tempDir, "keepalive-task-*.xml")
	if err != nil {
		return fmt.Errorf("service: %w", err)
	}
	defer os.Remove(f.Name())
	_, err = f.Write(encodeUTF16LE(taskXML(spec, m.user)))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("service: write task XML: %w", err)
	}
	if _, err := m.run.Run("schtasks", "/Create", "/TN", TaskName, "/XML", f.Name(), "/F"); err != nil {
		return hinted(err, schtasksHint)
	}
	if _, err := m.run.Run("schtasks", "/Run", "/TN", TaskName); err != nil {
		return hinted(err, schtasksHint)
	}
	return nil
}

func (m *schtasks) Uninstall() error {
	st, err := m.Status()
	if err != nil {
		return err
	}
	if !st.Installed {
		return ErrNotInstalled
	}
	m.run.Run("schtasks", "/End", "/TN", TaskName) // fails when not running; Delete is what counts
	if _, err := m.run.Run("schtasks", "/Delete", "/TN", TaskName, "/F"); err != nil {
		return hinted(err, schtasksHint)
	}
	return nil
}

// Status parses "Status:" and "Last Result:" from the verbose list output.
// A failed query counts as not installed. The labels are English; on a
// localized Windows only Installed is reliable.
func (m *schtasks) Status() (State, error) {
	out, err := m.run.Run("schtasks", "/Query", "/TN", TaskName, "/FO", "LIST", "/V")
	if err != nil {
		return State{Path: TaskName, Detail: "not installed"}, nil
	}
	fields := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if k = strings.TrimSpace(k); ok && fields[k] == "" {
			fields[k] = strings.TrimSpace(v)
		}
	}
	st := State{Installed: true, Path: TaskName, Running: fields["Status"] == "Running"}
	st.Detail = "status " + fields["Status"]
	if fields["Status"] == "" {
		st.Detail = "installed (status unknown)"
	}
	if r := fields["Last Result"]; r != "" {
		st.Detail += ", last result " + r
	}
	return st, nil
}

// taskXML renders the task definition. The declaration says UTF-16 because
// schtasks reads the file as UTF-16; encodeUTF16LE produces those bytes.
func taskXML(spec Spec, user string) string {
	var args []string
	for _, a := range spec.programArgs() {
		args = append(args, escapeArg(a))
	}
	u := xmlText(user)
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Keeps the computer awake. Installed by "keepalive service install".</Description>
    <URI>\` + TaskName + `</URI>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + u + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + u + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + xmlText(spec.Executable) + `</Command>
      <Arguments>` + xmlText(strings.Join(args, " ")) + `</Arguments>
    </Exec>
  </Actions>
</Task>
`
}

// encodeUTF16LE returns s as UTF-16 little endian with a byte order mark.
func encodeUTF16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

// escapeArg quotes one argument for a Windows command line exactly like
// syscall.EscapeArg (which only exists on Windows; a test there compares
// them).
func escapeArg(s string) string {
	if s == "" {
		return `""`
	}
	needsBackslash, hasSpace := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			needsBackslash = true
		case ' ', '\t':
			hasSpace = true
		}
	}
	if !needsBackslash && !hasSpace {
		return s
	}
	if !needsBackslash {
		return `"` + s + `"`
	}
	var b []byte
	if hasSpace {
		b = append(b, '"')
	}
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		default:
			slashes = 0
		case '\\':
			slashes++
		case '"':
			for ; slashes > 0; slashes-- {
				b = append(b, '\\')
			}
			b = append(b, '\\')
		}
		b = append(b, c)
	}
	if hasSpace {
		for ; slashes > 0; slashes-- {
			b = append(b, '\\')
		}
		b = append(b, '"')
	}
	return string(b)
}
