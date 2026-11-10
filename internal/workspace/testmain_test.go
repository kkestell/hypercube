package workspace

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// All fixtures use local repositories, including submodules. Configure
	// their shared Git environment once, before independent tests run.
	_ = os.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	_ = os.Setenv("GIT_CONFIG_COUNT", "1")
	_ = os.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	_ = os.Setenv("GIT_CONFIG_VALUE_0", "always")
	os.Exit(m.Run())
}
