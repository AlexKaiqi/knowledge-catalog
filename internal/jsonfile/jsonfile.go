package jsonfile

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

func Write(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".kc-json-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	line := append(body, '\n')
	if n, err := f.Write(line); err != nil {
		_ = f.Close()
		return err
	} else if n != len(line) {
		_ = f.Close()
		return io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// Grants, control state and provisioning acknowledgements use this helper.
	// Their success must include persistence of the rename, not just the bytes.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func Read(path string, dest any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dest)
}

func AppendJSONL(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	line := append(body, '\n')
	if n, writeErr := f.Write(line); writeErr != nil {
		_ = f.Close()
		return writeErr
	} else if n != len(line) {
		_ = f.Close()
		return io.ErrShortWrite
	}
	// Access evidence, audit trails, and hook outbox items use this primitive.
	// Success therefore means the append reached the filesystem durability
	// boundary, not merely the process page cache.
	if syncErr := f.Sync(); syncErr != nil {
		_ = f.Close()
		return syncErr
	}
	return f.Close()
}
