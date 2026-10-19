package taskwarrior

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// isolatedEnvironment is mandatory for every real Taskwarrior integration
// test. It is created before the first client command and rejects the user's
// default paths before TASKRC/TASKDATA are installed.
type isolatedEnvironment struct {
	Root    string
	TaskRC  string
	DataDir string
}

func newIsolatedEnvironment(t *testing.T) isolatedEnvironment {
	t.Helper()
	if _, err := exec.LookPath("task"); err != nil {
		t.Skip("Taskwarrior 3.x binary unavailable; isolated integration skipped")
	}
	root := t.TempDir()
	env := isolatedEnvironment{Root: root, TaskRC: filepath.Join(root, "taskrc"), DataDir: filepath.Join(root, "data")}
	if err := os.MkdirAll(env.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{env.TaskRC, env.DataDir} {
		if filepath.Clean(path) == filepath.Join(home, ".taskrc") || filepath.Clean(path) == filepath.Join(home, ".task") {
			t.Fatalf("integration guard rejected production Taskwarrior path %q", path)
		}
	}
	config := "data.location=" + env.DataDir + "\nconfirmation=no\nhooks=off\nverbose=no\n"
	if err := os.WriteFile(env.TaskRC, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	// t.Setenv restores both process variables after this test. No test in this
	// file is parallel because Taskwarrior reads process environment.
	t.Setenv("TASKRC", env.TaskRC)
	t.Setenv("TASKDATA", env.DataDir)
	if got := os.Getenv("TASKRC"); filepath.Clean(got) == filepath.Join(home, ".taskrc") {
		t.Fatal("integration guard detected real TASKRC after setup")
	}
	return env
}

func isolatedClient(t *testing.T) (*CommandClient, isolatedEnvironment) {
	t.Helper()
	env := newIsolatedEnvironment(t)
	return NewClient("task"), env
}

func TestIntegrationNoRealDatabasePathCanBeSelected(t *testing.T) {
	_, env := isolatedClient(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(env.TaskRC) == filepath.Join(home, ".taskrc") || filepath.Clean(env.DataDir) == filepath.Join(home, ".task") {
		t.Fatal("real database path selected")
	}
}
