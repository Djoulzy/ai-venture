package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"time"

	"ai-venture/internal/inference"
	"ai-venture/internal/llamacpp"
	"ai-venture/internal/locale"
	"ai-venture/internal/modelruntime"
	"ai-venture/internal/ollama"
)

type backendConfig struct {
	provider       string
	model          string
	ollamaURL      string
	ollamaBinary   string
	llamaURL       string
	llamaBinary    string
	gguf           string
	llamaContext   int
	autoStart      bool
	timeout        time.Duration
	startupTimeout time.Duration
}

// Ollama tags and llama.cpp aliases have separate defaults. In particular,
// OLLAMA_MODEL must never label an unrelated GGUF as LFM3.
func resolveModel(provider, model, gguf string) (string, error) {
	if model != "" {
		return model, nil
	}
	switch provider {
	case "ollama":
		return env("OLLAMA_MODEL", "lfm3:1.7b"), nil
	case "llamacpp":
		if gguf != "" {
			return filepath.Base(gguf), nil
		}
		return "", locale.Errorf("llama.cpp requires -gguf to select a local file, or -model to select an existing server model")
	default:
		return "", locale.Errorf("unknown provider %q: use ollama or llamacpp", provider)
	}
}

func startBackend(ctx context.Context, cfg backendConfig) (inference.Client, *modelruntime.Service, error) {
	var client inference.Client
	var probe func(context.Context) (bool, error)
	var command func() (*exec.Cmd, error)
	switch cfg.provider {
	case "ollama":
		c, err := ollama.New(cfg.ollamaURL, cfg.model, cfg.timeout)
		if err != nil {
			return nil, nil, err
		}
		client, probe = c, c.Ready
		command = func() (*exec.Cmd, error) { return modelruntime.OllamaCommand(cfg.ollamaBinary, cfg.ollamaURL) }
	case "llamacpp":
		c, err := llamacpp.New(cfg.llamaURL, cfg.model, cfg.timeout)
		if err != nil {
			return nil, nil, err
		}
		client = c
		probe = func(ctx context.Context) (bool, error) {
			ready, err := c.Ready(ctx)
			if err != nil || !ready || cfg.gguf == "" {
				return ready, err
			}
			if err := c.VerifyModelFile(ctx, cfg.gguf); err != nil {
				return false, err
			}
			return true, nil
		}
		command = func() (*exec.Cmd, error) {
			return modelruntime.LlamaCommand(cfg.llamaBinary, cfg.llamaURL, cfg.gguf, cfg.model, cfg.llamaContext)
		}
	default:
		return nil, nil, locale.Errorf("unknown provider %q: use ollama or llamacpp", cfg.provider)
	}
	service, err := modelruntime.Ensure(ctx, modelruntime.Config{Probe: probe, Command: command, AutoStart: cfg.autoStart, StartupTimeout: cfg.startupTimeout})
	if err != nil {
		return nil, nil, locale.Errorf("preparing %s: %w\nUse -offline to play without an inference service.", cfg.provider, err)
	}
	return client, service, nil
}
