package main

import (
	"os"
	"path/filepath"
	"strings"

	"ai-venture/internal/gameconfig"
)

func bundleResources(executable string) string {
	macos := filepath.Dir(executable)
	contents := filepath.Dir(macos)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(filepath.Dir(contents), ".app") {
		return ""
	}
	return filepath.Join(contents, "Resources")
}

func defaultGameConfig(executable, directory string) string {
	candidates := []string{}
	if resources := bundleResources(executable); resources != "" {
		candidates = append(candidates, filepath.Join(resources, gameconfig.Filename))
	} else {
		candidates = append(candidates, filepath.Join(directory, gameconfig.Filename))
		if executable != "" {
			candidates = append(candidates, filepath.Join(filepath.Dir(executable), gameconfig.Filename))
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
			return path
		}
	}
	return ""
}
