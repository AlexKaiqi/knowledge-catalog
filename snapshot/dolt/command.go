package dolt

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (r *DoltRepository) run(args ...string) (string, error) {
	bin := strings.TrimSpace(os.Getenv("KC_DOLT_BIN"))
	forceDocker := strings.TrimSpace(os.Getenv("KC_DOLT_FORCE_DOCKER")) == "1"
	var cmd *exec.Cmd
	if bin != "" {
		cmd = exec.Command(bin, args...)
		cmd.Dir = r.rootDir
	} else if found, err := exec.LookPath("dolt"); err == nil && !forceDocker {
		cmd = exec.Command(found, args...)
		cmd.Dir = r.rootDir
	} else {
		image := strings.TrimSpace(os.Getenv("KC_DOLT_DOCKER_IMAGE"))
		if image == "" {
			image = doltDockerImage
		}
		dockerArgs := []string{"run", "--rm", "-v", r.rootDir + ":/repo", "-w", "/repo", image}
		cmd = exec.Command("docker", append(dockerArgs, args...)...)
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("dolt %s: %s", strings.Join(args, " "), text)
	}
	return stripANSI(text), nil
}

func stripANSI(value string) string {
	for {
		start := strings.IndexByte(value, 0x1b)
		if start < 0 {
			return strings.TrimSpace(value)
		}
		end := strings.IndexByte(value[start:], 'm')
		if end < 0 {
			return strings.TrimSpace(value[:start])
		}
		value = value[:start] + value[start+end+1:]
	}
}

type doltRows struct {
	Rows []map[string]any `json:"rows"`
}

func (r *DoltRepository) query(query string) ([]map[string]any, error) {
	out, err := r.run("sql", "-r", "json", "-q", query)
	if err != nil {
		return nil, err
	}
	var result doltRows
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("decode Dolt JSON: %w (%s)", err, out)
	}
	return result.Rows, nil
}

func sqlString(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
