package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Launches and shells in a workspace get the environment the runtime's Env
// hook returns for it.
func TestWorkspaceEnvReachesAgentsAndShells(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	env := f.rt.Env
	f.rt.Env = func(workspace string) []string {
		if workspace != f.workspace.ID {
			return env(workspace)
		}
		return append(env(workspace), "GH_TOKEN=t")
	}
	s, err := f.rt.Create(f.workspace.ID, lead, false, "!env GH_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	f.until("the agent to report its GH_TOKEN", func() bool { return f.summary(s.ID) == "user:!env GH_TOKEN|text:t" })

	marker := filepath.Join(t.TempDir(), "token")
	f.setenv("HYPERCUBE_TEST_MARKER", marker)
	f.rt.Terminals.Command = []string{"/bin/sh", "-c", `printf %s "$GH_TOKEN" > "$HYPERCUBE_TEST_MARKER"; exec /bin/cat`}
	read := func() bool { data, _ := os.ReadFile(marker); return strings.TrimSpace(string(data)) == "t" }
	if _, err := f.rt.Shell(s.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	f.until("the session shell to see GH_TOKEN", read)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	f.until("the workspace shell to see GH_TOKEN", read)
}
