package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/light"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/renderer"
	"github.com/g3n/engine/window"

	"gcodegen.local/viewer/internal/parser"
	"gcodegen.local/viewer/internal/scene"
	"gcodegen.local/viewer/internal/version"
)

// windowTitle composes the title-bar string. Format:
//
//	GcodeSim V3.0.0 | Nightbots 10686 | 416aab
//
// or, when a newer release is detected on GitHub:
//
//	GcodeSim V3.0.0 → V3.0.1 available | Nightbots 10686 | 416aab
func windowTitle(current, latest string) string {
	cur := displayVersion(current)
	if latest != "" && latest != current {
		return "GcodeSim " + cur + " → " + displayVersion(latest) + " available | Nightbots 10686 | 416aab"
	}
	return "GcodeSim " + cur + " | Nightbots 10686 | 416aab"
}

// displayVersion normalizes a git tag (typically "v3.0.0" with a lowercase
// leading 'v', the git/semver convention) into the display string we want
// in the title bar ("V3.0.0"). Returns "(dev)" for unversioned local
// builds — the binary that wasn't built via build.bat / build.sh, so no
// -ldflags injection happened.
func displayVersion(v string) string {
	if v == "" || v == "dev" {
		return "(dev)"
	}
	return "V" + strings.TrimLeft(v, "vV")
}

// Default bit diameter (mm) until the user changes it via the toolbar entry.
const defaultBitDiameter = 6.35 // 1/4" — common CNC bit

// View-cube viewport size in pixels (corner widget; positioned top-right
// just below the toolbar).
const cubeViewportSize = 90

