package notify

import (
	"context"
	"encoding/base64"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestAppleScriptString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", `"plain"`},
		{`say "hi"`, `"say \"hi\""`},
		{`C:\path`, `"C:\\path"`},
		{`\"`, `"\\\""`},
		{"two\nlines", "\"two\nlines\""},
		{"", `""`},
		{"æøå ✓", `"æøå ✓"`},
	}
	for _, tt := range tests {
		if got := appleScriptString(tt.in); got != tt.want {
			t.Errorf("appleScriptString(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestOsascriptArgs(t *testing.T) {
	got := osascriptArgs(`keep "alive"`, `back\slash`)
	want := []string{"-e", `display notification "back\\slash" with title "keep \"alive\""`}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("osascriptArgs = %q, want %q", got, want)
	}
}

func TestXMLEscape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{`<b>&"'`, "&lt;b&gt;&amp;&#34;&#39;"},
		{"a\nb", "a&#xA;b"},
		{"bell\x07", "bell\uFFFD"}, // not allowed in XML
		{"æøå", "æøå"},
	}
	for _, tt := range tests {
		if got := xmlEscape(tt.in); got != tt.want {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToastScript(t *testing.T) {
	script := toastScript(`Tom & "Jerry"`, "it's <done>\n'@\nWrite-Host pwned")
	for _, want := range []string{
		"<text>Tom &amp; &#34;Jerry&#34;</text>",
		"<text>it&#39;s &lt;done&gt;&#xA;&#39;@&#xA;Write-Host pwned</text>",
		`CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe')`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("toast script lacks %q:\n%s", want, script)
		}
	}
	// The XML sits in a single-quoted here-string, which only a line
	// starting with '@ can end; escaping leaves no quote in the text.
	if strings.Count(script, "\n'@") != 1 {
		t.Errorf("the here-string terminator appears more than once:\n%s", script)
	}
}

func TestPowerShellArgs(t *testing.T) {
	args := powershellArgs("Write-Host 'æ ✓'")
	if len(args) < 2 || args[len(args)-2] != "-EncodedCommand" {
		t.Fatalf("powershellArgs = %q, want -EncodedCommand last", args)
	}
	raw, err := base64.StdEncoding.DecodeString(args[len(args)-1])
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("encoded command is not base64 UTF-16: %v", err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8 // little-endian
	}
	if got := string(utf16.Decode(units)); got != "Write-Host 'æ ✓'" {
		t.Fatalf("decoded command = %q", got)
	}
}

func TestNotifySendArgs(t *testing.T) {
	got := notifySendArgs("-title", "body")
	want := []string{"--app-name=keepalive", "--", "-title", "body"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("notifySendArgs = %q, want %q", got, want)
	}
}

func TestRunReportsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	err := run(context.Background(), "sh", []string{"-c", "echo boom >&2; exit 3"}, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "notify: sh") {
		t.Fatalf("run error = %v, want one naming sh and its output", err)
	}
	if err := run(context.Background(), "sh", []string{"-c", "exit 0"}, nil); err != nil {
		t.Fatalf("run(exit 0) = %v", err)
	}
}

func TestRunHonoursDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := run(ctx, "sleep", []string{"10"}, nil)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("run blocked for %v past its deadline", took)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run error = %v, want a deadline error", err)
	}
}

func TestAvailableDescribes(t *testing.T) {
	ok, how := Available()
	if how == "" {
		t.Fatalf("Available() = %v with no description", ok)
	}
	t.Logf("Available() = %v, %q", ok, how)
}
