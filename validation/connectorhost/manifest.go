package connectorhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"kc/repository"
)

type LoadedConnector struct {
	Manifest   Manifest
	Dir        string
	Generation string
}

type DiscoveredConnector struct {
	ID     string
	Path   string
	Loaded *LoadedConnector
	Error  error
}

func ValidateRepository(repoPath string) error {
	root, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("open connector repository %s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("connector repository %s is not a directory", root)
	}
	info, err = os.Stat(filepath.Join(root, "connectors"))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("public Connector repository is missing connectors/")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("connector repository connectors path is not a directory")
	}
	return nil
}

func Discover(repoPath string) ([]LoadedConnector, error) {
	entries, err := InspectRepository(repoPath)
	if err != nil {
		return nil, err
	}
	out := make([]LoadedConnector, 0, len(entries))
	for _, entry := range entries {
		if entry.Error != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(entry.Path, "connector.yaml"), entry.Error)
		}
		out = append(out, *entry.Loaded)
	}
	return out, nil
}

// InspectRepository discovers every flat package without allowing one broken
// business directory to hide or suspend the others.
func InspectRepository(repoPath string) ([]DiscoveredConnector, error) {
	root, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	if err := ValidateRepository(root); err != nil {
		return nil, err
	}
	if err := rejectNestedManifests(root); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(root, "connectors", "*", "connector.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	out := make([]DiscoveredConnector, 0, len(matches))
	seen := map[string]struct{}{}
	for _, path := range matches {
		dir := filepath.Dir(path)
		id := filepath.Base(dir)
		entry := DiscoveredConnector{ID: id, Path: dir}
		loaded, err := LoadConnector(path)
		if err != nil {
			entry.Error = err
			out = append(out, entry)
			continue
		}
		if _, ok := seen[loaded.Manifest.Metadata.ID]; ok {
			entry.Error = fmt.Errorf("duplicate connector id %q", loaded.Manifest.Metadata.ID)
			out = append(out, entry)
			continue
		}
		seen[loaded.Manifest.Metadata.ID] = struct{}{}
		entry.Loaded = &loaded
		out = append(out, entry)
	}
	return out, nil
}

func LoadConnector(manifestPath string) (LoadedConnector, error) {
	f, err := os.Open(manifestPath)
	if err != nil {
		return LoadedConnector{}, err
	}
	defer f.Close()
	var manifest Manifest
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&manifest); err != nil {
		return LoadedConnector{}, fmt.Errorf("decode manifest: %w", err)
	}
	dir := filepath.Dir(manifestPath)
	if err := ValidateManifest(manifest, filepath.Base(dir)); err != nil {
		return LoadedConnector{}, err
	}
	digest, err := BundleDigest(dir)
	if err != nil {
		return LoadedConnector{}, err
	}
	return LoadedConnector{Manifest: manifest, Dir: dir, Generation: digest}, nil
}

func ValidateManifest(m Manifest, directoryID string) error {
	if m.APIVersion != APIVersion {
		return fmt.Errorf("apiVersion must be %q", APIVersion)
	}
	if m.Kind != Kind {
		return fmt.Errorf("kind must be %q", Kind)
	}
	id := strings.TrimSpace(m.Metadata.ID)
	if !validConnectorID(id) {
		return fmt.Errorf("metadata.id must use lowercase letters, digits and single hyphens, starting and ending with a letter or digit")
	}
	if directoryID != "" && id != directoryID {
		return fmt.Errorf("metadata.id %q must match directory %q", id, directoryID)
	}
	if len(m.Spec.Command) == 0 || strings.TrimSpace(m.Spec.Command[0]) == "" {
		return fmt.Errorf("spec.command is required")
	}
	if strings.TrimSpace(m.Metadata.Owner) == "" {
		return fmt.Errorf("metadata.owner is required in the shared connector repository")
	}
	if m.Spec.Maintenance.Representation != "current-state" {
		return fmt.Errorf("MVP supports maintenance.representation=current-state only")
	}
	if strings.TrimSpace(m.Spec.Target.Repository) == "" {
		return fmt.Errorf("spec.target.repository is required")
	}
	if m.Spec.Target.Ref == "" {
		m.Spec.Target.Ref = repository.DefaultRef
	}
	if err := m.Spec.Target.Scope.Protocol().Validate(); err != nil {
		return err
	}
	if _, err := connectorIntervals(m); err != nil {
		return err
	}
	if m.Spec.Runtime.Timeout != "" {
		if d, err := time.ParseDuration(m.Spec.Runtime.Timeout); err != nil || d <= 0 {
			return fmt.Errorf("spec.runtime.timeout must be a positive duration")
		}
	}
	if m.Spec.Access != nil {
		if err := validateAccessSpec(*m.Spec.Access); err != nil {
			return err
		}
	}
	return nil
}

