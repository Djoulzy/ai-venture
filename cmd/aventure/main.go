package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
	"ai-venture/internal/terminal"
	"ai-venture/internal/world"
	"ai-venture/scenarios"
)

func main() {
	if err := run(); err != nil {
		selected, _ := locale.Parse(flag.Lookup("lang").Value.String())
		fmt.Fprintln(os.Stderr, selected.Text("Error:"), selected.Error(err))
		os.Exit(1)
	}
}

func run() (runErr error) {
	langFlag := flag.String("lang", "en", "Game language / langue du jeu: en or fr")
	offline := flag.Bool("offline", false, "Play without an LLM using a limited local grammar")
	provider := flag.String("provider", "ollama", "Provider: ollama or llamacpp")
	model := flag.String("model", env("OLLAMA_MODEL", "lfm3:1.7b"), "Ollama model or llama-server alias")
	host := flag.String("ollama-url", env("OLLAMA_HOST", "http://localhost:11434"), "Ollama service URL")
	ollamaBinary := flag.String("ollama-bin", "ollama", "Path to the Ollama executable")
	llamaURL := flag.String("llama-url", "http://127.0.0.1:8080", "llama-server URL, without /v1")
	llamaBinary := flag.String("llama-bin", defaultLlamaBinary(), "Path to llama-server (local runtime detected automatically)")
	gguf := flag.String("gguf", "", "GGUF model used to start llama-server")
	llamaContext := flag.Int("llama-context", 4096, "Context size for the llama-server started by the game")
	autoStart := flag.Bool("auto-start", true, "Start the local service if it is absent")
	startupTimeout := flag.Duration("startup-timeout", 2*time.Minute, "Maximum time to wait for the service to become ready")
	scenarioPath := flag.String("scenario", "", "Scenario JSON (default: embedded lighthouse adventure)")
	savePath := flag.String("save", "", "Save file path (default: savegame-en.json or savegame-fr.json)")
	load := flag.Bool("load", false, "Load the save file at startup")
	noNarration := flag.Bool("no-narration", false, "Show only the engine facts")
	timeout := flag.Duration("timeout", 45*time.Second, "Timeout for each LLM call")
	flag.Parse()
	selected, err := locale.Parse(*langFlag)
	if err != nil {
		return err
	}
	if *savePath == "" {
		*savePath = "savegame-" + string(selected) + ".json"
	}
	if flag.NArg() != 0 {
		return locale.Errorf("unexpected arguments; use -help")
	}
	if *timeout <= 0 || *startupTimeout <= 0 || *savePath == "" {
		return locale.Errorf("positive timeout and save file path required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Closing stdin also unblocks the prompt on Ctrl+C; HTTP calls use ctx directly.
	go func() { <-ctx.Done(); _ = os.Stdin.Close() }()
	var source world.Source = scenarios.Demo{Language: selected}
	if *scenarioPath != "" {
		source = world.FileSource{Path: *scenarioPath}
	}
	w, err := source.Load(ctx)
	if err != nil {
		return locale.Errorf("scenario: %w", err)
	}
	e, err := engine.NewWithLanguage(w, selected)
	if err != nil {
		return err
	}
	if *load {
		if err := persistence.Load(*savePath, e); err != nil {
			return err
		}
	}
	var interpreter language.Interpreter = language.Offline{Language: selected}
	var narrator language.Narrator
	if !*offline {
		fmt.Println(selected.Text("Preparing %s (%s)...", *provider, *model))
		client, service, err := startBackend(ctx, backendConfig{
			provider: *provider, model: *model, ollamaURL: *host, ollamaBinary: *ollamaBinary,
			llamaURL: *llamaURL, llamaBinary: *llamaBinary, gguf: *gguf, llamaContext: *llamaContext,
			autoStart: *autoStart, timeout: *timeout, startupTimeout: *startupTimeout,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		defer func() { runErr = errors.Join(runErr, service.Close()) }()
		if service.Owned() {
			fmt.Println(selected.Text("Local service started; it will stop when the game closes."))
		} else {
			fmt.Println(selected.Text("Using an existing service; it will remain running after the game."))
		}
		llm := language.LLM{Client: client, Language: selected}
		interpreter = llm
		if !*noNarration {
			narrator = llm
		}
		fmt.Println(selected.Text("LLM mode: %s via %s", *model, *provider))
	} else {
		fmt.Println(selected.Text("Offline mode: simple commands (help)."))
	}
	fmt.Println(selected.Text("Language: English"))
	s := terminal.Session{Engine: e, Interpreter: interpreter, Narrator: narrator, Input: os.Stdin, Output: os.Stdout,
		SavePath: *savePath, Timeout: *timeout, Title: w.Title, Introduction: w.Introduction}
	err = s.Run(ctx)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
