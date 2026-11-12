package visuals_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/visuals"
)

func asset(t *testing.T, id string, c color.NRGBA) catalogue.Asset {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			img.SetNRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return catalogue.Asset{ID: id, SHA256: id, Width: 2, Height: 2, PNG: data.Bytes()}
}

type assets map[string]catalogue.Asset

func (a assets) ReadAssets(_ context.Context, ids []string) (map[string]catalogue.Asset, error) {
	result := map[string]catalogue.Asset{}
	for _, id := range ids {
		if value, ok := a[id]; ok {
			result[id] = value
		}
	}
	return result, nil
}

func TestAlphaPositionAndStableZOrder(t *testing.T) {
	input := assets{"red": asset(t, "red", color.NRGBA{R: 255, A: 255}), "blue": asset(t, "blue", color.NRGBA{B: 255, A: 128})}
	layers := []engine.VisibleVisual{{Asset: "blue", X: 25, Y: 25, Width: 50, Height: 50, Z: 10}, {Asset: "red", Width: 100, Height: 100, Z: 0}}
	canvas, err := visuals.Compose(context.Background(), layers, input)
	if err != nil {
		t.Fatal(err)
	}
	outside := canvas.NRGBAAt(10, 10)
	inside := canvas.NRGBAAt(320, 192)
	if outside != (color.NRGBA{R: 255, A: 255}) || inside.R < 125 || inside.R > 128 || inside.B < 127 || inside.B > 129 || inside.A != 255 {
		t.Fatalf("outside=%+v, inside=%+v", outside, inside)
	}
	if layers[0].Asset != "blue" {
		t.Fatal("sort mutated engine view")
	}
	layers[0].Z = 0
	canvas, err = visuals.Compose(context.Background(), layers, input)
	if err != nil {
		t.Fatal(err)
	}
	if canvas.NRGBAAt(320, 192) != (color.NRGBA{R: 255, A: 255}) {
		t.Fatal("equal-Z order is not stable")
	}
}

func TestPNGExportIsAtomicBoundedAndReconstructed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "images")
	input := assets{"red": asset(t, "red", color.NRGBA{R: 255, A: 255})}
	c := visuals.Compositor{Assets: input, Directory: dir}
	view := engine.View{Visuals: []engine.VisibleVisual{{Asset: "red", Width: 100, Height: 100}}}
	path, err := c.Render(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatal("export escaped output directory")
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(first))
	if err != nil || config.Width != 640 || config.Height != 384 {
		t.Fatalf("%+v %v", config, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("export file is not private")
	}
	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := c.Render(context.Background(), view)
	if err != nil || again != path {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(again)
	if err != nil || !bytes.Equal(restored, first) {
		t.Fatal("exported file treated as authority")
	}
	view.Visuals[0].Width = 50
	other, err := c.Render(context.Background(), view)
	if err != nil || other == path {
		t.Fatal("placement was not part of scene signature")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Render(ctx, view); err == nil {
		t.Fatal("cancelled render succeeded")
	}
	view.Visuals[0].Width = 101
	if _, err := c.Render(context.Background(), view); err == nil {
		t.Fatal("invalid layer exported")
	}
	view.Visuals[0] = engine.VisibleVisual{Asset: "missing", Width: 10, Height: 10}
	if _, err := c.Render(context.Background(), view); err == nil {
		t.Fatal("missing asset accepted")
	}
	temporary, err := filepath.Glob(filepath.Join(dir, ".scene-*.png"))
	if err != nil || len(temporary) != 0 {
		t.Fatal("temporary export leaked")
	}
	if path, err := c.Render(context.Background(), engine.View{}); err != nil || path != "" {
		t.Fatal("empty scene created an image")
	}
}
