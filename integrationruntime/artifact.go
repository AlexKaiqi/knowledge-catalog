package integrationruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
)

func (r *Runtime) Build(ctx context.Context, spec ArtifactSpec) (Artifact, error) {
	if strings.TrimSpace(spec.ID) == "" || !filepath.IsAbs(spec.Directory) || spec.Executable == "" {
		return Artifact{}, kernel.Fail(kernel.ErrUsageInvalid, "artifact requires id, absolute directory and executable")
	}
	for _, name := range spec.CredentialEnv {
		if !credentialNameAllowed(name) {
			return Artifact{}, kernel.Fail(kernel.ErrUsageInvalid, "artifact credential reference is invalid or belongs to the KC deployment")
		}
	}
	if len(spec.Build) > 0 {
		command := exec.CommandContext(ctx, spec.Build[0], spec.Build[1:]...)
		command.Dir = spec.Directory
		command.Env = baseEnvironment()
		buildHome := filepath.Join(r.Directory, "build-home")
		if err := os.MkdirAll(buildHome, 0700); err != nil {
			return Artifact{}, err
		}
		command.Env = append(command.Env, "HOME="+buildHome, "GOCACHE="+filepath.Join(buildHome, "go-cache"))
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Run(); err != nil {
			return Artifact{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "integration build failed: %v", err)
		}
	}
	executable := spec.Executable
	if !filepath.IsAbs(executable) {
		executable = filepath.Join(spec.Directory, executable)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return Artifact{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return Artifact{}, kernel.Fail(kernel.ErrUsageInvalid, "artifact must be an executable file")
	}
	raw, err := os.ReadFile(executable)
	if err != nil {
		return Artifact{}, err
	}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	directory := filepath.Join(r.Directory, "artifacts")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return Artifact{}, err
	}
	stored := filepath.Join(directory, hash)
	if existing, readErr := os.ReadFile(stored); readErr != nil || !bytes.Equal(existing, raw) {
		if readErr != nil && !os.IsNotExist(readErr) {
			return Artifact{}, readErr
		}
		file, err := os.CreateTemp(directory, ".build-*")
		if err != nil {
			return Artifact{}, err
		}
		defer os.Remove(file.Name())
		if err := file.Chmod(0500); err != nil {
			_ = file.Close()
			return Artifact{}, err
		}
		if _, err := file.Write(raw); err != nil {
			_ = file.Close()
			return Artifact{}, err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return Artifact{}, err
		}
		if err := file.Close(); err != nil {
			return Artifact{}, err
		}
		if err := os.Rename(file.Name(), stored); err != nil {
			return Artifact{}, err
		}
		dir, err := os.Open(directory)
		if err != nil {
			return Artifact{}, err
		}
		err = dir.Sync()
		_ = dir.Close()
		if err != nil {
			return Artifact{}, err
		}
	}
	artifact := Artifact{ID: spec.ID, Executable: stored, Digest: hash, CredentialEnv: append([]string(nil), spec.CredentialEnv...), BuiltAt: time.Now()}
	err = r.transaction(true, func(tx *bolt.Tx) error {
		raw, err := json.Marshal(artifact)
		if err != nil {
			return err
		}
		return tx.Bucket(artifactsBucket).Put([]byte(spec.ID), raw)
	})
	return artifact, err
}

func credentialNameAllowed(name string) bool {
	if name == "" || strings.HasPrefix(name, "KC_") {
		return false
	}
	switch name {
	case "HOME", "PATH", "TMPDIR", "TMP", "TEMP", "SYSTEMROOT":
		return false
	}
	for i, char := range name {
		if char != '_' && !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(i > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func baseEnvironment() []string {
	result := []string{}
	for _, name := range []string{"PATH", "TMPDIR", "TMP", "TEMP", "SYSTEMROOT"} {
		if value, ok := os.LookupEnv(name); ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(raw []byte) (int, error) {
	if b.Len()+len(raw) > b.limit {
		return 0, fmt.Errorf("integration output exceeded %d bytes", b.limit)
	}
	return b.Buffer.Write(raw)
}

func collect(ctx context.Context, artifact Artifact, input CollectInput) (Collection, error) {
	raw, err := os.ReadFile(artifact.Executable)
	if err != nil {
		return Collection{}, err
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != artifact.Digest {
		return Collection{}, kernel.Fail(kernel.ErrPreconditionFailed, "built integration artifact was changed; rebuild and reactivate it")
	}
	environment := baseEnvironment()
	// These are location hints, not credentials. A provider that reads its
	// Observed set can invoke kc against this run's pinned server using the
	// same saved session, including refresh, without a second token copy.
	for _, name := range []string{"HOME", "KC_CONFIG_DIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	environment = append(environment, "KC_SERVER_URL="+input.Manifest.Server)
	for _, name := range artifact.CredentialEnv {
		value, ok := os.LookupEnv(name)
		if !ok || value == "" {
			return Collection{}, kernel.Fail(kernel.ErrUnauthenticated, "integration credential reference %s is unavailable", name)
		}
		environment = append(environment, name+"="+value)
	}
	body, err := json.Marshal(input)
	if err != nil {
		return Collection{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(input.Manifest.TimeoutSeconds)*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, artifact.Executable)
	command.Dir = filepath.Dir(artifact.Executable)
	command.Env = environment
	command.Stdin = bytes.NewReader(body)
	output := &boundedOutput{limit: 8 << 20}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return Collection{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "integration collection failed: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	var collection Collection
	if decoder.Decode(&collection) != nil {
		return Collection{}, kernel.Fail(kernel.ErrUsageInvalid, "integration output is not a valid Collection")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Collection{}, kernel.Fail(kernel.ErrUsageInvalid, "integration output must contain one JSON Collection")
	}
	return collection, nil
}