// Run is the top-level entry: opens a g3n window, sets up scene/camera/lights,
// installs key handlers + toolbar, and runs the manual render loop until the
// user exits.
//
// We bypass `engine/app` because that package transitively imports
// `engine/audio/al` and `engine/audio/vorbis`, forcing a runtime dep on
// OpenAL32.dll + libvorbis.dll. By going directly to `window` + `renderer`
// the binary stays self-contained and runs on stock Windows.
func Run(initialPath string) {
	if err := window.Init(1280, 800, windowTitle(version.Version, "")); err != nil {
		panic(fmt.Errorf("window init: %w", err))
	}
	win := window.Get().(*window.GlfwWindow)

	// Async update check: fetch the latest release tag from GitHub off the
	// main thread (5-sec timeout, silent fail). When (and IF) it returns,
	// the main loop reads the channel and updates the window title once.
	// Skipped entirely for `go run` / unversioned dev builds.
	updateCheckCh := make(chan string, 1)
	if !version.IsDev() {
		go func() {
			latest, err := version.LatestRelease()
			if err == nil && latest != "" && latest != version.Version {
				updateCheckCh <- latest
			}
		}()
	}

	rend := renderer.NewRenderer(win.Gls())
	if err := rend.AddDefaultShaders(); err != nil {
		panic(fmt.Errorf("shaders: %w", err))
	}

	// Main scene: camera, lights, content
	sceneRoot := core.NewNode()
	gui.Manager().Set(sceneRoot)

	contentRoot := core.NewNode()
	sceneRoot.Add(contentRoot)

	cam := camera.New(1)
	cam.SetPosition(150, -150, 150)
	cam.LookAt(&math32.Vector3{X: 0, Y: 0, Z: 0}, &math32.Vector3{X: 0, Y: 0, Z: 1})
	sceneRoot.Add(cam)

	// Custom Z-up orbiter (g3n's OrbitControl is hard-coded to Y-up — see
	// orbiter.go for the long story).
	orbit := NewOrbiter(cam, win)

	sceneRoot.Add(light.NewAmbient(&math32.Color{R: 1, G: 1, B: 1}, 0.5))
	dir := light.NewDirectional(&math32.Color{R: 1, G: 1, B: 1}, 0.7)
	dir.SetPosition(100, -100, 200)
	sceneRoot.Add(dir)

	cube := scene.NewViewCube()

	state := &sceneState{
		win:         win,
		contentRoot: contentRoot,
		cam:         cam,
		orbit:       orbit,
		cube:        cube,
		bitDiameter: defaultBitDiameter,
		focal:       math32.Vector3{X: 0, Y: 0, Z: 0},
		camDist:     260,
	}

	// Toolbar — sits at top, full-width, two rows.
	w0, _ := win.GetSize()
	state.toolbar = NewToolbar(float32(w0), defaultBitDiameter, ToolbarCallbacks{
		OnOpen:                     func() { state.openFileDialog() },
		OnPlayPause:                func() { state.togglePlayPause() },
		OnReset:                    func() { state.resetPlayback() },
		OnReframe:                  func() { state.frameCamera() },
		OnSpeedChanged:             func(mult float64) { state.setSpeed(mult) },
		OnBitDiaApplied:            func(d float64) { state.setBitDiameter(d) },
		OnProgressScrub:            func(f float64) { state.scrubTo(f) },
		OnMaterialThicknessApplied: func(mm float64) { state.setMaterialThickness(mm) },
	})
	sceneRoot.Add(state.toolbar.Panel)
	// OptionsPanel is a sibling of the toolbar (not a child) so it can render
	// freely below the toolbar instead of being clipped to the toolbar's
	// 64-pixel rectangle.
	sceneRoot.Add(state.toolbar.OptionsPanel)

	resize := func(_ string, _ interface{}) {
		// On macOS Retina displays the framebuffer is 2× the window size.
		// GL viewport must be in framebuffer (physical) pixels; toolbar
		// width and camera aspect use window (logical) pixels — those
		// match cursor coords for hit-testing.
		w, h := win.GetSize()
		fbW, fbH := win.GetFramebufferSize()
		win.Gls().Viewport(0, 0, int32(fbW), int32(fbH))
		cam.SetAspect(float32(w) / float32(h))
		state.toolbar.Resize(float32(w))
	}
	win.Subscribe(window.OnWindowSize, resize)
	resize("", nil)

	win.Subscribe(window.OnKeyDown, func(_ string, ev interface{}) {
		ke, ok := ev.(*window.KeyEvent)
		if !ok {
			return
		}
		ctrl := ke.Mods&window.ModControl != 0

		switch {
		case ke.Key == window.KeyO && ctrl:
			state.openFileDialog()
		case ke.Key == window.KeyR:
			state.frameCamera()
		case ke.Key == window.KeySpace:
			state.togglePlayPause()
		case ke.Key == window.KeyEscape:
			win.SetShouldClose(true)
		}
	})

	// Mouse click on the view cube → snap the main camera to that face.
	win.Subscribe(window.OnMouseDown, func(_ string, ev interface{}) {
		me, ok := ev.(*window.MouseEvent)
		if !ok {
			return
		}
		if me.Button != window.MouseButtonLeft {
			return
		}
		w, h := win.GetSize()
		if !inCubeViewport(int(me.Xpos), int(me.Ypos), w, h) {
			return
		}
		state.cube.SyncToMainCamera(cam, state.focal)
		ndcX, ndcY := cubeNDC(int(me.Xpos), int(me.Ypos), w, h)
		if face := state.cube.PickFace(ndcX, ndcY); face != nil {
			state.snapCameraToFace(face)
		}
	})

	win.Gls().ClearColor(0.10, 0.11, 0.13, 1.0)

	if initialPath != "" {
		if err := state.loadFile(initialPath); err != nil {
			fmt.Printf("warning: failed to load %q: %v\n", initialPath, err)
		}
	}

	// Manual main loop with multi-pass render and per-frame playback tick.
	lastTick := time.Now()
	for !win.ShouldClose() {
		now := time.Now()
		dt := now.Sub(lastTick).Seconds()
		lastTick = now

		// Window size = logical/cursor pixels. Framebuffer size = physical
		// (2× on macOS Retina). All GL viewport/scissor calls use
		// framebuffer pixels; cursor hit-tests stay in window pixels.
		w, h := win.GetSize()
		fbW, fbH := win.GetFramebufferSize()
		scale := float32(fbW) / float32(w)
		if scale <= 0 {
			scale = 1
		}

		// Pick up the update-check result if it arrived this frame and
		// stamp the title bar. Non-blocking — most frames will hit the
		// default branch and skip.
		select {
		case latest := <-updateCheckCh:
			win.SetTitle(windowTitle(version.Version, latest))
		default:
		}

		// Refresh focal from the orbiter (handles user pans).
		state.focal = state.orbit.Target()

		// Advance simulation clock.
		state.tickPlayback(dt)

		// Sync cube cam BEFORE hover so picking matches what's rendered.
		state.cube.SyncToMainCamera(cam, state.focal)
		state.updateCubeHover(w, h)

		// Pass 1: main scene, full framebuffer viewport
		win.Gls().Viewport(0, 0, int32(fbW), int32(fbH))
		win.Gls().Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		if err := rend.Render(sceneRoot, cam); err != nil {
			fmt.Println("render error:", err)
		}

		// Pass 2: view cube into top-right corner — sized in framebuffer
		// pixels via the DPI scale so it lines up with the toolbar even
		// on Retina displays.
		cubeSizePhys := int32(float32(cubeViewportSize) * scale)
		toolbarPhys := int32(float32(toolbarHeight) * scale)
		cubeX := int32(fbW) - cubeSizePhys
		cubeY := int32(fbH) - cubeSizePhys - toolbarPhys
		win.Gls().Enable(gls.SCISSOR_TEST)
		win.Gls().Scissor(cubeX, cubeY, uint32(cubeSizePhys), uint32(cubeSizePhys))
		win.Gls().Viewport(cubeX, cubeY, cubeSizePhys, cubeSizePhys)
		win.Gls().Clear(gls.DEPTH_BUFFER_BIT)
		if err := rend.Render(state.cube.Scene, state.cube.Camera); err != nil {
			fmt.Println("cube render error:", err)
		}
		win.Gls().Disable(gls.SCISSOR_TEST)
		win.Gls().Viewport(0, 0, int32(fbW), int32(fbH))

		win.SwapBuffers()
		win.PollEvents()
	}
}

