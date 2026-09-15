package taskwarrior

import (
	"context"
	"strings"
	"testing"
)

func TestIntegrationVersionIsTaskwarriorThree(t *testing.T) {
	client, _ := isolatedClient(t)
	version, err := client.Version(context.Background())
	if err != nil || !strings.HasPrefix(version, "3.") {
		t.Fatalf("version=%q err=%v", version, err)
	}
}
