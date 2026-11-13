// Package desktop displays the existing sequential session in a native window.
package desktop

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"io"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"ai-venture/internal/illustration"
	"ai-venture/internal/locale"
	"ai-venture/internal/presentation"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

type Config struct {
	Language locale.Language
	// A fresh sequential session owns all engine/LLM/SQLite work. forceOffline is
	// used only for an explicit recovery click after startup has failed.
	Start func(context.Context, io.Reader, presentation.Observer, bool) error
}

type button struct {
	bounds  image.Rectangle
	label   string
	enabled bool
	action  func()
}
type line struct {
	text string
	tone presentation.Tone
}
type window struct {
	config              Config
	parent              context.Context
	cancel              context.CancelFunc
	done                <-chan error
	events              <-chan presentation.Event
	commands            chan<- string
	completed           bool
	result              error
	fatal               bool
	model               model
	input               editBuffer
	history             []string
	historyAt           int
	inventory           bool
	selectedTool        string
	itemScroll          int
	logScroll           int
	logVersion          uint64
	logWidth            int
	logLines            []line
	buttons             []button
	inputBounds         image.Rectangle
	logBounds           image.Rectangle
	itemsBounds         image.Rectangle
	scene               *ebiten.Image
	regular, bold, mono *text.GoTextFaceSource
	tick                int
	w, h                int
}

var (
	background = color.NRGBA{R: 12, G: 20, B: 30, A: 255}
	panel      = color.NRGBA{R: 21, G: 33, B: 46, A: 255}
	border     = color.NRGBA{R: 44, G: 63, B: 77, A: 255}
	ink        = color.NRGBA{R: 232, G: 237, B: 233, A: 255}
	muted      = color.NRGBA{R: 147, G: 166, B: 180, A: 255}
	accent     = color.NRGBA{R: 221, G: 180, B: 108, A: 255}
	green      = color.NRGBA{R: 128, G: 190, B: 157, A: 255}
	red        = color.NRGBA{R: 239, G: 133, B: 123, A: 255}
	blue       = color.NRGBA{R: 132, G: 185, B: 200, A: 255}
)

func Run(ctx context.Context, config Config) error {
	if config.Start == nil {
		return locale.Errorf("window session is not configured")
	}
	g := &window{config: config, parent: ctx, w: 1280, h: 900}
	var err error
	for _, font := range []struct {
		data   []byte
		target **text.GoTextFaceSource
	}{{goregular.TTF, &g.regular}, {gobold.TTF, &g.bold}, {gomono.TTF, &g.mono}} {
		*font.target, err = text.NewGoTextFaceSource(bytes.NewReader(font.data))
		if err != nil {
			return err
		}
	}
	g.start(false)
	ebiten.SetWindowTitle("AI Venture")
	ebiten.SetWindowSize(1280, 900)
	ebiten.SetWindowSizeLimits(1040, 760, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)
	ebiten.SetRunnableOnUnfocused(true)
	err = ebiten.RunGame(g)
	g.cancel()
	if !g.completed {
		select {
		case result := <-g.done:
			g.result = result
		case <-time.After(7 * time.Second):
			return errors.Join(err, locale.Errorf("game session did not finish closing"))
		}
	}
	if errors.Is(g.result, context.Canceled) {
		g.result = nil
	}
	return errors.Join(err, g.result)
}

func (g *window) start(offline bool) {
	if g.cancel != nil {
		g.cancel()
	}
	ctx, cancel := context.WithCancel(g.parent)
	g.cancel = cancel
	events := make(chan presentation.Event, 256)
	commands := make(chan string, 1)
	done := make(chan error, 1)
	g.events, g.commands, g.done = events, commands, done
	g.completed = false
	g.result = nil
	g.fatal = false
	g.model = model{status: g.label("Preparing the adventure…")}
	if g.scene != nil {
		g.scene.Dispose()
	}
	g.scene = nil
	g.logLines = nil
	g.logVersion = 0
	g.logWidth = 0
	g.logScroll = 0
	g.selectedTool = ""
	g.input.set("")
	go func() {
		observe := func(event presentation.Event) {
			select {
			case events <- event:
			case <-ctx.Done():
			}
		}
		err := g.config.Start(ctx, &presentation.Reader{Context: ctx, Commands: commands}, observe, offline)
		done <- err
	}()
}

