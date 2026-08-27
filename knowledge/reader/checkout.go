package reader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

const (
	CheckoutPinFile = ".kc-pin.json"
	GrepProvider    = "grep"
)

var checkoutDirRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// GrepLimitations is the 7.10 declaration for the checkout grep Provider.
var GrepLimitations = []string{"no-tokenization", "no-synonyms", "no-cross-repo-ranking"}

// CheckoutPin is the on-disk pin for one Workspace checkout. Same coordinates as
// WorkspacePin; provider states this tree is grep, not FTS or vector.
type CheckoutPin struct {
	WorkspaceID  string                                  `json:"workspaceId"`
	Revision     int                                     `json:"revision"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	Provider     string                                  `json:"provider"`
	Limitations  []string                                `json:"limitations"`
}

// CheckoutReport is what WriteCheckout / kc checkout return.
type CheckoutReport struct {
	WorkspaceID string      `json:"workspaceId"`
	Revision    int         `json:"revision"`
	Dir         string      `json:"dir"`
	Objects     int         `json:"objects"`
	Pin         CheckoutPin `json:"pin"`
}

// EncodeCheckoutDir turns a repository or workspace id into a single path segment.
// Same rule as cli.EncodeRepoDir: kr://acme/public/core → kr_acme_public_core.
func EncodeCheckoutDir(id string) string {
	return checkoutDirRe.ReplaceAllString(id, "_")
}

func PinFromWorkspace(pin WorkspacePin) CheckoutPin {
	return CheckoutPin{
		WorkspaceID:  pin.WorkspaceID,
		Revision:     pin.Revision,
		Repositories: pin.Repositories,
		Provider:     GrepProvider,
		Limitations:  append([]string{}, GrepLimitations...),
	}
}

// ObjectCheckoutRel is 仓内相对路径：object_id + ".json"。不用 pathHint。
func ObjectCheckoutRel(objectID knowledge.ObjectID) (string, error) {
	raw := strings.TrimSpace(string(objectID))
	if raw == "" {
		return "", fmt.Errorf("object_id is required")
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("object_id must be a relative path: %s", objectID)
	}
	for _, part := range strings.Split(filepath.ToSlash(raw), "/") {
		if part == ".." {
			return "", fmt.Errorf("object_id escapes checkout root: %s", objectID)
		}
	}
	rel := filepath.ToSlash(raw) + ".json"
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("object_id escapes checkout root: %s", objectID)
	}
	return clean, nil
}

// WriteCheckout materializes one ResolveWorkspace as a read-only tree.
// Path = <encoded Repository> / <object_id>.json (assembled READ value, not Git blob).
// Same object_id in two repositories is two files. Replace the whole tree on each call.
func WriteCheckout(root string, pin WorkspacePin, values []FederatedValue) (CheckoutReport, error) {
	builder, err := BeginCheckout(root, pin)
	if err != nil {
		return CheckoutReport{}, err
	}
	defer builder.Abort()
	if err := builder.Add(values); err != nil {
		return CheckoutReport{}, err
	}
	return builder.Commit()
}

// CheckoutBuilder writes bounded pages into a candidate directory and only
// publishes after every page succeeds.
type CheckoutBuilder struct {
	root      string
	tmp       string
	pin       WorkspacePin
	written   CheckoutPin
	objects   int
	committed bool
}

func BeginCheckout(root string, pin WorkspacePin) (*CheckoutBuilder, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("checkout root is required")
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(parent, ".kc-checkout-*")
	if err != nil {
		return nil, err
	}
	written := PinFromWorkspace(pin)
	if err := writeCheckoutFile(filepath.Join(tmp, CheckoutPinFile), written); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	return &CheckoutBuilder{root: root, tmp: tmp, pin: pin, written: written}, nil
}

func (b *CheckoutBuilder) Add(values []FederatedValue) error {
	if b == nil || b.committed || b.tmp == "" {
		return fmt.Errorf("checkout builder is closed")
	}
	for _, item := range values {
		if err := writeCheckoutObject(b.tmp, item); err != nil {
			return err
		}
		b.objects++
	}
	return nil
}

func (b *CheckoutBuilder) Commit() (CheckoutReport, error) {
	if b == nil || b.committed || b.tmp == "" {
		return CheckoutReport{}, fmt.Errorf("checkout builder is closed")
	}
	if err := replaceCheckoutDir(b.root, b.tmp); err != nil {
		return CheckoutReport{}, err
	}
	b.committed = true
	b.tmp = ""
	return CheckoutReport{
		WorkspaceID: b.pin.WorkspaceID,
		Revision:    b.pin.Revision,
		Dir:         b.root,
		Objects:     b.objects,
		Pin:         b.written,
	}, nil
}

func (b *CheckoutBuilder) Abort() {
	if b == nil || b.committed || b.tmp == "" {
		return
	}
	_ = os.RemoveAll(b.tmp)
	b.tmp = ""
}

func writeCheckoutObject(root string, item FederatedValue) error {
	repoDir := EncodeCheckoutDir(string(item.Repository))
	if repoDir == "" || repoDir == "_" {
		return fmt.Errorf("repository id is required")
	}
	rel, err := ObjectCheckoutRel(item.ObjectID)
	if err != nil {
		return err
	}
	path := filepath.Join(root, repoDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeCheckoutFile(path, item.Value)
}

func writeCheckoutFile(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o444); err != nil {
		return err
	}
	return os.Chmod(path, 0o444)
}

func replaceCheckoutDir(dest, src string) error {
	if err := makeCheckoutWritable(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(src, dest)
}

func makeCheckoutWritable(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644)
	})
}
