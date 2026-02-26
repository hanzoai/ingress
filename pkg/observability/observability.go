package observability

import (
	"fmt"
	"os"
)

func EnsureUserEnvVar() error {
	if os.Getenv("USER") == "" {
		if err := os.Setenv("USER", "ingress"); err != nil {
			return fmt.Errorf("could not set USER environment variable: %w", err)
		}
	}
	return nil
}
