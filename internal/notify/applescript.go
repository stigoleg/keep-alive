package notify

import "strings"

var appleScriptEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// appleScriptString quotes s as an AppleScript string literal. Only the
// backslash and the double quote need escaping; newlines may stay literal.
func appleScriptString(s string) string {
	return `"` + appleScriptEscaper.Replace(s) + `"`
}

// osascriptArgs builds the osascript arguments that show a notification.
func osascriptArgs(title, body string) []string {
	return []string{"-e", "display notification " + appleScriptString(body) + " with title " + appleScriptString(title)}
}
