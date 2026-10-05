package warmuphelper

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{nil, {""}, {"--invalid-entrypoint"}, {installArgument}, {installArgument, ""}} {
		if err := Run(args); err == nil {
			t.Errorf("expected invalid arguments %q to fail", args)
		}
	}
	if err := Run([]string{completeArgument}); err != nil {
		t.Fatalf("harmless completion failed: %v", err)
	}
}

func TestInstallCopiesExecutableAndReplacesPreviousFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper supports Linux file permissions")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "installed")
	const content = "executable fixture"
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := install(source, destination); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != content {
		t.Fatalf("installed contents %q, wanted %q", actual, content)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("installed permissions %v, wanted 0755", info.Mode().Perm())
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("temporary installation files leaked: %v", files)
	}
}

func TestInstallReportsFailures(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("helper"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("missing source", func(t *testing.T) {
		if err := install(filepath.Join(dir, "absent"), filepath.Join(dir, "destination")); err == nil {
			t.Fatal("expected missing source to fail")
		}
	})
	t.Run("missing destination directory", func(t *testing.T) {
		if err := install(source, filepath.Join(dir, "absent", "destination")); err == nil {
			t.Fatal("expected missing destination directory to fail")
		}
	})
	t.Run("destination is directory", func(t *testing.T) {
		if err := install(source, dir); err == nil {
			t.Fatal("expected installation over a directory to fail")
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("failed installation leaked temporary files: %v", files)
		}
	})
}

func TestRunInstallsCurrentExecutable(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "helper")
	if err := Run([]string{installArgument, destination}); err != nil {
		t.Fatal(err)
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	installedInfo, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if sourceInfo.Size() != installedInfo.Size() {
		t.Fatal("installer did not copy the complete current executable")
	}
}
