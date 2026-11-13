package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundleDetection(t *testing.T) {
	for _, executable := range []string{"/tmp/bin/aventure", "/tmp/foo/Contents/MacOS/aventure"} {
		if bundleResources(executable) != "" {
			t.Fatal("ordinary binary treated as an app")
		}
	}
	if got := bundleResources("/tmp/AI Venture.app/Contents/MacOS/aventure"); got != "/tmp/AI Venture.app/Contents/Resources" {
		t.Fatal(got)
	}
}

func TestConfigDiscoveryDoesNotChooseAnAdventure(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "aventure")
	if got := defaultGameConfig(executable, root); got != "" {
		t.Fatal(got)
	}
	path := filepath.Join(root, "game.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := defaultGameConfig(executable, root); got != path {
		t.Fatal(got)
	}
	app := filepath.Join(root, "Custom.app", "Contents", "MacOS", "aventure")
	if got := defaultGameConfig(app, root); got != "" {
		t.Fatal("app used working-directory configuration")
	}
	resources := bundleResources(app)
	if err := os.MkdirAll(resources, 0700); err != nil {
		t.Fatal(err)
	}
	appConfig := filepath.Join(resources, "game.json")
	if err := os.WriteFile(appConfig, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := defaultGameConfig(app, root); got != appConfig {
		t.Fatal(got)
	}
}