// sceneState holds everything that gets rebuilt or mutated on file load /
// playback / user input.
type sceneState struct {
	win         *window.GlfwWindow
	contentRoot *core.Node
	cam         *camera.Camera
	orbit       *Orbiter
	cube        *scene.ViewCube
	toolbar     *Toolbar

	moves    []*parser.Move
	min, max parser.Point
	tool     *scene.Tool
	playback *scene.Playback

	// Material-removal heightmap: rebuilt on file load and on Reset, eroded
	// per-tick by CutSegment along the swept tool path. The actor returned
	// by heightmap.Actor() is cached, so removing on rebuild uses the same
	// reference we added.
	heightmap   *scene.Heightmap
	lastToolPos parser.Point
	refreshCtr  int // throttles GPU mesh re-upload to ~15 Hz

	focal   math32.Vector3
	camDist float32

	bitDiameter float64
}

func (s *sceneState) openFileDialog() {
	path, err := OpenGCodeFile()
	if err != nil {
		ShowError("Open file", "Failed to open file dialog: %v", err)
		return
	}
	if path == "" {
		return
	}
	if err := s.loadFile(path); err != nil {
		ShowError("Open file", "Failed to load %q:\n%v", filepath.Base(path), err)
	}
}

// loadFile parses, tears down the previous content tree, and rebuilds the
// path/stock/tool actors plus the playback state for the new program.
func (s *sceneState) loadFile(path string) error {
	moves, err := parser.ParseFile(path)
	if err != nil {
		return err
	}
	if len(moves) == 0 {
		return fmt.Errorf("no moves parsed from %s", filepath.Base(path))
	}

	min, max, ok := parser.Bounds(moves)
	if !ok {
		return fmt.Errorf("could not compute bounds")
	}
	deepest := parser.DeepestCutZ(moves)

	children := s.contentRoot.Children()
	for i := len(children) - 1; i >= 0; i-- {
		s.contentRoot.Remove(children[i])
	}

	// Stock outline (no fill — the carved heightmap surface fills the volume)
	s.contentRoot.Add(scene.NewStockWireframe(min, max, 0))
	s.contentRoot.Add(scene.NewPathActor(moves, deepest))

	// Heightmap material-removal surface
	s.heightmap = scene.NewHeightmap(
		[2]float64{min.X - scene.StockMargin, max.X + scene.StockMargin},
		[2]float64{min.Y - scene.StockMargin, max.Y + scene.StockMargin},
		0,
		scene.HeightmapCellSize(s.bitDiameter),
	)
	s.contentRoot.Add(s.heightmap.Actor(scene.StockColor))

	tool := scene.NewTool(s.bitDiameter)
	first := moves[0].Points[0]
	tool.SetPosition(first.X, first.Y, first.Z)
	tool.SetSpindle(false)
	s.contentRoot.Add(tool.Node)
	s.tool = tool
	s.lastToolPos = first

	s.moves = moves
	s.min = min
	s.max = max
	s.playback = scene.NewPlayback(moves)
	s.toolbar.SetPlaying(false)
	s.toolbar.SetProgress(0)
	s.refreshCtr = 0

	s.frameCamera()
	return nil
}

