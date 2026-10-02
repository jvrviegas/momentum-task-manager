package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

const (
	ThemeAuto  = "auto"
	ThemeDark  = "dark"
	ThemeLight = "light"

	IconsUnicode = "unicode"
	IconsNerd    = "nerd"
	IconsASCII   = "ascii"
)

// SyncConfig controls when Momentum invokes Taskwarrior's native sync.
type SyncConfig struct {
	Enabled       bool          `toml:"enabled"`
	Interval      time.Duration `toml:"interval"`
	MutationDelay time.Duration `toml:"mutation_delay"`
	Startup       bool          `toml:"startup"`
	Shutdown      bool          `toml:"shutdown"`
}

// Config contains settings owned by Momentum. Taskwarrior sync credentials are
// deliberately not represented here; they remain in Taskwarrior configuration.
type Config struct {
	RefreshInterval time.Duration         `toml:"refresh_interval"`
	Theme           string                `toml:"theme"`
	Icons           string                `toml:"icons"`
	Sync            SyncConfig            `toml:"sync"`
	Projects        domain.ProjectCatalog `toml:"projects"`
}

// LoadOptions makes configuration loading deterministic in tests and callers
// that provide their own environment. Empty fields use the process environment.
type LoadOptions struct {
	PathOverride  string
	HomeDir       string
	XDGConfigHome string
	Env           map[string]string
}

// Defaults returns the approved no-config behavior.
func Defaults() Config {
	return Config{
		RefreshInterval: time.Minute,
		Theme:           ThemeAuto,
		Icons:           IconsUnicode,
		Sync: SyncConfig{
			Enabled:       true,
			Interval:      5 * time.Minute,
			MutationDelay: 15 * time.Second,
			Startup:       true,
			Shutdown:      true,
		},
	}
}

// Load resolves the standard path and loads it, falling back to defaults when
// no file exists.
func Load(pathOverride string) (Config, error) {
	config, _, err := LoadWithOptions(LoadOptions{PathOverride: pathOverride})
	return config, err
}

// LoadWithOptions resolves and loads a config file without consulting any
// user's file when PathOverride is supplied. The returned path is the resolved
// path, including when the file is absent.
func LoadWithOptions(options LoadOptions) (Config, string, error) {
	home := options.HomeDir
	if home == "" {
		home = lookup(options.Env, "HOME")
	}
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Config{}, "", fmt.Errorf("resolve home directory: %w", err)
		}
	}
	xdg := options.XDGConfigHome
	if xdg == "" {
		xdg = lookup(options.Env, "XDG_CONFIG_HOME")
	}
	path := ResolvePathFor(options.PathOverride, home, xdg)

	config := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, path, fmt.Errorf("read config %s: %w", path, err)
		}
	} else if err := decode(data, &config); err != nil {
		return Config{}, path, err
	}

	icons := lookup(options.Env, "MOMENTUM_ICONS")
	if icons == "" && options.Env == nil {
		icons = os.Getenv("MOMENTUM_ICONS")
	}
	if icons != "" {
		config.Icons = strings.ToLower(strings.TrimSpace(icons))
	}
	if err := config.Validate(); err != nil {
		return Config{}, path, err
	}
	return config, path, nil
}

// LoadFile loads a specific file, using defaults if it is absent.
func LoadFile(path string) (Config, error) {
	config, _, err := LoadWithOptions(LoadOptions{PathOverride: path})
	return config, err
}

// ResolvePath resolves an explicit path or the user's XDG/default config path.
func ResolvePath(pathOverride string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return ResolvePathFor(pathOverride, home, os.Getenv("XDG_CONFIG_HOME")), nil
}

// ResolvePathFor is the injectable path-resolution variant used by tests.
func ResolvePathFor(pathOverride, homeDir, xdgConfigHome string) string {
	if pathOverride != "" {
		return filepath.Clean(pathOverride)
	}
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "momentum", "config.toml")
	}
	return filepath.Join(homeDir, ".config", "momentum", "config.toml")
}

func decode(data []byte, config *Config) error {
	metadata, err := toml.Decode(string(data), config)
	if err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	unknown := metadata.Undecoded()
	if len(unknown) > 0 {
		keys := make([]string, 0, len(unknown))
		for _, key := range unknown {
			keys = append(keys, key.String())
		}
		return fmt.Errorf("config key %q is not recognized; remove it or check the spelling", strings.Join(keys, ", "))
	}
	return nil
}

// Validate checks enum and duration values after decoding and environment
// overrides have been applied.
func (c Config) Validate() error {
	if c.RefreshInterval < 0 {
		return fmt.Errorf("config key %q must be zero or a positive duration", "refresh_interval")
	}
	c.Theme = strings.ToLower(strings.TrimSpace(c.Theme))
	switch c.Theme {
	case ThemeAuto, ThemeDark, ThemeLight:
	default:
		return fmt.Errorf("config key %q must be one of auto, dark, or light", "theme")
	}
	c.Icons = strings.ToLower(strings.TrimSpace(c.Icons))
	switch c.Icons {
	case IconsUnicode, IconsNerd, IconsASCII:
	default:
		return fmt.Errorf("config key %q must be one of unicode, nerd, or ascii", "icons")
	}
	if c.Sync.Interval <= 0 {
		return fmt.Errorf("config key %q must be a positive duration", "sync.interval")
	}
	if c.Sync.MutationDelay < 0 {
		return fmt.Errorf("config key %q must be zero or a positive duration", "sync.mutation_delay")
	}
	return c.Projects.Validate()
}

// IsZero identifies omitted settings without requiring Config to be comparable.
func (c Config) IsZero() bool {
	return c.RefreshInterval == 0 && c.Theme == "" && c.Icons == "" && c.Sync == (SyncConfig{}) && c.Projects == nil
}

func lookup(values map[string]string, key string) string {
	if values == nil {
		return ""
	}
	return values[key]
}
