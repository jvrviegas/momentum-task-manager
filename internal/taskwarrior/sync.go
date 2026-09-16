package taskwarrior

import (
	"context"
	"errors"
	"strings"
)

// Sync invokes Taskwarrior's native sync command and returns only safe output.
func (c *CommandClient) Sync(ctx context.Context) (SyncResult, error) {
	result, err := c.run(ctx, "sync", "sync")
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Changed: strings.TrimSpace(result.Stdout) != "", Output: Redact(strings.TrimSpace(result.Stdout))}, nil
}

// SyncConfigured reports whether all three Taskwarrior sync settings are
// present without returning their values.
func (c *CommandClient) SyncConfigured(ctx context.Context) (bool, error) {
	for _, key := range []string{"sync.server.url", "sync.server.client_id", "sync.encryption_secret"} {
		result, err := c.run(ctx, "sync-config", "_get", key)
		if err != nil {
			var commandErr *CommandError
			if errors.As(err, &commandErr) && strings.Contains(commandErr.Stderr, "not a DOM reference") {
				return false, nil
			}
			return false, err
		}
		if strings.TrimSpace(result.Stdout) == "" {
			return false, nil
		}
	}
	return true, nil
}