// tickPlayback advances simulated time, repositions the tool, updates the
// LED, erodes the heightmap along the swept path, and refreshes the toolbar
// progress slider.
func (s *sceneState) tickPlayback(dt float64) {
	if s.playback == nil || s.tool == nil {
		return
	}
	wasRunning := s.playback.Running

	pos, spindle := s.playback.Tick(dt)

	// Material removal: erode the heightmap along the segment from the
	// previous frame's position to the current one. Skip rapids (G0) and
	// any time the spindle was off — both match the v2 behavior.
	if wasRunning && s.heightmap != nil && spindle && s.playback.MoveIndex < len(s.moves) {
		if s.moves[s.playback.MoveIndex].Kind != "G0" {
			s.heightmap.CutSegment(s.lastToolPos, pos, s.bitDiameter/2)
		}
	}
	s.lastToolPos = pos

	s.tool.SetPosition(pos.X, pos.Y, pos.Z)
	s.tool.SetSpindle(spindle)

	// Throttle the GPU re-upload — cuts every tick, but mesh refresh ~15 Hz
	// (matches v2's SURFACE_REFRESH_EVERY = 4 at 60 Hz).
	s.refreshCtr++
	if s.refreshCtr >= 4 {
		if s.heightmap != nil {
			s.heightmap.RefreshMesh()
		}
		s.refreshCtr = 0
	}

	s.toolbar.SetProgress(s.playback.Progress())

	if wasRunning && !s.playback.Running {
		s.toolbar.SetPlaying(false)
		// Final mesh refresh so the carved surface reflects every tick
		// even if we weren't due for a refresh on the last frame.
		if s.heightmap != nil {
			s.heightmap.RefreshMesh()
		}
	}
}

func (s *sceneState) togglePlayPause() {
	if s.playback == nil {
		return
	}
	if s.playback.Running {
		s.playback.Running = false
	} else {
		// If we ran to the end, start from 0 again on play.
		if s.playback.Done() {
			s.playback.Reset()
		}
		s.playback.Running = true
	}
	s.toolbar.SetPlaying(s.playback.Running)
}

func (s *sceneState) resetPlayback() {
	if s.playback == nil {
		return
	}
	s.playback.Reset()

	// Rebuild the heightmap so the carved surface starts fresh. Cell size
	// follows the CURRENT bit diameter, so changing the bit and resetting
	// gives a properly resolved mesh for the new tool.
	if s.heightmap != nil && len(s.moves) > 0 {
		s.contentRoot.Remove(s.heightmap.Actor(scene.StockColor))
		s.heightmap = scene.NewHeightmap(
			[2]float64{s.min.X - scene.StockMargin, s.max.X + scene.StockMargin},
			[2]float64{s.min.Y - scene.StockMargin, s.max.Y + scene.StockMargin},
			0,
			scene.HeightmapCellSize(s.bitDiameter),
		)
		s.contentRoot.Add(s.heightmap.Actor(scene.StockColor))
	}

	pos := s.playback.CurrentPosition()
	s.tool.SetPosition(pos.X, pos.Y, pos.Z)
	s.tool.SetSpindle(false)
	s.lastToolPos = pos
	s.refreshCtr = 0
	s.toolbar.SetPlaying(false)
	s.toolbar.SetProgress(0)
}

func (s *sceneState) setSpeed(mult float64) {
	if s.playback == nil {
		return
	}
	s.playback.SpeedMult = mult
}

func (s *sceneState) scrubTo(fraction float64) {
	if s.playback == nil {
		return
	}
	s.playback.SetProgress(fraction)
	pos := s.playback.CurrentPosition()
	s.tool.SetPosition(pos.X, pos.Y, pos.Z)
	// Reset the cut anchor to the scrub position so resuming play doesn't
	// erode a phantom segment from the pre-scrub location to the new one.
	// Note: scrubbing back doesn't un-cut already-eroded material — to
	// undo cuts the user has to Reset (which rebuilds the heightmap).
	s.lastToolPos = pos
	if s.playback.MoveIndex < len(s.moves) {
		s.tool.SetSpindle(s.moves[s.playback.MoveIndex].Spindle)
	}
}

// setMaterialThickness pushes a new stock-thickness value into the heightmap
// so future cuts that reach the bottom mark cells as "through". Re-evaluates
// existing cells against the new bottom and immediately refreshes the mesh
// (so toggling the value has an instant visual effect).
func (s *sceneState) setMaterialThickness(mm float64) {
	if s.heightmap == nil {
		return
	}
	s.heightmap.SetMaterialThickness(mm)
	s.heightmap.RefreshMesh()
}