func (g *window) label(key string, args ...any) string { return g.config.Language.Text(key, args...) }
func (g *window) local(en, fr string) string {
	if g.config.Language == locale.French {
		return "/" + fr
	}
	return "/" + en
}

func (g *window) submit(command string) {
	command = strings.TrimSpace(command)
	if !g.model.ready || g.fatal || command == "" {
		return
	}
	select {
	case g.commands <- command:
		g.model.ready = false
		g.model.status = g.label("Working…")
		g.model.append(presentation.Command, command)
		g.history = append(g.history, command)
		if len(g.history) > 64 {
			g.history = append([]string(nil), g.history[len(g.history)-64:]...)
		}
		g.historyAt = len(g.history)
		g.input.set("")
		g.selectedTool = ""
		g.logScroll = 0
	default:
	}
}

func (g *window) Update() error {
	g.tick++
	if g.parent.Err() != nil || ebiten.IsWindowBeingClosed() {
		g.cancel()
		return ebiten.Termination
	}
	for {
		select {
		case event := <-g.events:
			if event.Kind == presentation.View {
				if g.scene != nil {
					g.scene.Dispose()
				}
				g.scene = nil
			}
			if event.Kind == presentation.Scene && event.Image != nil {
				if g.scene != nil {
					g.scene.Dispose()
				}
				g.scene = ebiten.NewImageFromImage(event.Image)
			}
			g.model.apply(event)
			if event.Kind == presentation.Title {
				ebiten.SetWindowTitle("AI Venture — " + g.model.title)
			}
			if event.Kind == presentation.Ready {
				g.model.status = g.label("Ready.")
			}
		default:
			goto received
		}
	}
received:
	if !g.completed {
		select {
		case result := <-g.done:
			g.completed = true
			g.result = result
			if result == nil || errors.Is(result, context.Canceled) {
				return ebiten.Termination
			}
			g.fatal = true
			g.model.ready = false
			g.model.status = g.label("The adventure could not start.")
			g.model.append(presentation.Error, g.config.Language.Error(result))
		default:
		}
	}
	x, y := ebiten.CursorPosition()
	pointer := image.Pt(x, y)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		for _, b := range g.buttons {
			if b.enabled && pointer.In(b.bounds) {
				b.action()
				break
			}
		}
	}
	_, wheel := ebiten.Wheel()
	if pointer.In(g.logBounds) {
		g.logScroll = max(0, g.logScroll+int(wheel*3))
	}
	if pointer.In(g.itemsBounds) {
		g.itemScroll = max(0, g.itemScroll-int(wheel*2))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		g.logScroll += 10
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		g.logScroll = max(0, g.logScroll-10)
	}
	if g.fatal {
		return nil
	}
	control := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	if control && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		g.paste()
	}
	if control && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		g.input.all = len(g.input.text) > 0
	}
	if !control {
		g.input.insert(string(ebiten.AppendInputChars(nil)))
	}
	for _, key := range []ebiten.Key{ebiten.KeyBackspace, ebiten.KeyDelete, ebiten.KeyLeft, ebiten.KeyRight} {
		duration := inpututil.KeyPressDuration(key)
		if duration != 1 && !(duration > 25 && duration%3 == 0) {
			continue
		}
		switch key {
		case ebiten.KeyBackspace:
			g.input.backspace()
		case ebiten.KeyDelete:
			g.input.delete()
		case ebiten.KeyLeft:
			if g.input.all {
				g.input.cursor = 0
			} else {
				g.input.cursor = max(0, g.input.cursor-1)
			}
			g.input.all = false
		case ebiten.KeyRight:
			if g.input.all {
				g.input.cursor = len(g.input.text)
			} else {
				g.input.cursor = min(len(g.input.text), g.input.cursor+1)
			}
			g.input.all = false
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		g.input.cursor = 0
		g.input.all = false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		g.input.cursor = len(g.input.text)
		g.input.all = false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.input.set("")
		g.selectedTool = ""
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) && len(g.history) > 0 {
		g.historyAt = max(0, g.historyAt-1)
		g.input.set(g.history[g.historyAt])
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) && len(g.history) > 0 {
		g.historyAt = min(len(g.history), g.historyAt+1)
		if g.historyAt == len(g.history) {
			g.input.set("")
		} else {
			g.input.set(g.history[g.historyAt])
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		g.submit(g.input.String())
	}
	return nil
}

// Clipboard utilities are fixed OS commands, invoked only on an explicit paste
// shortcut. Clipboard text is never executed, and still requires submission.
func (g *window) paste() {
	var cmd *exec.Cmd
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "/usr/bin/pbpaste")
	case "linux":
		if tool, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.CommandContext(ctx, tool, "--no-newline")
		} else if tool, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, tool, "-selection", "clipboard", "-o")
		}
	}
	if cmd == nil {
		g.model.status = g.label("Paste is unavailable; type your command.")
		return
	}
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	data, _ := io.ReadAll(io.LimitReader(reader, 4097))
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	g.input.insert(string(data))
}

