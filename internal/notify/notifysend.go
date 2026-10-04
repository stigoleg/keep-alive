package notify

// notifySendArgs builds the notify-send arguments; "--" keeps a title that
// starts with a dash from being read as an option.
func notifySendArgs(title, body string) []string {
	return []string{"--app-name=" + appName, "--", title, body}
}
