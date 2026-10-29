package bridge

import (
	"context"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/diagnostics"
)

// doctorCheckWire is one check of Diagnostics: Status is "ok", "fail" or
// "skip", and Detail has had anything that looks like a secret removed.
type doctorCheckWire struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type doctorResult struct {
	ConfigPath    string            `json:"configPath"`
	BridgeVersion string            `json:"bridgeVersion"`
	Checks        []doctorCheckWire `json:"checks"`
	// Ready is true when no check failed.
	Ready bool `json:"ready"`
}

// doctor runs the terminal app's doctor, read-only, without its Terminal and
// State directory checks, and answers with each check. It answers before a
// server is named or a sign-in is made, and says so in its checks. A file that
// no longer validates fails the config check with its message, while the
// settings in use stay the last good ones.
func (s *Server) doctor(ctx context.Context) (*doctorResult, error) {
	options := diagnostics.DoctorOptions{Config: s.config, ConfigPath: s.configPath, NoTerminal: true}
	if s.config.Backend.Enabled() {
		options.Backend = &diagnostics.BackendOptions{URL: s.config.Backend.URL, Server: s.client, Zone: s.DeviceZone}
	}
	report := diagnostics.Run(ctx, options)
	result := &doctorResult{ConfigPath: s.configPath, BridgeVersion: s.Version, Checks: []doctorCheckWire{}, Ready: report.OK()}
	for _, check := range report.Checks {
		if check.Name == "Momentum config" && s.configError != "" {
			check.OK, check.Detail = false, s.configError
			result.Ready = false
		}
		status := "ok"
		switch {
		case check.Skipped:
			status = "skip"
		case !check.OK:
			status = "fail"
		}
		result.Checks = append(result.Checks, doctorCheckWire{Name: check.Name, Status: status, Detail: diagnostics.Redact(check.Detail)})
	}
	return result, nil
}