func (g *window) Layout(w, h int) (int, int) { g.w = max(w, 1040); g.h = max(h, 760); return g.w, g.h }

func (g *window) face(size float64, bold bool) *text.GoTextFace {
	source := g.regular
	if bold {
		source = g.bold
	}
	return &text.GoTextFace{Source: source, Size: size}
}
func (g *window) text(dst *ebiten.Image, value string, x, y float64, size float64, c color.Color, bold bool) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, presentation.Clean(value), g.face(size, bold), op)
}
func rect(dst *ebiten.Image, b image.Rectangle, c color.Color) {
	vector.FillRect(dst, float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy()), c, false)
}
func outline(dst *ebiten.Image, b image.Rectangle, c color.Color) {
	vector.StrokeRect(dst, float32(b.Min.X)+0.5, float32(b.Min.Y)+0.5, float32(b.Dx())-1, float32(b.Dy())-1, 1, c, false)
}

func (g *window) addButton(dst *ebiten.Image, b image.Rectangle, label string, enabled bool, action func()) {
	x, y := ebiten.CursorPosition()
	fill := color.Color(panel)
	if enabled && image.Pt(x, y).In(b) {
		fill = color.NRGBA{R: 43, G: 61, B: 73, A: 255}
	}
	rect(dst, b, fill)
	outline(dst, b, border)
	c := color.Color(muted)
	if enabled {
		c = ink
	}
	size := 15.0
	w, h := text.Measure(label, g.face(size, false), size)
	g.text(dst, label, float64(b.Min.X)+(float64(b.Dx())-w)/2, float64(b.Min.Y)+(float64(b.Dy())-h)/2-1, size, c, false)
	g.buttons = append(g.buttons, button{b, label, enabled, action})
}

func (g *window) wrap(value string, width, size float64) []string {
	result := []string{}
	for _, paragraph := range strings.Split(presentation.Clean(value), "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		current := ""
		for _, word := range strings.Fields(paragraph) {
			candidate := word
			if current != "" {
				candidate = current + " " + word
			}
			if w, _ := text.Measure(candidate, g.face(size, false), size); w <= width {
				current = candidate
				continue
			}
			if current != "" {
				result = append(result, current)
				current = ""
			}
			for _, r := range word {
				next := current + string(r)
				if w, _ := text.Measure(next, g.face(size, false), size); w > width && current != "" {
					result = append(result, current)
					current = string(r)
				} else {
					current = next
				}
			}
		}
		if current != "" {
			result = append(result, current)
		}
	}
	return result
}