func validateAccessSpec(spec AccessSpec) error {
	if spec.Protocol != "resource-access/v1" {
		return fmt.Errorf("spec.access.protocol must be resource-access/v1")
	}
	if len(spec.Command) == 0 || strings.TrimSpace(spec.Command[0]) == "" {
		return fmt.Errorf("spec.access.command is required")
	}
	if len(spec.Operations) == 0 {
		return fmt.Errorf("spec.access.operations is required")
	}
	seen := map[string]struct{}{}
	for _, operation := range spec.Operations {
		operation = strings.TrimSpace(operation)
		if operation == "" || !validAccessOperation(operation) {
			return fmt.Errorf("spec.access.operations must use lowercase letters, digits and single hyphens")
		}
		if _, ok := seen[operation]; ok {
			return fmt.Errorf("spec.access.operations contains duplicate %q", operation)
		}
		seen[operation] = struct{}{}
	}
	if spec.Timeout != "" {
		if d, err := time.ParseDuration(spec.Timeout); err != nil || d <= 0 {
			return fmt.Errorf("spec.access.timeout must be a positive duration")
		}
	}
	return nil
}

func validAccessOperation(value string) bool {
	if value == "" || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	previousHyphen := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			previousHyphen = false
		case r == '-' && !previousHyphen:
			previousHyphen = true
		default:
			return false
		}
	}
	return true
}

func validConnectorID(id string) bool {
	if id == "" || id[0] == '-' || id[len(id)-1] == '-' {
		return false
	}
	previousHyphen := false
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			previousHyphen = false
		case r == '-' && !previousHyphen:
			previousHyphen = true
		default:
			return false
		}
	}
	return true
}

func rejectNestedManifests(root string) error {
	connectorsDir := filepath.Join(root, "connectors")
	return filepath.WalkDir(connectorsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "connector.yaml" {
			return nil
		}
		rel, err := filepath.Rel(connectorsDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 2 {
			return fmt.Errorf("connector manifest %s violates flat layout; expected connectors/<connector-id>/connector.yaml", rel)
		}
		return nil
	})
}

func ValidateConnector(ctx context.Context, loaded LoadedConnector) error {
	if loaded.Manifest.Spec.Test == nil || len(loaded.Manifest.Spec.Test.Command) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, loaded.Manifest.Spec.Test.Command[0], loaded.Manifest.Spec.Test.Command[1:]...)
	cmd.Dir = loaded.Dir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("connector test failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func BundleDigest(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != dir && (entry.Name() == ".git" || entry.Name() == ".connector-state") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, rel := range files {
		if _, err := io.WriteString(h, rel+"\x00"); err != nil {
			return "", err
		}
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if _, err := h.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func connectorIntervals(m Manifest) ([]time.Duration, error) {
	var out []time.Duration
	for _, trigger := range m.Spec.Maintenance.Triggers {
		switch trigger.Kind {
		case "manual":
			if trigger.Every != "" {
				return nil, fmt.Errorf("manual trigger cannot declare every")
			}
		case "schedule":
			d, err := time.ParseDuration(trigger.Every)
			if err != nil || d <= 0 {
				return nil, fmt.Errorf("schedule trigger requires a positive every duration")
			}
			out = append(out, d)
		default:
			return nil, fmt.Errorf("MVP trigger kind must be manual or schedule")
		}
	}
	return out, nil
}

func connectorInterval(m Manifest) time.Duration {
	values, _ := connectorIntervals(m)
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
}
