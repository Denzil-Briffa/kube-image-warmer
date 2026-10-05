package warmuphelper

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	installArgument  = "--install"
	completeArgument = "--complete"
)

func Run(args []string) error {
	if len(args) == 1 && args[0] == completeArgument {
		return nil
	}
	if len(args) != 2 || args[0] != installArgument || args[1] == "" {
		return fmt.Errorf("expected --complete or --install <destination>")
	}
	source, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate helper executable: %w", err)
	}
	return install(source, args[1])
}

func install(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open helper executable: %w", err)
	}
	defer func() { _ = input.Close() }()

	output, err := os.CreateTemp(filepath.Dir(destination), ".warmup-helper-*")
	if err != nil {
		return fmt.Errorf("create helper executable: %w", err)
	}
	defer func() {
		_ = output.Close()
		_ = os.Remove(output.Name())
	}()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy helper executable: %w", err)
	}
	if err := output.Chmod(0o755); err != nil {
		return fmt.Errorf("make helper executable: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close helper executable: %w", err)
	}
	if err := os.Rename(output.Name(), destination); err != nil {
		return fmt.Errorf("install helper executable: %w", err)
	}
	return nil
}