func (g *window) Draw(screen *ebiten.Image) {
	screen.Fill(background)
	g.buttons = g.buttons[:0]
	g.text(screen, "AI VENTURE", 24, 18, 23, accent, true)
	g.text(screen, "1987  /  "+g.label("An illustrated text adventure"), 24, 48, 14, muted, false)
	ready := g.model.ready && !g.fatal
	g.addButton(screen, image.Rect(g.w-444, 22, g.w-342, 58), g.label("Look"), ready, func() { g.submit(g.local("look", "regarder")) })
	g.addButton(screen, image.Rect(g.w-332, 22, g.w-230, 58), g.label("Save"), ready, func() { g.submit(g.local("save", "sauver")) })
	g.addButton(screen, image.Rect(g.w-220, 22, g.w-118, 58), g.label("Load"), ready, func() { g.submit(g.local("load", "charger")) })
	g.addButton(screen, image.Rect(g.w-108, 22, g.w-24, 58), g.label("Help"), ready, func() { g.submit(g.local("help", "aide")) })
	body := image.Rect(24, 86, g.w-24, g.h-122)
	leftW := int(float64(body.Dx()-20) * 0.53)
	left := image.Rect(body.Min.X, body.Min.Y, body.Min.X+leftW, body.Max.Y)
	right := image.Rect(left.Max.X+20, body.Min.Y, body.Max.X, body.Max.Y)
	rect(screen, left, panel)
	outline(screen, left, border)
	name := g.model.view.Name
	if name == "" {
		name = g.label("The adventure awaits")
	}
	g.text(screen, name, float64(left.Min.X+16), float64(left.Min.Y+12), 24, ink, true)
	imageH := int(math.Min(float64(left.Dx()-32)*0.6, float64(left.Dy())*0.45))
	imageBox := image.Rect(left.Min.X+16, left.Min.Y+50, left.Max.X-16, left.Min.Y+50+imageH)
	rect(screen, imageBox, background)
	if g.scene != nil {
		bounds := g.scene.Bounds()
		scale := math.Min(float64(imageBox.Dx())/float64(bounds.Dx()), float64(imageBox.Dy())/float64(bounds.Dy()))
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(imageBox.Min.X)+(float64(imageBox.Dx())-float64(bounds.Dx())*scale)/2, float64(imageBox.Min.Y)+(float64(imageBox.Dy())-float64(bounds.Dy())*scale)/2)
		screen.DrawImage(g.scene, op)
	} else if len(g.model.art.Lines) > 0 {
		g.drawArt(screen, imageBox)
	} else {
		g.text(screen, g.label("No illustration for this location yet."), float64(imageBox.Min.X+20), float64(imageBox.Min.Y+imageBox.Dy()/2-8), 16, muted, false)
	}
	outline(screen, imageBox, border)
	descY := imageBox.Max.Y + 12
	lines := g.wrap(g.model.view.Description, float64(left.Dx()-32), 17)
	for i, line := range lines {
		if i == 3 {
			break
		}
		g.text(screen, line, float64(left.Min.X+16), float64(descY+i*22), 17, ink, false)
	}
	exitY := descY + 72
	for i, exit := range g.model.view.Exits {
		if i == 6 {
			break
		}
		x := left.Min.X + 16 + i*(left.Dx()-32)/max(len(g.model.view.Exits), 1)
		width := (left.Dx()-32)/max(len(g.model.view.Exits), 1) - 6
		label := exit.Direction
		if exit.Blocked {
			label += " ×"
		}
		direction := exit.Direction
		g.addButton(screen, image.Rect(x, exitY, x+width, exitY+33), label, ready && !exit.Blocked, func() { g.submit("/" + direction) })
	}
	g.drawItems(screen, image.Rect(left.Min.X+16, exitY+47, left.Max.X-16, left.Max.Y-12), ready)
	g.drawLog(screen, right)
	g.drawInput(screen)
	if g.fatal {
		g.addButton(screen, image.Rect(right.Min.X+16, right.Max.Y-53, right.Max.X-16, right.Max.Y-16), g.label("Play offline"), true, func() { g.start(true) })
	}
}