// setBitDiameter rebuilds the tool actor at the new bit size. The new tool
// is placed at the current playback position so the user sees the change
// in-place without re-loading the file.
func (s *sceneState) setBitDiameter(d float64) {
	s.bitDiameter = d
	if s.tool == nil || s.contentRoot == nil {
		return
	}
	s.contentRoot.Remove(s.tool.Node)

	tool := scene.NewTool(d)
	pos := parser.Point{}
	if s.playback != nil {
		pos = s.playback.CurrentPosition()
	} else if len(s.moves) > 0 {
		pos = s.moves[0].Points[0]
	}
	tool.SetPosition(pos.X, pos.Y, pos.Z)
	if s.playback != nil && s.playback.MoveIndex < len(s.moves) {
		tool.SetSpindle(s.moves[s.playback.MoveIndex].Spindle && s.playback.Running)
	}
	s.contentRoot.Add(tool.Node)
	s.tool = tool
}

// frameCamera repositions the camera to fit the current model bounds.
func (s *sceneState) frameCamera() {
	if s.moves == nil {
		return
	}
	cx := float32((s.min.X + s.max.X) / 2)
	cy := float32((s.min.Y + s.max.Y) / 2)
	cz := float32((s.min.Z + s.max.Z) / 2)

	dx := float32(s.max.X - s.min.X)
	dy := float32(s.max.Y - s.min.Y)
	dz := float32(s.max.Z - s.min.Z)
	span := dx
	if dy > span {
		span = dy
	}
	if dz > span {
		span = dz
	}
	if span < 1 {
		span = 1
	}
	dist := span * 1.8

	s.focal = math32.Vector3{X: cx, Y: cy, Z: cz}
	s.camDist = dist

	s.cam.SetPosition(cx-dist*0.6, cy-dist*0.6, cz+dist*0.7)
	s.cam.LookAt(
		&math32.Vector3{X: cx, Y: cy, Z: cz},
		&math32.Vector3{X: 0, Y: 0, Z: 1},
	)
	s.orbit.SetTarget(s.focal)
}

// snapCameraToFace places the main camera looking down `face.Normal` toward
// the current focal point. Distance is read from the CURRENT camera so the
// snap respects any zoom the user has done.
func (s *sceneState) snapCameraToFace(face *scene.ViewCubeFace) {
	p := s.cam.Position()
	dx := p.X - s.focal.X
	dy := p.Y - s.focal.Y
	dz := p.Z - s.focal.Z
	d := math32.Sqrt(dx*dx + dy*dy + dz*dz)
	if d < 1 {
		d = s.camDist
	}
	if d < 1 {
		d = 100
	}

	s.cam.SetPosition(
		s.focal.X+face.Normal.X*d,
		s.focal.Y+face.Normal.Y*d,
		s.focal.Z+face.Normal.Z*d,
	)
	s.cam.LookAt(
		&math32.Vector3{X: s.focal.X, Y: s.focal.Y, Z: s.focal.Z},
		&math32.Vector3{X: face.Up.X, Y: face.Up.Y, Z: face.Up.Z},
	)
	s.orbit.SyncFromCamera()
}

func (s *sceneState) updateCubeHover(w, h int) {
	mx, my := s.win.GetCursorPos()
	if !inCubeViewport(int(mx), int(my), w, h) {
		s.cube.SetHovered(nil)
		return
	}
	ndcX, ndcY := cubeNDC(int(mx), int(my), w, h)
	s.cube.SetHovered(s.cube.PickFace(ndcX, ndcY))
}

func inCubeViewport(mx, my, winW, winH int) bool {
	_ = winH
	x0 := winW - cubeViewportSize
	y0 := int(toolbarHeight)
	y1 := y0 + cubeViewportSize
	return mx >= x0 && my >= y0 && my <= y1
}

func cubeNDC(mx, my, winW, winH int) (ndcX, ndcY float32) {
	_ = winH
	localX := float32(mx - (winW - cubeViewportSize))
	localY := float32(my - int(toolbarHeight))
	ndcX = (localX/float32(cubeViewportSize))*2 - 1
	ndcY = ((float32(cubeViewportSize)-localY)/float32(cubeViewportSize))*2 - 1
	return
}
