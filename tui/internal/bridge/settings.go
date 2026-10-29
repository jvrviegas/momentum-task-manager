package bridge

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// CodeConfigConflict: config.toml changed after the Settings tab read it, so
// nothing was written.
const CodeConfigConflict = "configConflict"

// localOps never reach the server, so they work before a server is named and
// before a sign-in.
var localOps = map[string]bool{"status": true, "settings": true, "savePlanning": true, "saveCalendar": true, "doctor": true}

type settingsResult struct {
	ConfigPath string `json:"configPath"`
	// Revision names the version of config.toml the values were read from;
	// a save sends it back and is refused when the file has changed since.
	Revision        string               `json:"revision"`
	ConfigError     string               `json:"configError,omitempty"`
	RestartRequired bool                 `json:"restartRequired,omitempty"`
	Planning        planningSettingsWire `json:"planning"`
	Calendar        calendarSettingsWire `json:"calendar"`
}

// planningSettingsWire gives lengths as a person types them: 8h, 7h30m.
type planningSettingsWire struct {
	Enabled         bool              `json:"enabled"`
	DailyCapacity   string            `json:"dailyCapacity"`
	Buffer          string            `json:"buffer"`
	WeekdayCapacity map[string]string `json:"weekdayCapacity"`
}

type calendarSettingsWire struct {
	// Source is "files" or "google", as [calendar] source says. Enabled and
	// Files are about files only (ADR 0003, D9).
	Source  string             `json:"source"`
	Enabled bool               `json:"enabled"`
	Files   []calendarFileWire `json:"files"`
	// WorkingStart and WorkingEnd are HH:MM; StaleAfter is a length as a
	// person types it, and 0m means a file is never stale.
	WorkingStart       string `json:"workingStart"`
	WorkingEnd         string `json:"workingEnd"`
	IncludeAllDay      bool   `json:"includeAllDay"`
	IncludeTransparent bool   `json:"includeTransparent"`
	StaleAfter         string `json:"staleAfter"`
}

