package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tomledit "github.com/smm-h/go-toml-edit"

	"github.com/jvrviegas/momentum/internal/domain"
)

var (
	// ErrProjectConfigConflict means the source or symlink changed after the
	// caller took its snapshot. The store never overwrites such a source.
	ErrProjectConfigConflict = errors.New("project config changed since snapshot")
	// ErrProjectConfigReadOnly means the existing target has no write bits.
	ErrProjectConfigReadOnly = errors.New("project config target is read-only")
)

// ProjectCatalogStore persists the catalog independently of Taskwarrior.
type ProjectCatalogStore interface {
	Read(context.Context) (ProjectCatalogSnapshot, error)
	Save(context.Context, ProjectCatalogSnapshot, domain.ProjectCatalog) (ProjectCatalogSnapshot, error)
}

// FileProjectCatalogStore edits the catalog in one explicit Momentum config
// file. Its writes replace the resolved regular target, not the config path
// itself, so a symlink at Path remains a symlink.
type FileProjectCatalogStore struct {
	Path string
}

// ProjectCatalogRevision is the optimistic-concurrency identity captured by a
// catalog read. Digest covers source bytes; the remaining fields cover the
// path, link, target identity, and target permissions.
type ProjectCatalogRevision struct {
	Exists     bool
	Symlink    bool
	LinkTarget string
	TargetPath string
	TargetMode fs.FileMode
	Digest     [32]byte

	targetInfo os.FileInfo
}

// ProjectCatalogSnapshot is the catalog and source revision used for a save.
type ProjectCatalogSnapshot struct {
	Path     string
	Projects domain.ProjectCatalog
	Revision ProjectCatalogRevision
}

// NewProjectCatalogStore constructs a store for an explicit config path.
func NewProjectCatalogStore(path string) *FileProjectCatalogStore {
	if path != "" {
		path = filepath.Clean(path)
	}
	return &FileProjectCatalogStore{Path: path}
}

// Read returns the configured catalog and an optimistic source snapshot.
// Missing files are represented by an empty snapshot and are not created.
func (s *FileProjectCatalogStore) Read(ctx context.Context) (ProjectCatalogSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	path, err := s.resolvedPath()
	if err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	data, revision, err := readConfigSource(path)
	if err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("read project config %s: %w", path, err)
	}
	if !revision.Exists {
		return ProjectCatalogSnapshot{Path: path, Revision: revision}, nil
	}
	_, config, err := parseAndValidateConfig(data)
	if err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("validate project config %s: %w", path, err)
	}
	return ProjectCatalogSnapshot{
		Path:     path,
		Projects: cloneProjects(config.Projects),
		Revision: revision,
	}, nil
}

