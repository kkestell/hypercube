package github

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Keep Git's test environment fixed before parallel tests start. Each
	// watcher still owns its database, repositories, CLI, and API server.
	_ = os.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Exit(m.Run())
}