func (g *window) drawItems(screen *ebiten.Image, b image.Rectangle, ready bool) {
	g.itemsBounds = b
	g.addButton(screen, image.Rect(b.Min.X, b.Min.Y, b.Min.X+145, b.Min.Y+29), g.label("Objects here"), true, func() { g.inventory = false; g.itemScroll = 0 })
	g.addButton(screen, image.Rect(b.Min.X+152, b.Min.Y, b.Min.X+297, b.Min.Y+29), g.label("Inventory (%d)", len(g.model.view.Inventory)), true, func() { g.inventory = true; g.itemScroll = 0 })
	items := g.model.view.Items
	if g.inventory {
		items = g.model.view.Inventory
	}
	rows := max(1, (b.Dy()-38)/38)
	g.itemScroll = min(g.itemScroll, max(0, len(items)-rows))
	if len(items) == 0 {
		g.text(screen, g.label("Nothing here."), float64(b.Min.X+3), float64(b.Min.Y+42), 16, muted, false)
		return
	}
	for i := g.itemScroll; i < min(len(items), g.itemScroll+rows); i++ {
		item := items[i]
		y := b.Min.Y + 38 + (i-g.itemScroll)*38
		label := item.Name
		maxWidth := float64(b.Dx() - 250)
		for len(label) > 0 {
			if w, _ := text.Measure(label, g.face(16, false), 16); w <= maxWidth {
				break
			}
			r := []rune(label)
			label = string(r[:len(r)-1])
		}
		if label != item.Name {
			label += "…"
		}
		g.text(screen, label, float64(b.Min.X+3), float64(y+5), 16, ink, false)
		g.addButton(screen, image.Rect(b.Max.X-239, y, b.Max.X-166, y+29), g.label("Examine"), ready, func() { g.submit(g.local("examine "+item.ID, "observer "+item.ID)) })
		if g.inventory {
			g.addButton(screen, image.Rect(b.Max.X-159, y, b.Max.X-86, y+29), g.label("Use"), ready, func() {
				g.selectedTool = item.ID
				g.inventory = false
				prefix := g.local("use "+item.ID+" on ", "utiliser "+item.ID+" sur ")
				g.input.set(prefix)
				g.model.status = g.label("Choose a target object.")
			})
			g.addButton(screen, image.Rect(b.Max.X-79, y, b.Max.X, y+29), g.label("Drop"), ready, func() { g.submit(g.local("drop "+item.ID, "deposer "+item.ID)) })
		} else if g.selectedTool != "" {
			g.addButton(screen, image.Rect(b.Max.X-159, y, b.Max.X, y+29), g.label("Use on this"), ready, func() {
				g.submit(g.local("use "+g.selectedTool+" on "+item.ID, "utiliser "+g.selectedTool+" sur "+item.ID))
			})
		} else {
			g.addButton(screen, image.Rect(b.Max.X-159, y, b.Max.X, y+29), g.label("Take"), ready && item.Portable, func() { g.submit(g.local("take "+item.ID, "prendre "+item.ID)) })
		}
	}
}

func (g *window) drawLog(screen *ebiten.Image, b image.Rectangle) {
	rect(screen, b, panel)
	outline(screen, b, border)
	g.text(screen, g.label("ADVENTURE JOURNAL"), float64(b.Min.X+18), float64(b.Min.Y+15), 16, accent, true)
	g.text(screen, g.label("Turn %d", g.model.turn), float64(b.Max.X-100), float64(b.Min.Y+17), 14, muted, false)
	area := image.Rect(b.Min.X+18, b.Min.Y+49, b.Max.X-18, b.Max.Y-20)
	g.logBounds = area
	if g.model.version != g.logVersion || g.logWidth != area.Dx() {
		old := len(g.logLines)
		g.logLines = nil
		for _, entry := range g.model.entries {
			prefix := ""
			switch entry.tone {
			case presentation.Command:
				prefix = "> "
			}
			for _, value := range g.wrap(prefix+entry.text, float64(area.Dx()-9), 18) {
				g.logLines = append(g.logLines, line{value, entry.tone})
			}
			g.logLines = append(g.logLines, line{"", entry.tone})
		}
		if g.logScroll > 0 && g.logWidth == area.Dx() {
			g.logScroll = max(0, g.logScroll+len(g.logLines)-old)
		}
		g.logVersion = g.model.version
		g.logWidth = area.Dx()
	}
	rows := max(1, area.Dy()/25)
	g.logScroll = min(g.logScroll, max(0, len(g.logLines)-rows))
	start := max(0, len(g.logLines)-rows-g.logScroll)
	clipped := screen.SubImage(area).(*ebiten.Image)
	for i := start; i < min(len(g.logLines), start+rows); i++ {
		line := g.logLines[i]
		c := color.Color(ink)
		switch line.tone {
		case presentation.Command:
			c = muted
		case presentation.Fact, presentation.Success:
			c = green
		case presentation.Narration:
			c = accent
		case presentation.Error:
			c = red
		case presentation.Warning:
			c = accent
		}
		g.text(clipped, line.text, float64(area.Min.X), float64(area.Min.Y+(i-start)*25), 18, c, false)
	}
	if len(g.logLines) > rows {
		height := max(12, area.Dy()*rows/len(g.logLines))
		y := area.Min.Y + (area.Dy()-height)*(max(0, len(g.logLines)-rows)-g.logScroll)/max(1, len(g.logLines)-rows)
		rect(screen, image.Rect(b.Max.X-7, y, b.Max.X-4, y+height), border)
	}
}

