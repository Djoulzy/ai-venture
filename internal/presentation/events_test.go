package presentation_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"ai-venture/internal/presentation"
)

func TestReaderPreservesFrenchCommandsAcrossSmallReads(t *testing.T) {
	commands := make(chan string, 2)
	commands <- "observer la clé"
	commands <- "/quitter"
	close(commands)
	r := &presentation.Reader{Context: context.Background(), Commands: commands}
	var output strings.Builder
	buffer := make([]byte, 3)
	for {
		n, err := r.Read(buffer)
		output.Write(buffer[:n])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := output.String(); got != "observer la clé\n/quitter\n" {
		t.Fatalf("corrupted command stream: %q", got)
	}
}

func TestReaderCancellationUnblocksIdleWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &presentation.Reader{Context: ctx, Commands: make(chan string)}
	done := make(chan error, 1)
	go func() { _, err := bufio.NewReader(r).ReadString('\n'); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("closing an idle window left the session blocked on input")
	}
}

func TestCleanPreservesAccentsWithoutControls(t *testing.T) {
	if got := presentation.Clean("Clé\x00\x1b\r\u202e du phare\n\tété"); got != "Clé du phare\n\tété" {
		t.Fatalf("unexpected display text: %q", got)
	}
}
