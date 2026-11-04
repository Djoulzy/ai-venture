package modelruntime

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"ai-venture/internal/inference/httpjson"
	"ai-venture/internal/locale"
)

// localAddress ensures that a locally spawned process serves the requested URL.
// Remote/proxied URLs remain usable with an already running service.
func localAddress(address string) (host, port string, err error) {
	u, err := httpjson.ParseURL(address)
	if err != nil {
		return "", "", err
	}
	host = u.Hostname()
	ip := net.ParseIP(host)
	if u.Scheme != "http" || (u.Path != "" && u.Path != "/") || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
		return "", "", locale.Errorf("automatic startup requires a loopback HTTP URL without a path prefix")
	}
	port = u.Port()
	if port == "" {
		port = "80"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", "", locale.Errorf("invalid local port")
	}
	return host, port, nil
}

func OllamaCommand(binary, address string) (*exec.Cmd, error) {
	host, port, err := localAddress(address)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "serve")
	// Replace an inherited value so the child binds exactly the probed address.
	cmd.Env = replaceEnv(os.Environ(), "OLLAMA_HOST", net.JoinHostPort(host, port))
	cmd.Env = replaceEnv(cmd.Env, "OLLAMA_KEEP_ALIVE", "-1")
	return cmd, nil
}

func LlamaCommand(binary, address, modelPath, alias string, contextSize int) (*exec.Cmd, error) {
	host, port, err := localAddress(address)
	if err != nil {
		return nil, err
	}
	if modelPath == "" {
		return nil, locale.Errorf("-gguf is required to start llama-server")
	}
	if strings.TrimSpace(alias) == "" || strings.Contains(alias, ",") || contextSize < 512 {
		return nil, locale.Errorf("invalid llama.cpp alias or context size")
	}
	path, err := filepath.Abs(modelPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, locale.Errorf("GGUF model: %w", err)
	}
	info, err := f.Stat()
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, locale.Errorf("GGUF model must be a nonempty file")
	}
	cmd := exec.Command(binary, "--model", path, "--alias", alias, "--host", host, "--port", port,
		"--ctx-size", strconv.Itoa(contextSize), "--parallel", "1", "--jinja", "--offline")
	// The owned server uses the explicit options above. Inherited llama settings
	// could enable a different model, TLS, authentication, or external tools.
	// Keep general CPU/GPU environment variables, but isolate llama's own config.
	cmd.Env = make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "LLAMA_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	return cmd, nil
}

func replaceEnv(env []string, key, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}
