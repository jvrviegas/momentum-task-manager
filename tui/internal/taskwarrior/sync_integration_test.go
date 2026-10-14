package taskwarrior

import (
	"context"
	"os"
	"testing"
)

// This checks readiness only; the loopback URL is never contacted because the
// test does not call Sync.
func TestIntegrationSyncConfiguredReadsTaskwarriorDOMReferences(t *testing.T) {
	client, env := isolatedClient(t)
	if configured, err := client.SyncConfigured(context.Background()); err != nil || configured {
		t.Fatalf("unconfigured profile: configured=%v err=%v", configured, err)
	}

	file, err := os.OpenFile(env.TaskRC, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("sync.server.url=http://127.0.0.1:1\nsync.server.client_id=00000000-0000-4000-8000-000000000001\nsync.encryption_secret=0000000000000000000000000000000000000000000000000000000000000000\n")
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	configured, err := client.SyncConfigured(context.Background())
	if err != nil || !configured {
		t.Fatalf("configured isolated profile: configured=%v err=%v", configured, err)
	}
}