// Save validates and atomically replaces only the projects content represented
// by the supplied catalog. The source must still match snapshot.Revision.
func (s *FileProjectCatalogStore) Save(ctx context.Context, snapshot ProjectCatalogSnapshot, projects domain.ProjectCatalog) (ProjectCatalogSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	if err := projects.Validate(); err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	path, err := s.resolvedPath()
	if err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	if snapshot.Path != "" {
		snapshotPath, pathErr := filepath.Abs(filepath.Clean(snapshot.Path))
		if pathErr != nil || snapshotPath != path {
			return ProjectCatalogSnapshot{}, fmt.Errorf("%w: snapshot path does not match store path", ErrProjectConfigConflict)
		}
	}

	if snapshotNeedsRead(snapshot) {
		fresh, readErr := s.Read(ctx)
		if readErr != nil {
			return ProjectCatalogSnapshot{}, readErr
		}
		snapshot = fresh
	}
	source, revision, err := readConfigSource(path)
	if err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("read current project config %s: %w", path, err)
	}
	if !sameRevision(snapshot.Revision, revision) {
		return ProjectCatalogSnapshot{}, fmt.Errorf("%w: %s", ErrProjectConfigConflict, path)
	}

	doc, _, err := parseAndValidateConfig(source)
	if err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("validate current project config %s: %w", path, err)
	}
	if err := patchProjects(doc, projects); err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("edit projects in %s: %w", path, err)
	}
	output := normalizeNewlines(source, doc.Bytes())
	if _, _, err := parseAndValidateConfig(output); err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("validate edited project config %s: %w", path, err)
	}

	// An existing source needs no replacement when the source editor produced
	// identical bytes. A missing path is still deliberately created, including
	// when the requested catalog is empty.
	if revision.Exists && bytes.Equal(source, output) {
		return ProjectCatalogSnapshot{Path: path, Projects: cloneProjects(projects), Revision: revision}, nil
	}

	// Check again after parsing/editing and immediately before creating the
	// temporary target. This detects ordinary external edits and symlink swaps
	// as late as possible before the write begins.
	latestSource, latestRevision, err := readConfigSource(path)
	if err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("recheck current project config %s: %w", path, err)
	}
	if !sameRevision(snapshot.Revision, latestRevision) {
		return ProjectCatalogSnapshot{}, fmt.Errorf("%w: %s", ErrProjectConfigConflict, path)
	}
	if !bytes.Equal(source, latestSource) {
		return ProjectCatalogSnapshot{}, fmt.Errorf("%w: %s", ErrProjectConfigConflict, path)
	}
	if err := contextError(ctx); err != nil {
		return ProjectCatalogSnapshot{}, err
	}

	if !latestRevision.Exists {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return ProjectCatalogSnapshot{}, fmt.Errorf("create config directory: %w", err)
		}
		latestRevision.TargetPath = path
		latestRevision.TargetMode = 0o600
	} else if latestRevision.TargetMode.Perm()&0o222 == 0 {
		return ProjectCatalogSnapshot{}, fmt.Errorf("%w: %s", ErrProjectConfigReadOnly, latestRevision.TargetPath)
	}

	// Check once more after creating missing parent directories and before the
	// temporary file exists. The final check after writing closes the normal
	// external-edit window; a filesystem change racing the final check and
	// rename remains an inherent limitation of optimistic replacement.
	latestSource, latestRevision, err = readConfigSource(path)
	if err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("recheck project config before write: %w", err)
	}
	if !sameRevision(snapshot.Revision, latestRevision) || !bytes.Equal(source, latestSource) {
		return ProjectCatalogSnapshot{}, fmt.Errorf("%w: %s", ErrProjectConfigConflict, path)
	}
	if !latestRevision.Exists {
		latestRevision.TargetPath = path
		latestRevision.TargetMode = 0o600
	} else if latestRevision.TargetMode.Perm()&0o222 == 0 {
		return ProjectCatalogSnapshot{}, fmt.Errorf("%w: %s", ErrProjectConfigReadOnly, latestRevision.TargetPath)
	}

	if err := atomicReplace(latestRevision.TargetPath, output, latestRevision.TargetMode.Perm()); err != nil {
		return ProjectCatalogSnapshot{}, fmt.Errorf("replace project config %s: %w", path, err)
	}
	return s.Read(ctx)
}

func (s *FileProjectCatalogStore) resolvedPath() (string, error) {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return "", errors.New("project config path is not configured")
	}
	path, err := filepath.Abs(filepath.Clean(s.Path))
	if err != nil {
		return "", fmt.Errorf("resolve project config path: %w", err)
	}
	return path, nil
}

func parseAndValidateConfig(data []byte) (*tomledit.Document, Config, error) {
	doc, err := tomledit.Parse(data)
	if err != nil {
		return nil, Config{}, fmt.Errorf("parse TOML: %w", err)
	}
	config := Defaults()
	if err := decode(data, &config); err != nil {
		return nil, Config{}, err
	}
	if err := config.Validate(); err != nil {
		return nil, Config{}, err
	}
	return doc, config, nil
}

func patchProjects(doc *tomledit.Document, projects domain.ProjectCatalog) error {
	entry, exists := doc.Root().Get("projects")
	if !exists {
		return appendProjectTables(doc, projects)
	}
	switch entry.Kind() {
	case tomledit.EntryRecords:
		records, ok := entry.Records()
		if !ok {
			return errors.New("projects array-table entries are unavailable")
		}
		for i := 0; i < len(records) && i < len(projects); i++ {
			if err := setProjectTableFields(doc, fmt.Sprintf("projects[%d]", i), projects[i]); err != nil {
				return err
			}
		}
		for i := len(records) - 1; i >= len(projects); i-- {
			if err := doc.Delete(fmt.Sprintf("projects[%d]", i)); err != nil {
				return err
			}
		}
		for i := len(records); i < len(projects); i++ {
			if err := doc.NewArrayTable("projects"); err != nil {
				return err
			}
			if err := setProjectTableFields(doc, "projects[-1]", projects[i]); err != nil {
				return err
			}
		}
		return nil
	case tomledit.EntryValue:
		node, ok := entry.Node()
		if !ok || node.Type() != tomledit.NodeArray {
			return errors.New("projects must be an array or array of tables")
		}
		return doc.Set("projects", inlineProjectArray(projects))
	default:
		return fmt.Errorf("projects has unsupported TOML kind %s", entry.Kind())
	}
}

func appendProjectTables(doc *tomledit.Document, projects domain.ProjectCatalog) error {
	for _, project := range projects {
		if err := doc.NewArrayTable("projects"); err != nil {
			return err
		}
		if err := setProjectTableFields(doc, "projects[-1]", project); err != nil {
			return err
		}
	}
	return nil
}

