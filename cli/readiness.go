package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

type readinessResult struct {
	Status     string `json:"status"`
	Surface    string `json:"surface"`
	ReasonCode string `json:"reasonCode,omitempty"`
}

func readiness(home, surface string) readinessResult {
	if surface != "consumer" && surface != "writer" && surface != "search" {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "SURFACE_UNKNOWN"}
	}
	if !homeReady(home) {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "HOME_NOT_INITIALIZED"}
	}
	stores, err := ReadStores(home)
	if err != nil || stores.validateProfile() != nil {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "HOME_CONFIGURATION_INVALID"}
	}
	if surface == "writer" {
		if err := evidenceWriteProbe(home); err != nil {
			return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "EVIDENCE_STORE_UNWRITABLE"}
		}
	}
	return readinessResult{Status: "ready", Surface: surface}
}

func overallReadiness(home string) readinessResult {
	for _, surface := range []string{"consumer", "writer", "search"} {
		if result := readiness(home, surface); result.Status != "ready" {
			result.Surface = "all"
			return result
		}
	}
	return readinessResult{Status: "ready", Surface: "all"}
}

// evidenceWriteProbe verifies the real access target when it exists. For a new
// target it exercises the same directory durability primitive without
// appending a fake audit/access event or changing catalog state.
func evidenceWriteProbe(home string) error {
	accessPath := filepath.Join(home, "access.jsonl")
	if info, err := os.Stat(accessPath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("access evidence target is not a regular file")
		}
		file, err := os.OpenFile(accessPath, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(home, ".kc-ready-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write([]byte("ready\n")); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
