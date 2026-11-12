// Package visuals composites imported images using engine-approved layers only.
package visuals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/locale"
)

const Width = 640
const Height = 384

type Renderer interface {
	Render(context.Context, engine.View) (string, error)
}

type Compositor struct {
	Assets    catalogue.AssetReader
	Directory string
}

func (c Compositor) Render(ctx context.Context, view engine.View) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(view.Visuals) == 0 {
		return "", nil
	}
	if c.Assets == nil || c.Directory == "" {
		return "", locale.Errorf("visual compositor is not configured")
	}
	if err := validateLayers(view.Visuals); err != nil {
		return "", err
	}
	ids := []string{}
	for _, layer := range view.Visuals {
		ids = append(ids, layer.Asset)
	}
	assets, err := c.Assets.ReadAssets(ctx, ids)
	if err != nil {
		return "", err
	}
	// Hash order, placement and actual image content, not only asset identifiers.
	key, _ := json.Marshal(struct {
		Layers []engine.VisibleVisual
		Assets map[string]string
	}{view.Visuals, hashes(assets)})
	hash := sha256.Sum256(key)
	name := "scene-" + hex.EncodeToString(hash[:]) + ".png"
	path, err := filepath.Abs(filepath.Join(c.Directory, name))
	if err != nil {
		return "", err
	}
	// Existing files are re-encoded atomically too: exported files are not authority.
	canvas, err := Compose(ctx, view.Visuals, assets)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".scene-*.png")
	if err != nil {
		return "", err
	}
	temp := file.Name()
	defer os.Remove(temp)
	defer file.Close()
	if err := png.Encode(file, canvas); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		return "", err
	}
	return path, nil
}

func hashes(assets map[string]catalogue.Asset) map[string]string {
	result := map[string]string{}
	for id, a := range assets {
		result[id] = a.SHA256
	}
	return result
}

func validateLayers(layers []engine.VisibleVisual) error {
	if len(layers) > 32 {
		return locale.Errorf("too many visual layers")
	}
	for _, l := range layers {
		if l.Asset == "" || l.X < 0 || l.Y < 0 || l.Width < 1 || l.Height < 1 || l.Width > 100 || l.Height > 100 || l.X > 100-l.Width || l.Y > 100-l.Height || l.Z < -1000 || l.Z > 1000 {
			return locale.Errorf("invalid visual layer: %s", l.Asset)
		}
	}
	return nil
}

func Compose(ctx context.Context, layers []engine.VisibleVisual, assets map[string]catalogue.Asset) (*image.NRGBA, error) {
	if err := validateLayers(layers); err != nil {
		return nil, err
	}
	ordered := append([]engine.VisibleVisual(nil), layers...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Z < ordered[j].Z })
	canvas := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	for _, layer := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		asset, ok := assets[layer.Asset]
		if !ok {
			return nil, locale.Errorf("unknown catalogue asset: %s", layer.Asset)
		}
		config, err := png.DecodeConfig(bytes.NewReader(asset.PNG))
		if err != nil {
			return nil, err
		}
		if err := catalogue.CheckImage(config); err != nil {
			return nil, err
		}
		source, err := png.Decode(bytes.NewReader(asset.PNG))
		if err != nil {
			return nil, err
		}
		rect := image.Rect(layer.X*Width/100, layer.Y*Height/100, (layer.X+layer.Width)*Width/100, (layer.Y+layer.Height)*Height/100)
		resized := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		bounds := source.Bounds()
		// Nearest-neighbour scaling keeps pixel-art edges and source transparency.
		for y := 0; y < resized.Bounds().Dy(); y++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for x := 0; x < resized.Bounds().Dx(); x++ {
				resized.Set(x, y, source.At(bounds.Min.X+x*bounds.Dx()/rect.Dx(), bounds.Min.Y+y*bounds.Dy()/rect.Dy()))
			}
		}
		draw.Draw(canvas, rect, resized, image.Point{}, draw.Over)
	}
	return canvas, ctx.Err()
}
