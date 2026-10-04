package notify

import (
	"encoding/base64"
	"encoding/xml"
	"strings"
	"unicode/utf16"
)

// powershellAUMID is the AppUserModelID of Windows PowerShell. A toast must
// name a registered app; this one exists on every Windows 10 and 11 system.
const powershellAUMID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// xmlEscape escapes s for XML text, including both quotes, newlines and
// characters XML does not allow (replaced by U+FFFD).
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s)) // a strings.Builder never fails
	return b.String()
}

// toastScript is the PowerShell that shows a toast. The escaped XML has no
// quote characters, so it cannot end the single-quoted here-string early.
func toastScript(title, body string) string {
	return `$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml(@'
<toast><visual><binding template="ToastGeneric"><text>` + xmlEscape(title) + `</text><text>` + xmlEscape(body) + `</text></binding></visual></toast>
'@)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('` + powershellAUMID + `').Show($toast)
`
}

// powershellArgs runs script through -EncodedCommand (base64 of UTF-16LE),
// which sidesteps command-line quoting and console code pages.
func powershellArgs(script string) []string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 2*len(units))
	for i, u := range units {
		raw[2*i] = byte(u)
		raw[2*i+1] = byte(u >> 8)
	}
	return []string{"-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw)}
}
