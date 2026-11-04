package modelruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The Go test executable doubles as a controllable process, without Ollama,
// shells, network access, or a downloaded model.
func TestRuntimeChild(t *testing.T) {
	mode := os.Getenv("AI_VENTURE_RUNTIME_CHILD")
	if mode == "" {
		return
	}
	readyPath := os.Getenv("AI_VENTURE_CHILD_READY")
	stopPath := os.Getenv("AI_VENTURE_CHILD_STOP")
	if mode == "exit" {
		fmt.Fprintln(os.Stderr, "GGUF invalide\x1b[2J")
		os.Exit(7)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	if mode == "worker" {
		for {
			_ = os.WriteFile(readyPath, []byte(time.Now().String()), 0600)
			time.Sleep(10 * time.Millisecond)
		}
	}
	if mode == "fork" {
		child := exec.Command(os.Args[0], "-test.run=^TestRuntimeChild$")
		child.Env = replaceEnv(os.Environ(), "AI_VENTURE_RUNTIME_CHILD", "worker")
		child.Env = replaceEnv(child.Env, "AI_VENTURE_CHILD_READY", readyPath+".worker")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(8)
		}
		for {
			if _, err := os.Stat(readyPath + ".worker"); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	_ = os.WriteFile(readyPath, []byte("ready"), 0600)
	if mode == "ignore" {
		select {}
	}
	<-signals
	_ = os.WriteFile(stopPath, []byte("stopped"), 0600)
	os.Exit(0)
}

func childConfig(t *testing.T, mode string) (Config, *exec.Cmd, string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "ready")
	cmd := exec.Command(binary, "-test.run=^TestRuntimeChild$")
	cmd.Env = append(os.Environ(), "AI_VENTURE_RUNTIME_CHILD="+mode, "AI_VENTURE_CHILD_READY="+readyPath, "AI_VENTURE_CHILD_STOP="+readyPath+".stop")
	started := false
	cfg := Config{
		Probe: func(context.Context) (bool, error) {
			if !started {
				return false, syscall.ECONNREFUSED
			}
			_, err := os.Stat(readyPath)
			return err == nil, nil
		},
		Command:   func() (*exec.Cmd, error) { started = true; return cmd, nil },
		AutoStart: true, StartupTimeout: 5 * time.Second, ShutdownTimeout: 100 * time.Millisecond, PollInterval: 10 * time.Millisecond,
	}
	return cfg, cmd, readyPath
}

func TestOwnProcessStartsAndStops(t *testing.T) {
	cfg, cmd, path := childConfig(t, "wait")
	s, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if !s.Owned() {
		t.Fatal("processus lancé non déclaré possédé")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("processus non attendu")
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if _, err := os.Stat(path + ".stop"); err != nil {
			t.Fatal("arrêt gracieux absent")
		}
	}
}

func TestExistingServiceIsNeverOwned(t *testing.T) {
	calls := 0
	cfg := Config{Probe: func(context.Context) (bool, error) { calls++; return calls >= 3, nil }, AutoStart: true, StartupTimeout: time.Second, PollInterval: time.Millisecond,
		Command: func() (*exec.Cmd, error) { t.Fatal("un service en chargement a été doublonné"); return nil, nil }}
	s, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Owned() {
		t.Fatal("service externe considéré possédé")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNoLaunchOnConfigurationErrorOrDisabledStart(t *testing.T) {
	for _, probeErr := range []error{errors.New("modèle absent"), syscall.ECONNREFUSED} {
		_, err := Ensure(context.Background(), Config{
			Probe:          func(context.Context) (bool, error) { return false, probeErr },
			Command:        func() (*exec.Cmd, error) { t.Fatal("lancement inattendu"); return nil, nil },
			StartupTimeout: time.Second,
		})
		if err == nil {
			t.Fatal("erreur ignorée")
		}
	}
	_, err := Ensure(context.Background(), Config{Probe: func(context.Context) (bool, error) { return false, errors.New("mauvais service") }, AutoStart: true, StartupTimeout: time.Second,
		Command: func() (*exec.Cmd, error) { t.Fatal("lancement malgré un service incompatible"); return nil, nil }})
	if err == nil {
		t.Fatal("mauvais service accepté")
	}
}

func TestStartupTimeoutCleansUp(t *testing.T) {
	cfg, cmd, _ := childConfig(t, "wait")
	first := true
	cfg.Probe = func(context.Context) (bool, error) {
		if first {
			first = false
			return false, syscall.ECONNREFUSED
		}
		return false, nil
	}
	cfg.StartupTimeout = 150 * time.Millisecond
	if _, err := Ensure(context.Background(), cfg); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("délai ignoré : %v", err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("processus non nettoyé après le délai")
	}
}

func TestStartupExitIncludesDiagnostics(t *testing.T) {
	cfg, cmd, _ := childConfig(t, "exit")
	_, err := Ensure(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "GGUF invalide") || strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("diagnostic incorrect : %v", err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("processus non attendu")
	}
}

func TestModelErrorAfterLaunchCleansUp(t *testing.T) {
	cfg, cmd, _ := childConfig(t, "wait")
	probe := cfg.Probe
	cfg.Probe = func(ctx context.Context) (bool, error) {
		ready, err := probe(ctx)
		if ready {
			return false, errors.New("modèle absent")
		}
		return ready, err
	}
	if _, err := Ensure(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "modèle absent") {
		t.Fatalf("%v", err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("service abandonné après une erreur de modèle")
	}
}

func TestMissingExecutable(t *testing.T) {
	_, err := Ensure(context.Background(), Config{
		Probe:     func(context.Context) (bool, error) { return false, syscall.ECONNREFUSED },
		Command:   func() (*exec.Cmd, error) { return exec.Command(filepath.Join(t.TempDir(), "absent")), nil },
		AutoStart: true, StartupTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "starting service") {
		t.Fatalf("exécutable absent accepté : %v", err)
	}
}

func TestCancellationStopsOwnedProcess(t *testing.T) {
	cfg, cmd, _ := childConfig(t, "wait")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Ensure(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	cancel()
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("annulation sans arrêt du service")
	}
	if cmd.ProcessState == nil {
		t.Fatal("processus non attendu")
	}
}

func TestKillFallbackAndDescendantCleanup(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("groupes de processus Unix")
	}
	for _, mode := range []string{"ignore", "fork"} {
		t.Run(mode, func(t *testing.T) {
			cfg, cmd, path := childConfig(t, mode)
			s, err := Ensure(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if cmd.ProcessState == nil {
				t.Fatal("processus non attendu")
			}
			if mode == "fork" {
				before, err := os.ReadFile(path + ".worker")
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(80 * time.Millisecond)
				after, err := os.ReadFile(path + ".worker")
				if err != nil || string(before) != string(after) {
					t.Fatal("un descendant a survécu à la fermeture")
				}
			}
		})
	}
}

func TestCancelledStartupDoesNotLaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Ensure(ctx, Config{Probe: func(context.Context) (bool, error) { t.Fatal("sonde appelée malgré l'annulation"); return false, nil }, StartupTimeout: time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("annulation perdue : %v", err)
	}
}

func TestCommandConfiguration(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://0.0.0.0:9999")
	cmd, err := OllamaCommand("/test/ollama", "http://127.0.0.1:11499")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cmd.Args, " ") != "/test/ollama serve" {
		t.Fatal("commande Ollama incorrecte")
	}
	count := 0
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "OLLAMA_HOST=") {
			count++
			if entry != "OLLAMA_HOST=127.0.0.1:11499" {
				t.Fatal(entry)
			}
		}
	}
	if count != 1 {
		t.Fatal("OLLAMA_HOST non remplacé")
	}
	if !strings.Contains(strings.Join(cmd.Env, "\n"), "OLLAMA_KEEP_ALIVE=-1") {
		t.Fatal("modèle non conservé pendant la session gérée")
	}
	t.Setenv("LLAMA_ARG_API_KEY_FILE", "/une/config/externe")
	path := filepath.Join(t.TempDir(), "mon modele.gguf")
	if err := os.WriteFile(path, []byte("GGUF"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd, err = LlamaCommand("llama-server", "http://127.0.0.1:8081", path, "lfm3:1.7b", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Args[2] != path || !strings.Contains(strings.Join(cmd.Args, " "), "--port 8081") {
		t.Fatalf("arguments GGUF incorrects : %v", cmd.Args)
	}
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "LLAMA_") {
			t.Fatal("configuration llama externe héritée")
		}
	}
	for _, address := range []string{"http://example.com:8080", "http://0.0.0.0:8080", "https://localhost:8080", "http://localhost:8080/proxy", "http://localhost:0"} {
		if _, err := OllamaCommand("ollama", address); err == nil {
			t.Fatalf("adresse de lancement acceptée : %s", address)
		}
	}
	if _, err := LlamaCommand("llama-server", "http://localhost:8080", "", "model", 4096); err == nil {
		t.Fatal("GGUF manquant accepté")
	}
}

func TestLogTailIsBounded(t *testing.T) {
	var buffer tailBuffer
	_, _ = buffer.Write([]byte(strings.Repeat("x", maxLogBytes*3)))
	_, _ = buffer.Write([]byte("fin"))
	if len(buffer.String()) != maxLogBytes || !strings.HasSuffix(buffer.String(), "fin") {
		t.Fatal("journal non borné")
	}
}
