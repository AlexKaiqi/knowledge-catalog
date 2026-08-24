package connectorhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyncRepository updates the Host-owned read copy to one exact commit from the
// authoritative public Connector Git repository. The checkout is runtime
// material, never a second source of truth.
func SyncRepository(ctx context.Context, repositoryURL, ref, checkoutPath string) (string, error) {
	repositoryURL = NormalizeRepositoryLocation(repositoryURL)
	ref = strings.TrimSpace(ref)
	if repositoryURL == "" || strings.HasPrefix(repositoryURL, "-") {
		return "", fmt.Errorf("authoritative repository URL or path is required")
	}
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("safe repository ref is required")
	}
	checkout, err := filepath.Abs(checkoutPath)
	if err != nil {
		return "", err
	}
	gitPath := filepath.Join(checkout, ".git")
	if _, err := os.Stat(gitPath); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(checkout); err == nil {
			return "", fmt.Errorf("host checkout %s exists but is not a Git clone", checkout)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(checkout)
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return "", err
		}
		temporary, err := os.MkdirTemp(parent, ".repository-clone-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(temporary)
		// clone requires the destination not to exist.
		if err := os.Remove(temporary); err != nil {
			return "", err
		}
		if _, err := runGit(ctx, "clone", "--no-checkout", "--origin", "origin", repositoryURL, temporary); err != nil {
			return "", err
		}
		if err := os.Rename(temporary, checkout); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	configured, err := runGit(ctx, "-C", checkout, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(configured) != repositoryURL {
		return "", fmt.Errorf("host checkout origin is %q, configured repository is %q", strings.TrimSpace(configured), repositoryURL)
	}
	if _, err := runGit(ctx, "-C", checkout, "fetch", "--force", "--prune", "origin", ref); err != nil {
		return "", err
	}
	commit, err := runGit(ctx, "-C", checkout, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", err
	}
	commit = strings.TrimSpace(commit)
	if _, err := runGit(ctx, "-C", checkout, "checkout", "--detach", "--force", commit); err != nil {
		return "", err
	}
	if err := ValidateRepository(checkout); err != nil {
		return "", fmt.Errorf("synced repository contract: %w", err)
	}
	return commit, nil
}

// NormalizeRepositoryLocation makes local Git paths durable across service
// restarts while leaving SSH/HTTPS repository URLs untouched.
func NormalizeRepositoryLocation(location string) string {
	location = strings.TrimSpace(location)
	if info, err := os.Stat(location); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
		if absolute, err := filepath.Abs(location); err == nil {
			return absolute
		}
	}
	return location
}

func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