func setProjectTableFields(doc *tomledit.Document, path string, project domain.Project) error {
	if err := doc.Set(path+".name", project.Name); err != nil {
		return err
	}
	return doc.Set(path+".value", project.Value)
}

func inlineProjectArray(projects domain.ProjectCatalog) []any {
	values := make([]any, len(projects))
	for i, project := range projects {
		values[i] = []tomledit.Pair{
			{Key: "name", Value: project.Name},
			{Key: "value", Value: project.Value},
		}
	}
	return values
}

type configPathObservation struct {
	revision ProjectCatalogRevision
}

func readConfigSource(path string) ([]byte, ProjectCatalogRevision, error) {
	before, err := observeConfigPath(path)
	if err != nil {
		return nil, ProjectCatalogRevision{}, err
	}
	if !before.revision.Exists {
		return nil, before.revision, nil
	}
	data, err := os.ReadFile(before.revision.TargetPath)
	if err != nil {
		return nil, ProjectCatalogRevision{}, err
	}
	after, err := observeConfigPath(path)
	if err != nil {
		return nil, ProjectCatalogRevision{}, err
	}
	if !sameMetadata(before.revision, after.revision) {
		return nil, ProjectCatalogRevision{}, fmt.Errorf("%w while reading %s", ErrProjectConfigConflict, path)
	}
	before.revision.Digest = sha256.Sum256(data)
	return data, before.revision, nil
}

func observeConfigPath(path string) (configPathObservation, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return configPathObservation{revision: ProjectCatalogRevision{TargetPath: path}}, nil
	}
	if err != nil {
		return configPathObservation{}, err
	}

	revision := ProjectCatalogRevision{
		Exists:     true,
		Symlink:    info.Mode()&os.ModeSymlink != 0,
		TargetPath: path,
	}
	if revision.Symlink {
		revision.LinkTarget, err = os.Readlink(path)
		if err != nil {
			return configPathObservation{}, err
		}
	}
	revision.TargetPath, err = filepath.EvalSymlinks(path)
	if err != nil {
		return configPathObservation{}, fmt.Errorf("resolve config target: %w", err)
	}
	revision.TargetPath, err = filepath.Abs(revision.TargetPath)
	if err != nil {
		return configPathObservation{}, err
	}
	targetInfo, err := os.Stat(revision.TargetPath)
	if err != nil {
		return configPathObservation{}, err
	}
	if !targetInfo.Mode().IsRegular() {
		return configPathObservation{}, fmt.Errorf("config target %s is not a regular file", revision.TargetPath)
	}
	revision.TargetMode = targetInfo.Mode().Perm()
	revision.targetInfo = targetInfo
	return configPathObservation{revision: revision}, nil
}

func snapshotNeedsRead(snapshot ProjectCatalogSnapshot) bool {
	return snapshot.Path == "" && !snapshot.Revision.Exists && snapshot.Revision.TargetPath == "" && snapshot.Revision.Digest == ([32]byte{})
}

func sameRevision(expected, current ProjectCatalogRevision) bool {
	return sameMetadata(expected, current) && expected.Digest == current.Digest
}

func sameMetadata(left, right ProjectCatalogRevision) bool {
	if left.Exists != right.Exists || left.Symlink != right.Symlink || left.LinkTarget != right.LinkTarget || left.TargetPath != right.TargetPath || left.TargetMode.Perm() != right.TargetMode.Perm() {
		return false
	}
	if left.targetInfo == nil || right.targetInfo == nil {
		return left.targetInfo == nil && right.targetInfo == nil
	}
	return os.SameFile(left.targetInfo, right.targetInfo)
}

func atomicReplace(path string, data []byte, mode fs.FileMode) error {
	if mode.Perm() == 0 {
		mode = 0o600
	}
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".momentum-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(tempPath)
	}
	if err := writeFileBytes(file, data); err != nil {
		cleanup()
		return err
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(tempPath, mode.Perm()); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func writeFileBytes(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// normalizeNewlines keeps a uniformly CRLF source from gaining mixed line
// endings when the editor creates new structural nodes. Mixed sources are left
// untouched because their original line-ending choices are part of their text.
func normalizeNewlines(source, output []byte) []byte {
	if !containsCRLF(source) || containsBareLF(source) || !containsBareLF(output) {
		return output
	}
	normalized := bytes.ReplaceAll(output, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(normalized, []byte("\n"), []byte("\r\n"))
}

func containsCRLF(data []byte) bool {
	return bytes.Contains(data, []byte("\r\n"))
}

func containsBareLF(data []byte) bool {
	for i, b := range data {
		if b == '\n' && (i == 0 || data[i-1] != '\r') {
			return true
		}
	}
	return false
}

func cloneProjects(projects domain.ProjectCatalog) domain.ProjectCatalog {
	if projects == nil {
		return nil
	}
	return append(domain.ProjectCatalog(nil), projects...)
}
