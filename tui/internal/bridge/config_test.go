package bridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

const (
	backendTable = "[backend]\nurl = \"https://momentum.example\"\n"
	planning6h   = backendTable + "[planning]\ndaily_capacity = \"6h\"\n"
	planning7h   = backendTable + "[planning]\ndaily_capacity = \"7h\"\n"
)

// configFile is a config.toml in a temporary folder that a test rewrites the
// way an editor or the terminal app would.
type configFile struct {
	t    *testing.T
	path string
	step int
}

func newConfigFile(t *testing.T, text string) *configFile {
	t.Helper()
	file := &configFile{t: t, path: filepath.Join(t.TempDir(), "config.toml")}
	file.write(text)
	return file
}

// write replaces the file and gives it a modification time of its own, since
// two writes in a row can share one.
func (f *configFile) write(text string) {
	f.t.Helper()
	if err := os.WriteFile(f.path, []byte(text), 0o600); err != nil {
		f.t.Fatal(err)
	}
	f.step++
	f.touch(time.Date(2026, 10, 14, 9, 0, f.step, 0, time.UTC))
}

func (f *configFile) touch(when time.Time) {
	f.t.Helper()
	if err := os.Chtimes(f.path, when, when); err != nil {
		f.t.Fatal(err)
	}
}

// server starts a bridge on the file as it is now.
func (f *configFile) server() *Server {
	f.t.Helper()
	settings, _, err := config.LoadWithOptions(config.LoadOptions{PathOverride: f.path})
	if err != nil {
		f.t.Fatal(err)
	}
	server := NewServer(func(string) Client { return &fakeClient{} }, settings, f.path)
	server.now = func() time.Time { return testNow }
	return server
}

func statusOf(t *testing.T, server *Server) *statusResult {
	t.Helper()
	result, err := server.dispatch(context.Background(), "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.(*statusResult)
}

func loadOf(t *testing.T, server *Server) *loadResult {
	t.Helper()
	result, err := server.dispatch(context.Background(), "load", nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.(*loadResult)
}

func TestAnEditToConfigAppliesOnTheNextLoad(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 360 {
		t.Fatalf("capacity=%d, want 360", got)
	}

	file.write(planning7h)
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 420 {
		t.Fatalf("capacity after the edit=%d, want 420", got)
	}

	file.write(backendTable + "[planning]\nenabled = false\n")
	if loadOf(t, server).Plan.Enabled {
		t.Fatal("planning should be off after the edit")
	}
}

func TestStatusReadsTheFileBeforeSettingsDoes(t *testing.T) {
	file := newConfigFile(t, "refresh_interval = \"1m\"\n"+backendTable+"[sync]\nmutation_delay = \"15s\"\n")
	server := file.server()
	if got := statusOf(t, server); got.RefreshIntervalSeconds != 60 || got.MutationDelaySeconds != 15 {
		t.Fatalf("status=%+v", got)
	}

	file.write("refresh_interval = \"2m\"\n" + backendTable + "[sync]\nmutation_delay = \"30s\"\n")
	if got := statusOf(t, server); got.RefreshIntervalSeconds != 120 || got.MutationDelaySeconds != 30 {
		t.Fatalf("status after the edit=%+v", got)
	}
}

func TestAFileThatNoLongerValidatesKeepsTheLastGoodSettings(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	loadOf(t, server)

	for name, bad := range map[string]string{
		"a value the config refuses": backendTable + "[calendar]\nenabled = true\n",
		"text that is not TOML":      backendTable + "[planning\n",
	} {
		file.write(bad)
		status := statusOf(t, server)
		if status.ConfigError == "" {
			t.Fatalf("%s: the status should carry the error", name)
		}
		if got := loadOf(t, server).Plan.CapacityMinutes; got != 360 {
			t.Fatalf("%s: capacity=%d, want the last good 360", name, got)
		}
	}

	file.write(planning7h)
	if status := statusOf(t, server); status.ConfigError != "" {
		t.Fatalf("a good file should clear the error, got %q", status.ConfigError)
	}
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 420 {
		t.Fatalf("capacity after the fix=%d, want 420", got)
	}
}

func TestAChangedBackendURLWaitsForARestart(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	if status := statusOf(t, server); status.RestartRequired {
		t.Fatal("nothing has changed yet")
	}

	file.write("[backend]\nurl = \"https://other.example\"\n[planning]\ndaily_capacity = \"7h\"\n")
	status := statusOf(t, server)
	if status.BackendURL != "https://momentum.example" || !status.RestartRequired {
		t.Fatalf("status=%+v: the old address stays and a restart is asked for", status)
	}
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 420 {
		t.Fatalf("capacity=%d: the rest of the file still applies", got)
	}

	file.write(planning7h)
	if status := statusOf(t, server); status.RestartRequired {
		t.Fatal("putting the address back should clear the restart notice")
	}
}

func TestOnlyAChangedFileIsReloaded(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	info, err := os.Stat(file.path)
	if err != nil {
		t.Fatal(err)
	}

	// The same length and the same modification time: no new version.
	if err := os.WriteFile(file.path, []byte(planning7h), 0o600); err != nil {
		t.Fatal(err)
	}
	file.touch(info.ModTime())
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 360 {
		t.Fatalf("capacity=%d: a file with the same modification time and size is not read again", got)
	}

	file.write(planning7h)
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 420 {
		t.Fatalf("capacity=%d, want 420 once the modification time changes", got)
	}
}

func TestTheConfigNoticeReachesTheAppInStatus(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	file.write("[backend]\nurl = \"https://other.example\"\n[calendar]\nenabled = true\n")

	var result map[string]any
	if err := json.Unmarshal(serve(t, server, request(t, 1, "status", nil))[1].Result, &result); err != nil {
		t.Fatal(err)
	}
	if message, _ := result["configError"].(string); message == "" {
		t.Fatalf("configError=%v", result["configError"])
	}

	file.write("[backend]\nurl = \"https://other.example\"\n")
	result = nil
	if err := json.Unmarshal(serve(t, server, request(t, 3, "status", nil))[3].Result, &result); err != nil {
		t.Fatal(err)
	}
	if result["restartRequired"] != true || result["configError"] != nil {
		t.Fatalf("result=%v", result)
	}
}