func (g *window) drawInput(screen *ebiten.Image) {
	y := g.h - 106
	label := g.label("WHAT DO YOU DO?")
	if g.selectedTool != "" {
		label = g.label("Choose a target, or press Escape to cancel.")
	}
	g.text(screen, label, 24, float64(y), 14, accent, true)
	b := image.Rect(24, y+24, g.w-156, y+68)
	g.inputBounds = b
	rect(screen, b, panel)
	outline(screen, b, blue)
	value := g.input.String()
	prefix := string(g.input.text[:g.input.cursor])
	offset := 0.0
	if w, _ := text.Measure(prefix, g.face(19, false), 19); w > float64(b.Dx()-26) {
		offset = w - float64(b.Dx()-26)
	}
	clipped := screen.SubImage(image.Rect(b.Min.X+10, b.Min.Y, b.Max.X-8, b.Max.Y)).(*ebiten.Image)
	if g.input.all {
		w, _ := text.Measure(value, g.face(19, false), 19)
		x := int(float64(b.Min.X+12) - offset)
		rect(clipped, image.Rect(x, b.Min.Y+8, x+int(w), b.Max.Y-7), border)
	}
	if value == "" {
		g.text(clipped, g.label("Describe your action…"), float64(b.Min.X+12), float64(b.Min.Y+10), 19, muted, false)
	} else {
		g.text(clipped, value, float64(b.Min.X+12)-offset, float64(b.Min.Y+10), 19, ink, false)
	}
	if g.tick%60 < 35 && !g.fatal {
		w, _ := text.Measure(prefix, g.face(19, false), 19)
		x := int(float64(b.Min.X+12) + w - offset)
		rect(clipped, image.Rect(x, b.Min.Y+10, x+1, b.Max.Y-9), accent)
	}
	g.addButton(screen, image.Rect(g.w-144, y+24, g.w-24, y+68), g.label("Send"), g.model.ready && !g.fatal, func() { g.submit(g.input.String()) })
	status := g.model.status
	if !g.model.ready && !g.fatal {
		if status == "" {
			status = g.label("Working…")
		}
		status += strings.Repeat(".", g.tick/25%4)
	}
	if g.model.view.Completed {
		status = g.label("Objective complete — you can save or keep exploring.")
	}
	statusLines := g.wrap(status, float64(g.w-380), 13)
	if len(statusLines) > 0 {
		g.text(screen, statusLines[0], 24, float64(g.h-18), 13, muted, false)
	}
	g.text(screen, g.label("Enter: send   ↑↓: history   Esc: clear"), float64(g.w-342), float64(g.h-18), 12, muted, false)
}

func (g *window) drawArt(screen *ebiten.Image, b image.Rectangle) {
	size := math.Min(float64(b.Dx())/(80*0.6), float64(b.Dy())/24/1.2)
	face := &text.GoTextFace{Source: g.mono, Size: size}
	for i, row := range g.model.art.Lines {
		c := color.Color(ink)
		switch row.Color {
		case illustration.Cyan, illustration.Blue:
			c = blue
		case illustration.Yellow:
			c = accent
		case illustration.Green:
			c = green
		case illustration.Red:
			c = red
		case illustration.Gray:
			c = muted
		}
		op := &text.DrawOptions{}
		op.GeoM.Translate(float64(b.Min.X+4), float64(b.Min.Y)+float64(i)*size*1.2)
		op.ColorScale.ScaleWithColor(c)
		text.Draw(screen, row.Text, face, op)
	}
}
