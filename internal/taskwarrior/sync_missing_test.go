package taskwarrior

import (
	"context"
	"errors"
	"testing"
)

func TestSyncConfiguredTreatsMissingDOMSettingAsLocalOnly(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 2, Stderr: "'rc.sync.server.url' is not a DOM reference."}, err: errors.New("exit status 2")}}}
	configured, err := NewClientWithRunner("task", runner).SyncConfigured(context.Background())
	if configured || err != nil {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
}
