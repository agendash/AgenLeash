package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BootstrapEnvironment loads a conventional agenleash env file before config is resolved.
// Existing process environment variables always win over file-based defaults.
func BootstrapEnvironment() error {
	for _, candidate := range envFileCandidates() {
		if candidate == "" {
			continue
		}
		if err := loadEnvFile(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func envFileCandidates() []string {
	var candidates []string
	if path := strings.TrimSpace(os.Getenv("AGENLEASH_ENV_FILE")); path != "" {
		candidates = append(candidates, path)
	}

	candidates = append(candidates, "agenleash.env", ".env")

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "etc", "agenleash", "agenleash.env"),
			filepath.Join(exeDir, "..", "etc", "agenleash.env"),
		)
	}

	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		candidates = append(candidates, filepath.Join(configDir, "agenleash", "agenleash.env"))
	}

	candidates = append(candidates, filepath.Join(string(os.PathSeparator), "etc", "agenleash", "agenleash.env"))
	return normalizePathList(candidates)
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("parse %s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("parse %s:%d: missing key", path, lineNo)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s from %s:%d: %w", key, path, lineNo, err)
		}
	}
	return scanner.Err()
}
