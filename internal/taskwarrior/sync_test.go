package taskwarrior

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSyncUsesNativeSyncArgvAndSafeOutput(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: "Synced 2 changes\n"}}}}
	got, err := NewClientWithRunner("task", runner).Sync(context.Background())
	if err != nil || !got.Changed || got.Output != "Synced 2 changes" {
		t.Fatalf("result=%#v err=%v", got, err)
	}
	if !reflect.DeepEqual(runner.calls[0], []string{"task", "sync"}) {
		t.Fatalf("argv=%#v", runner.calls)
	}
}

func TestSyncConfiguredReturnsOnlyPresence(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "https://sync.example"}},
		{result: CommandResult{ExitCode: 0, Stdout: "client-id"}},
		{result: CommandResult{ExitCode: 0, Stdout: "secret"}},
	}}
	configured, err := NewClientWithRunner("task", runner).SyncConfigured(context.Background())
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	for index, key := range []string{"sync.server.url", "sync.server.client_id", "sync.encryption_secret"} {
		if !reflect.DeepEqual(runner.calls[index], []string{"task", "_get", key}) {
			t.Fatalf("call %d=%#v", index, runner.calls[index])
		}
	}
}

func TestSyncConfiguredRequiresEverySetting(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "url"}},
		{result: CommandResult{ExitCode: 0, Stdout: "client"}},
		{result: CommandResult{ExitCode: 0}},
	}}
	configured, err := NewClientWithRunner("task", runner).SyncConfigured(context.Background())
	if err != nil || configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
}

func TestSyncConfiguredDoesNotReturnSecretOnError(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 1, Stderr: "encryption_secret=do-not-log"}, err: errors.New("failed")}}}
	configured, err := NewClientWithRunner("task", runner).SyncConfigured(context.Background())
	if configured || err == nil || strings.Contains(err.Error(), "do-not-log") {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
}