// calendarFileWire is one calendar file as stored, with what loading it on
// its own found. It never carries an event or a title.
type calendarFileWire struct {
	Path     string `json:"path"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Modified string `json:"modified,omitempty"`
}

type savePlanningParams struct {
	Revision        string            `json:"revision"`
	Enabled         *bool             `json:"enabled"`
	DailyCapacity   *string           `json:"dailyCapacity"`
	Buffer          *string           `json:"buffer"`
	WeekdayCapacity map[string]string `json:"weekdayCapacity"`
}

type saveCalendarParams struct {
	Revision           string    `json:"revision"`
	Source             *string   `json:"source"`
	Enabled            *bool     `json:"enabled"`
	Paths              *[]string `json:"paths"`
	WorkingStart       *string   `json:"workingStart"`
	WorkingEnd         *string   `json:"workingEnd"`
	IncludeAllDay      *bool     `json:"includeAllDay"`
	IncludeTransparent *bool     `json:"includeTransparent"`
	StaleAfter         *string   `json:"staleAfter"`
}

// calendarSource is the source [calendar] names, files when it names none.
func calendarSource(settings config.CalendarConfig) string {
	if settings.Source == "" {
		return config.CalendarSourceFiles
	}
	return settings.Source
}

// settings answers with what the Settings tabs show. The file has been read
// again if it changed, so the values are the ones in use.
func (s *Server) settings(ctx context.Context) (*settingsResult, error) {
	planning := s.config.Planning
	result := &settingsResult{
		ConfigPath:      s.configPath,
		ConfigError:     s.configError,
		RestartRequired: s.restartRequired,
		Planning: planningSettingsWire{
			Enabled:         planning.Enabled,
			DailyCapacity:   config.FormatLength(planning.DailyCapacity),
			Buffer:          config.FormatLength(planning.Buffer),
			WeekdayCapacity: map[string]string{},
		},
		Calendar: calendarSettingsWire{
			Source:             calendarSource(s.config.Calendar),
			Enabled:            s.config.Calendar.Enabled,
			Files:              []calendarFileWire{},
			WorkingStart:       s.config.Calendar.WorkingStart,
			WorkingEnd:         s.config.Calendar.WorkingEnd,
			IncludeAllDay:      s.config.Calendar.IncludeAllDay,
			IncludeTransparent: s.config.Calendar.IncludeTransparent,
			StaleAfter:         config.FormatLength(s.config.Calendar.StaleAfter),
		},
	}
	for day, value := range planning.WeekdayCapacity {
		result.Planning.WeekdayCapacity[strings.ToLower(day)] = value
	}
	if snapshot, err := config.NewProjectCatalogStore(s.configPath).Read(ctx); err == nil {
		result.Revision = config.RevisionID(snapshot.Revision)
	}

	home, _ := os.UserHomeDir()
	for _, path := range s.config.Calendar.Paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		status := calendar.Inspect(config.ExpandHome(path, home), s.now(), s.config.Calendar.StaleAfter)
		file := calendarFileWire{Path: path, Status: string(status.State), Detail: status.Detail}
		if !status.Modified.IsZero() {
			file.Modified = status.Modified.Format(time.RFC3339)
		}
		result.Calendar.Files = append(result.Calendar.Files, file)
	}
	return result, nil
}

// savePlanning writes the changed [planning] keys and answers with the
// settings as they are now.
func (s *Server) savePlanning(ctx context.Context, p savePlanningParams) (*settingsResult, error) {
	patch := config.PlanningPatch{Enabled: p.Enabled, DailyCapacity: p.DailyCapacity, Buffer: p.Buffer, WeekdayCapacity: p.WeekdayCapacity}
	return s.saveConfig(ctx, p.Revision, func(store *config.FileProjectCatalogStore, snapshot config.ProjectCatalogSnapshot) error {
		_, err := store.SavePlanning(ctx, snapshot, patch)
		return err
	})
}

// saveCalendar writes the changed [calendar] keys. A file under the home
// folder is kept as ~/..., and a value the config would refuse (the switch on
// with no files, hours that end before they start, a length that is not one)
// is refused with a message.
func (s *Server) saveCalendar(ctx context.Context, p saveCalendarParams) (*settingsResult, error) {
	home, _ := os.UserHomeDir()
	patch := config.CalendarPatch{
		Source: p.Source, Enabled: p.Enabled, Paths: p.Paths, WorkingStart: p.WorkingStart, WorkingEnd: p.WorkingEnd,
		IncludeAllDay: p.IncludeAllDay, IncludeTransparent: p.IncludeTransparent, StaleAfter: p.StaleAfter,
	}.Normalized(home)
	return s.saveConfig(ctx, p.Revision, func(store *config.FileProjectCatalogStore, snapshot config.ProjectCatalogSnapshot) error {
		if fieldErr := patch.Check(s.config.Calendar); fieldErr != nil {
			return fieldErr
		}
		_, err := store.SaveCalendar(ctx, snapshot, patch)
		return err
	})
}

// saveConfig runs one write on the file the Settings tab read. It refuses
// when the file changed since, or does not load, and otherwise reads the new
// file at once, so suggestions, the sidebar meter and Plan Today use the new
// values on the next load.
func (s *Server) saveConfig(ctx context.Context, revision string, save func(*config.FileProjectCatalogStore, config.ProjectCatalogSnapshot) error) (*settingsResult, error) {
	if s.configError != "" {
		return nil, &Error{Message: "Fix config.toml first. The app is still using the last settings that worked."}
	}
	store := config.NewProjectCatalogStore(s.configPath)
	snapshot, err := store.Read(ctx)
	if err != nil {
		return nil, &Error{Message: "Unable to read config.toml. " + err.Error()}
	}
	if config.RevisionID(snapshot.Revision) != revision {
		return nil, configConflict()
	}
	if err := save(store, snapshot); err != nil {
		var fieldErr *config.FieldError
		switch {
		case errors.As(err, &fieldErr):
			return nil, fieldError(fieldErr.Field, fieldErr.Message)
		case errors.Is(err, config.ErrProjectConfigConflict):
			return nil, configConflict()
		default:
			return nil, &Error{Message: "Unable to save config.toml. " + err.Error()}
		}
	}
	s.reloadConfig()
	return s.settings(ctx)
}

func configConflict() *Error {
	return &Error{
		Message: "config.toml changed outside Momentum. The tab now shows what it says; make the change again.",
		Code:    CodeConfigConflict,
	}
}
