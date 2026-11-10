// Package gittest gives a test binary a fixed Git environment. Every Go test
// package that runs Git, directly or through the code it tests, calls Isolate
// from its TestMain before any test starts.
package gittest

import "os"

// Isolate keeps the user's and the system's Git configuration out of the test
// process and its children, sets a fixed author and committer, and allows
// local file transports for submodule fixtures. The fixed identity keeps Git
// from deriving one from the machine's host name, a lookup that waits on the
// network.
func Isolate() {
	for name, value := range map[string]string{
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_COUNT":    "1",
		"GIT_CONFIG_KEY_0":    "protocol.file.allow",
		"GIT_CONFIG_VALUE_0":  "always",
		"GIT_AUTHOR_NAME":     "Test",
		"GIT_AUTHOR_EMAIL":    "test@example.com",
		"GIT_COMMITTER_NAME":  "Test",
		"GIT_COMMITTER_EMAIL": "test@example.com",
	} {
		_ = os.Setenv(name, value)
	}
}
