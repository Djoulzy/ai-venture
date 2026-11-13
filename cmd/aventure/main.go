package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/desktop"
	"ai-venture/internal/engine"
	"ai-venture/internal/gameconfig"
	"ai-venture/internal/illustration"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
	"ai-venture/internal/presentation"
	ragstore "ai-venture/internal/retrieval/sqlite"
	"ai-venture/internal/terminal"
	"ai-venture/internal/visuals"
	"ai-venture/internal/world"
)

func main() {
	if err := run(); err != nil {
		selected, _ := locale.Parse(flag.Lookup("lang").Value.String())
		mode, _ := terminal.ParseColorMode(flag.Lookup("color").Value.String())
		terminal.NewPrinter(os.Stderr, mode).Println(terminal.StyleError, selected.Text("Error:")+" "+selected.Error(err))
		os.Exit(1)
	}
}

func run() (runErr error) {
	langFlag := flag.String("lang", "en", "Game language / langue du jeu: en or fr")
	generate := flag.Bool("generate", false, "Generate a validated scenario from -pitch and -catalogue, then exit")
	pitch := flag.String("pitch", "", "Adventure pitch for -generate (maximum 2000 characters / 4096 bytes)")
	scenarioOut := flag.String("scenario-out", "", "New JSON scenario destination; existing files are never replaced")
	seed := flag.Int64("seed", 0, "Generation variation identifier (0: random; not a guarantee of deterministic LLM output)")
	uiFlag := flag.String("ui", "window", "Interface: window or terminal (default: independent graphical window)")
	dataDir := flag.String("data-dir", "", "Writable game data directory (default: current folder, app bundle: user configuration folder)")
	colorFlag := flag.String("color", "auto", "ANSI colors: auto, always or never (auto respects NO_COLOR and TERM=dumb)")
	offline := flag.Bool("offline", false, "Play without an LLM using a limited local grammar")
	provider := flag.String("provider", "ollama", "Provider: ollama or llamacpp")
	model := flag.String("model", "", "Model or alias (default: OLLAMA_MODEL/lfm3:1.7b for Ollama, GGUF filename for llama.cpp)")
	host := flag.String("ollama-url", env("OLLAMA_HOST", "http://localhost:11434"), "Ollama service URL")
	ollamaBinary := flag.String("ollama-bin", "ollama", "Path to the Ollama executable")
	llamaURL := flag.String("llama-url", "http://127.0.0.1:8080", "llama-server URL, without /v1")
	llamaBinary := flag.String("llama-bin", defaultLlamaBinary(), "Path to llama-server (local runtime detected automatically)")
	gguf := flag.String("gguf", "", "GGUF model used to start llama-server")
	llamaContext := flag.Int("llama-context", 4096, "Context size for the llama-server started by the game")
	autoStart := flag.Bool("auto-start", true, "Start the local service if it is absent")
	startupTimeout := flag.Duration("startup-timeout", 2*time.Minute, "Maximum time to wait for the service to become ready")
	gamePath := flag.String("game", "", "External game configuration (default: game.json from app resources, working directory or executable directory)")
	scenarioPath := flag.String("scenario", "", "External scenario JSON; bypasses automatic game.json selection")
	savePath := flag.String("save", "", "Save path (default: savegame-<game or scenario ID>-<language>.json)")
	load := flag.Bool("load", false, "Load the save file at startup")
	noNarration := flag.Bool("no-narration", false, "Disable prose narration (location illustrations use -art)")
	art := flag.Bool("art", false, "Generate LLM location illustrations, at most 80 columns by 24 lines")
	cataloguePath := flag.String("catalogue", "", "Permanent catalogue used by scenario references and visual assets")
	visualsEnabled := flag.Bool("visuals", false, "Export engine-composed PNG scenes (works offline)")
	visualDirectory := flag.String("visual-dir", "", "PNG output directory (default: <save file>.visuals)")
	rag := flag.Bool("rag", true, "Enrich narration with local SQLite FTS5 retrieval")
	ragPath := flag.String("rag-db", "", "RAG cache path (default: <save file>.rag.sqlite)")
	timeout := flag.Duration("timeout", 45*time.Second, "Timeout for each LLM call")
	flag.Parse()
	selected, err := locale.Parse(*langFlag)
	if err != nil {
		return err
	}
	colorMode, err := terminal.ParseColorMode(*colorFlag)
	if err != nil {
		return err
	}
	if *uiFlag != "window" && *uiFlag != "terminal" {
		return locale.Errorf("unknown interface %q: use window or terminal", *uiFlag)
	}
	saveProvided := *savePath != ""
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
	if *generate {
		if *gamePath != "" {
			return locale.Errorf("use -catalogue, not -game, for autonomous generation")
		}
		if *offline || *load || *scenarioPath != "" || *cataloguePath == "" || *scenarioOut == "" || *pitch == "" {
			return locale.Errorf("generation requires -pitch, -catalogue and -scenario-out, without -offline, -load or -scenario")
		}
		if _, err := os.Lstat(*scenarioOut); err == nil {
			return locale.Errorf("scenario output already exists: %s", *scenarioOut)
		} else if !os.IsNotExist(err) {
			return err
		}
		explicitContext := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "llama-context" {
				explicitContext = true
			}
		})
		if !explicitContext {
			*llamaContext = 8192
		}
		*model, err = resolveModel(*provider, *model, *gguf)
		if err != nil {
			return err
		}
		return generateScenario(ctx, selected, *pitch, *scenarioOut, *cataloguePath, *seed, backendConfig{
			provider: *provider, model: *model, ollamaURL: *host, ollamaBinary: *ollamaBinary,
			llamaURL: *llamaURL, llamaBinary: *llamaBinary, gguf: *gguf, llamaContext: *llamaContext,
			autoStart: *autoStart, timeout: *timeout, startupTimeout: *startupTimeout,
		}, terminal.NewPrinter(os.Stdout, colorMode))
	}
	if *pitch != "" || *scenarioOut != "" || *seed != 0 {
		return locale.Errorf("pitch, scenario-out and seed require -generate")
	}
	executable, _ := os.Executable()
	resources := bundleResources(executable)
	if *gamePath != "" && *scenarioPath != "" {
		return locale.Errorf("choose either -game or -scenario")
	}
	if *gamePath == "" && *scenarioPath == "" {
		directory, err := os.Getwd()
		if err != nil {
			return err
		}
		*gamePath = defaultGameConfig(executable, directory)
	}
	if resources != "" && *dataDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*dataDir = filepath.Join(configDir, "ai-venture")
	}
	play := func(ctx context.Context, input io.Reader, writer io.Writer, observe presentation.Observer, forceOffline bool) (runErr error) {
		output := terminal.NewPrinter(writer, colorMode)
		announce := func(style terminal.Style, message string) {
			output.Println(style, message)
			if observe != nil {
				observe(presentation.Event{Kind: presentation.Message, Tone: terminal.Tone(style), Text: presentation.Clean(message)})
			}
		}
		useOffline := *offline || forceOffline || (resources != "" && len(os.Args) == 1)
		if *dataDir != "" {
			if err := os.MkdirAll(*dataDir, 0700); err != nil {
				return err
			}
		}
		path, database, saveID := *scenarioPath, *cataloguePath, ""
		if *gamePath != "" {
			config, err := gameconfig.Load(ctx, *gamePath)
			if err != nil {
				return err
			}
			path, err = config.Scenario(selected)
			if err != nil {
				return err
			}
			saveID = config.ID
			if database == "" {
				directory := *dataDir
				if directory == "" {
					directory = "."
				}
				database, err = config.PrepareCatalogue(ctx, directory)
				if err != nil {
					return err
				}
			}
		}
		if path == "" {
			return locale.Errorf("no game selected: provide -game or -scenario, or a game.json configuration")
		}
		w, err := (world.FileSource{Path: path}).Load(ctx)
		if err != nil {
			return locale.Errorf("scenario: %w", err)
		}
		if !saveProvided {
			if saveID == "" {
				saveID = w.ID
			}
			*savePath = filepath.Join(*dataDir, "savegame-"+saveID+"-"+string(selected)+".json")
		}
		var content *catalogue.Store
		if database != "" {
			openCtx, openCancel := context.WithTimeout(ctx, 5*time.Second)
			content, err = catalogue.Open(openCtx, database, false)
			if err == nil {
				w, err = content.Bind(openCtx, w, selected)
			}
			openCancel()
			if err != nil {
				if content != nil {
					_ = content.Close()
				}
				return err
			}
			defer func() { runErr = errors.Join(runErr, content.Close()) }()
		} else if len(w.CatalogueDocuments) > 0 || *visualsEnabled {
			return locale.Errorf("this scenario or -visuals requires -catalogue")
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
		var illustrator illustration.Illustrator
		if !useOffline {
			*model, err = resolveModel(*provider, *model, *gguf)
			if err != nil {
				return err
			}
			announce(terminal.StyleInfo, selected.Text("Preparing %s (%s)...", *provider, *model))
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
				announce(terminal.StyleInfo, selected.Text("Local service started; it will stop when the game closes."))
			} else {
				announce(terminal.StyleInfo, selected.Text("Using an existing service; it will remain running after the game."))
			}
			if *provider == "llamacpp" && *gguf != "" {
				announce(terminal.StyleInfo, selected.Text("Verified GGUF: %s", *gguf))
			}
			llm := language.LLM{Client: client, Language: selected}
			interpreter = llm
			if *art {
				illustrator = illustration.LLM{Client: client, Language: selected}
				announce(terminal.StyleInfo, selected.Text("Location illustrations enabled (maximum 80 x 24)."))
			}
			if !*noNarration {
				narrator = llm
				if *rag {
					if *ragPath == "" {
						*ragPath = *savePath + ".rag.sqlite"
					}
					openCtx, openCancel := context.WithTimeout(ctx, 2*time.Second)
					store, err := ragstore.Open(openCtx, *ragPath)
					openCancel()
					if err != nil {
						announce(terminal.StyleWarning, selected.Text("RAG unavailable; continuing with engine facts: %s", selected.Error(err)))
					} else {
						defer func() { runErr = errors.Join(runErr, store.Close()) }()
						narrator = language.RAGNarrator{LLM: llm, Engine: e, Search: store, OnError: func(err error) {
							announce(terminal.StyleWarning, selected.Text("RAG unavailable; continuing with engine facts: %s", selected.Error(err)))
						}}
						announce(terminal.StyleInfo, selected.Text("RAG enabled: %s", *ragPath))
					}
				}
			}
			announce(terminal.StyleInfo, selected.Text("LLM mode: %s via %s", *model, *provider))
		} else {
			announce(terminal.StyleInfo, selected.Text("Offline mode: simple commands (help)."))
		}
		announce(terminal.StyleInfo, selected.Text("Language: English"))
		var sceneRenderer visuals.Renderer
		if *visualsEnabled || (*uiFlag == "window" && content != nil && len(w.Visuals) > 0) {
			if *visualDirectory == "" {
				*visualDirectory = *savePath + ".visuals"
			}
			sceneRenderer = visuals.Compositor{Assets: content, Directory: *visualDirectory}
			announce(terminal.StyleInfo, selected.Text("PNG scene export enabled: %s", *visualDirectory))
		}
		s := terminal.Session{Engine: e, Interpreter: interpreter, Narrator: narrator, Input: input, Output: writer, Observer: observe,
			Illustrator: illustrator, Visuals: sceneRenderer, Color: output.Color, SavePath: *savePath, Timeout: *timeout, Title: w.Title, Introduction: w.Introduction}
		err = s.Run(ctx)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	if *uiFlag == "terminal" {
		// Only the terminal frontend needs stdin closed to interrupt a read.
		go func() { <-ctx.Done(); _ = os.Stdin.Close() }()
		return play(ctx, os.Stdin, os.Stdout, nil, false)
	}
	return desktop.Run(ctx, desktop.Config{Language: selected, Start: func(ctx context.Context, input io.Reader, observe presentation.Observer, forceOffline bool) error {
		return play(ctx, input, io.Discard, observe, forceOffline)
	}})
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
