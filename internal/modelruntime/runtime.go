// Package modelruntime supervises inference processes owned by the game.
// It never stops a service that was already running.
package modelruntime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"ai-venture/internal/locale"
)

type Config struct {
	Probe func(context.Context) (bool, error)
	// Command is evaluated only when the endpoint refuses the connection.
	Command         func() (*exec.Cmd, error)
	AutoStart       bool
	StartupTimeout  time.Duration
	ShutdownTimeout time.Duration
	PollInterval    time.Duration
}

type Service struct {
	cmd             *exec.Cmd
	done            chan struct{}
	waitErr         error // published by closing done
	logs            tailBuffer
	shutdownTimeout time.Duration
	closeOnce       sync.Once
	closeErr        error
}

// Ensure reuses an existing service, waits for one loading, or starts one.
// A model/configuration/protocol error never triggers a second server.
func Ensure(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Probe == nil || cfg.StartupTimeout <= 0 {
		return nil, locale.Errorf("incomplete service configuration")
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 3 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	startupCtx, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	probe := func() (bool, error) {
		probeCtx, cancel := context.WithTimeout(startupCtx, 2*time.Second)
		defer cancel()
		if err := probeCtx.Err(); err != nil {
			return false, err
		}
		return cfg.Probe(probeCtx)
	}
	ready, err := probe()
	s := &Service{shutdownTimeout: cfg.ShutdownTimeout}
	if ready && err == nil {
		return s, nil
	}
	if err != nil {
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, locale.Errorf("service check: %w", err)
		}
		if !cfg.AutoStart {
			return nil, locale.Errorf("service unavailable and automatic startup disabled: %w", err)
		}
		if cfg.Command == nil {
			return nil, locale.Errorf("no startup command configured")
		}
		cmd, err := cfg.Command()
		if err != nil {
			return nil, err
		}
		if cmd == nil {
			return nil, locale.Errorf("empty startup command")
		}
		if err := startupCtx.Err(); err != nil {
			return nil, err
		}
		prepareProcess(cmd)
		cmd.Stdout = &s.logs
		cmd.Stderr = &s.logs
		// Bound Wait even when a descendant inherits the output pipes.
		cmd.WaitDelay = 500 * time.Millisecond
		if err := cmd.Start(); err != nil {
			return nil, locale.Errorf("starting service: %w", err)
		}
		s.cmd = cmd
		s.done = make(chan struct{})
		go func() {
			s.waitErr = cmd.Wait()
			// Reap our process group if its leader exits before its workers.
			cleanupProcess(cmd)
			close(s.done)
		}()
	}
	success := false
	defer func() {
		if !success {
			_ = s.Close()
		}
	}()
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-startupCtx.Done():
			return nil, s.diagnostic(locale.Errorf("startup timeout or cancellation: %w", startupCtx.Err()))
		case <-s.done:
			return nil, s.exited()
		case <-ticker.C:
		}
		ready, err := probe()
		select {
		case <-s.done:
			return nil, s.exited()
		default:
		}
		if err != nil && !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, s.diagnostic(locale.Errorf("service check: %w", err))
		}
		if ready && err == nil {
			if err := startupCtx.Err(); err != nil {
				return nil, err
			}
			success = true
			if s.Owned() {
				go func() {
					select {
					case <-ctx.Done():
						_ = s.Close()
					case <-s.done:
					}
				}()
			}
			return s, nil
		}
	}
}

func (s *Service) Owned() bool { return s.cmd != nil }

func (s *Service) exited() error {
	if s.waitErr != nil {
		return s.diagnostic(locale.Errorf("service exited during startup: %w", s.waitErr))
	}
	return s.diagnostic(locale.Errorf("service exited before becoming ready"))
}

func (s *Service) diagnostic(err error) error {
	if logs := s.logs.String(); logs != "" {
		return locale.Errorf("%w\nRecent service messages:\n%s", err, logs)
	}
	return err
}

// Close is idempotent, bounded and safe to call concurrently with cancellation.
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		if !s.Owned() {
			return
		}
		select {
		case <-s.done:
			return
		default:
		}
		if err := stopProcess(s.cmd); err != nil && !processGone(err) {
			s.closeErr = err
		}
		timer := time.NewTimer(s.shutdownTimeout)
		defer timer.Stop()
		select {
		case <-s.done:
			return
		case <-timer.C:
			if err := killProcess(s.cmd); err != nil && !processGone(err) {
				s.closeErr = err
			}
		}
		// Kill normally reaps immediately; retain a bound if the OS refuses it.
		select {
		case <-s.done:
		case <-time.After(time.Second):
			s.closeErr = locale.Errorf("inference process did not confirm its shutdown")
		}
	})
	return s.closeErr
}

func processGone(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}

const maxLogBytes = 8192

type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) >= maxLogBytes {
		b.data = append(b.data[:0], p[len(p)-maxLogBytes:]...)
	} else {
		b.data = append(b.data, p...)
		if len(b.data) > maxLogBytes {
			b.data = append(b.data[:0], b.data[len(b.data)-maxLogBytes:]...)
		}
	}
	return n, nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, string(b.data)))
}
