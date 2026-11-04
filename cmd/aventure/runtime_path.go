package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// Prefer the runtime shipped with the game, then the workspace build, then PATH.
func defaultLlamaBinary() string {
	executable, _ := os.Executable()
	directory, _ := os.Getwd()
	return findLlamaBinary(executable, directory)
}

func findLlamaBinary(executable, directory string) string {
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{}
	if executable != "" {
		folder := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(folder, "runtime", name), filepath.Join(folder, "..", "runtime", name))
	}
	if directory != "" {
		candidates = append(candidates, filepath.Join(directory, "runtime", name))
	}
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0) {
			return filepath.Clean(path)
		}
	}
	return name
}
