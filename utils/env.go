package utils

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnv reads a .env file and sets the environment variables.
// It silently skips missing files, making it safe for production deployments.
func LoadEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Safe to skip if the .env file does not exist
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip quotes if present (e.g. KEY="VALUE" or KEY='VALUE')
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}

		if key != "" {
			os.Setenv(key, val)
		}
	}

	return scanner.Err()
}
