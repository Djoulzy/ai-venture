package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalRuntimeDiscovery(t *testing.T) {
	dir := t.TempDir()
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable := filepath.Join(dir, "bin", "aventure")
	if got := findLlamaBinary(executable, dir); got != name {
		t.Fatalf("fallback PATH incorrect : %s", got)
	}
	if err := os.Mkdir(filepath.Join(dir, "runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "runtime", name)
	if err := os.WriteFile(path, []byte("fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ executable, directory string }{
		{executable, ""}, {filepath.Join(dir, "aventure"), ""}, {"", dir},
	} {
		if got := findLlamaBinary(entry.executable, entry.directory); got != path {
			t.Fatalf("runtime non détecté : %s", got)
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if got := findLlamaBinary(executable, dir); got != name {
			t.Fatal("fichier non exécutable sélectionné")
		}
	}
}

func TestShippedRuntimeTakesPrecedenceOverWorkingDirectory(t *testing.T) {
	shipped, working := t.TempDir(), t.TempDir()
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, dir := range []string{shipped, working} {
		if err := os.Mkdir(filepath.Join(dir, "runtime"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "runtime", name), []byte("fixture"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	want := filepath.Join(shipped, "runtime", name)
	if got := findLlamaBinary(filepath.Join(shipped, "aventure"), working); got != want {
		t.Fatalf("priorité incorrecte : %s", got)
	}
}
