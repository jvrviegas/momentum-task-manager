package bridge

import (
	"os"
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// fileState identifies one version of config.toml: when it was last written
// and how long it is. A missing file is the zero value.
type fileState struct {
	modified time.Time
	size     int64
}

func statFile(path string) fileState {
	info, err := os.Stat(path)
	if err != nil {
		return fileState{}
	}
	return fileState{modified: info.ModTime(), size: info.Size()}
}

// reloadConfig reads config.toml again when the file is not the version the
// settings came from, so an edit in an editor or the terminal app applies
// without a restart. A file that no longer loads keeps the last good
// settings and says why until a good one is saved. A changed [backend] url
// waits for a restart, since the client for that server is made once.
//
// It runs on the goroutine that answers server operations, the only one that
// reads the settings.
func (s *Server) reloadConfig() {
	state := statFile(s.configPath)
	if state == s.configState {
		return
	}
	s.configState = state
	loaded, _, err := config.LoadWithOptions(config.LoadOptions{PathOverride: s.configPath})
	if err != nil {
		s.configError = err.Error()
		return
	}
	s.configError = ""
	s.restartRequired = loaded.Backend != s.startBackend
	loaded.Backend = s.startBackend
	s.config = loaded
}
