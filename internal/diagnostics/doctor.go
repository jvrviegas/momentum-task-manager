package diagnostics

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
)

// Check is one safe doctor result.
type Check struct {
	Name    string
	OK      bool
	Skipped bool
	Detail  string
}

// Report is the complete diagnostic result.
type Report struct {
	Checks []Check
}

func (r Report) OK() bool {
	for _, check := range r.Checks {
		if !check.OK && !check.Skipped {
			return false
		}
	}
	return true
}

// DoctorOptions contains injectable checks for deterministic tests.
type DoctorOptions struct {
	Client      taskwarrior.Client
	Binary      string
	Config      config.Config
	ConfigPath  string
	HomeDir     string
	StateDir    string
	Terminal    string
	LookPath    func(string) (string, error)
	MkdirAll    func(string, os.FileMode) error
	WritableDir func(string) error
}

// Run performs only read-only Taskwarrior calls and local filesystem checks.
// It never invokes add/modify/done/delete/start/stop/undo/sync.
func Run(ctx context.Context, options DoctorOptions) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.Binary == "" {
		options.Binary = "task"
	}
	if options.LookPath == nil {
		options.LookPath = exec.LookPath
	}
	if options.MkdirAll == nil {
		options.MkdirAll = os.MkdirAll
	}
	if options.WritableDir == nil {
		options.WritableDir = checkWritableDir
	}
	var report Report
	path, err := options.LookPath(options.Binary)
	if err != nil {
		report.Checks = append(report.Checks, Check{Name: "Taskwarrior executable", Detail: "not found; install Taskwarrior 3.x", OK: false})
	} else {
		report.Checks = append(report.Checks, Check{Name: "Taskwarrior executable", Detail: filepath.Clean(path), OK: true})
	}
	if versioner, ok := options.Client.(interface {
		Version(context.Context) (string, error)
	}); ok {
		version, versionErr := versioner.Version(ctx)
		report.Checks = append(report.Checks, Check{Name: "Taskwarrior version", Detail: safeVersion(version), OK: versionErr == nil && version != ""})
	} else {
		report.Checks = append(report.Checks, Check{Name: "Taskwarrior version", Detail: "version check unavailable", Skipped: true, OK: true})
	}

	if options.Client == nil {
		report.Checks = append(report.Checks, Check{Name: "Taskwarrior export", Detail: "client unavailable", OK: false})
	} else {
		_, exportErr := options.Client.ExportPending(ctx)
		report.Checks = append(report.Checks, Check{Name: "Taskwarrior export", Detail: errorDetail(exportErr, "pending export decodes"), OK: exportErr == nil})
	}
	configErr := options.Config.Validate()
	report.Checks = append(report.Checks, Check{Name: "Momentum config", Detail: errorDetail(configErr, configPathDetail(options.ConfigPath)), OK: configErr == nil})

	if !options.Config.Sync.Enabled {
		report.Checks = append(report.Checks, Check{Name: "Sync settings", Detail: "disabled by Momentum config", Skipped: true, OK: true})
	} else if reader, ok := options.Client.(interface {
		SyncConfigured(context.Context) (bool, error)
	}); ok {
		configured, syncErr := reader.SyncConfigured(ctx)
		detail := "missing Taskwarrior sync settings"
		if configured && syncErr == nil {
			detail = "server URL, client ID, and encryption secret are present"
		}
		if syncErr != nil {
			detail = "unable to read Taskwarrior sync settings"
		}
		report.Checks = append(report.Checks, Check{Name: "Sync settings", Detail: detail, OK: syncErr == nil && configured})
	} else {
		report.Checks = append(report.Checks, Check{Name: "Sync settings", Detail: "sync readiness check unavailable", Skipped: true, OK: true})
	}

	terminal := options.Terminal
	if terminal == "" {
		terminal = os.Getenv("TERM")
	}
	report.Checks = append(report.Checks, Check{Name: "Terminal", Detail: terminalDetail(terminal), OK: terminal != ""})
	stateDir := options.StateDir
	if stateDir == "" {
		stateDir, _ = ProcessStateDir()
	}
	if stateDir == "" {
		report.Checks = append(report.Checks, Check{Name: "State directory", Detail: "home directory unavailable", OK: false})
	} else {
		mkdirErr := options.MkdirAll(stateDir, 0o700)
		writeErr := error(nil)
		if mkdirErr == nil {
			writeErr = options.WritableDir(stateDir)
		}
		report.Checks = append(report.Checks, Check{Name: "State directory", Detail: errorDetail(firstError(mkdirErr, writeErr), stateDir), OK: mkdirErr == nil && writeErr == nil})
	}
	return report
}

// Print writes a concise, secret-free report.
func (r Report) Print(w io.Writer) {
	for _, check := range r.Checks {
		status := "ok"
		if check.Skipped {
			status = "skip"
		} else if !check.OK {
			status = "fail"
		}
		fmt.Fprintf(w, "[%s] %-24s %s\n", status, check.Name, taskwarrior.Redact(check.Detail))
	}
	if r.OK() {
		fmt.Fprintln(w, "Doctor: ready")
	} else {
		fmt.Fprintln(w, "Doctor: issues found")
	}
}

func checkWritableDir(path string) error {
	file, err := os.CreateTemp(path, ".momentum-doctor-")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func safeVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "not reported"
	}
	return value
}

func configPathDetail(path string) string {
	if path == "" {
		return "defaults"
	}
	return path
}

func terminalDetail(value string) string {
	if value == "" {
		return "TERM is not set"
	}
	return value
}

func errorDetail(err error, success string) string {
	if err == nil {
		return success
	}
	return taskwarrior.Redact(err.Error())
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}
