//go:build js && wasm

package cosmos

import (
	"math"
	"syscall/js"
	"testing"
)

// glContext builds a real WebGL context to test against, or skips.
//
// Node has no canvas, so these only run under a browser:
//
//	make test-browser
//
// They are the only tests here that can say anything about the shaders. A
// GLSL error does not show up at build time — the source is a Go string —
// so without this it surfaces the first time someone opens the page.
func glContext(t *testing.T) *glCtx {
	t.Helper()
	doc := js.Global().Get("document")
	if !doc.Truthy() || doc.Get("createElement").Type() != js.TypeFunction {
		t.Skip("no document; run with make test-browser")
	}
	canvas := doc.Call("createElement", "canvas")
	canvas.Set("width", 64)
	canvas.Set("height", 64)

	c, err := newGLCtx(canvas)
	if err != nil {
		t.Skipf("no usable WebGL context: %v", err)
	}
	return c
}

// shaderPrograms is every vertex and fragment pair the package builds, named
// by its fragment shader.
func shaderPrograms() []struct {
	name, vert, frag string
} {
	return []struct{ name, vert, frag string }{
		{"calculateCentermass", calculateCentermassVert, calculateCentermassFrag},
		{"calculateLevel", calculateLevelVert, calculateLevelFrag},
		{"clusterCentermass", clusterCentermassVert, clusterCentermassFrag},
		{"drawHighlighted", drawHighlightedVert, drawHighlightedFrag},
		{"drawLine", drawLineVert, drawLineFrag},
		{"drawPoints", drawPointsVert, drawPointsFrag},
		{"fillSampledPoints", fillSampledPointsVert, fillSampledPointsFrag},
		{"findHoveredPoint", findHoveredPointVert, findHoveredPointFrag},
		{"hoveredLineIndex", hoveredLineIndexVert, hoveredLineIndexFrag},
		{"clear", quadVert, clearFrag},
		{"dragPoint", quadVert, dragPointFrag},
		{"findPointsOnAreaSelection", quadVert, findPointsOnAreaSelectionFrag},
		{"findPointsOnPolygonSelection", quadVert, findPointsOnPolygonSelectionFrag},
		{"forceCenter", quadVert, forceCenterFrag},
		{"forceCentermass", quadVert, forceCentermassFrag},
		{"forceCluster", quadVert, forceClusterFrag},
		{"forceGravity", quadVert, forceGravityFrag},
		{"forceLevel", quadVert, forceLevelFrag},
		{"forceMouse", quadVert, forceMouseFrag},
		// Both of these are generated per graph: the spring shader unrolls a loop
		// over the highest point degree, and the quadtree one over its levels. A
		// degree or level count that produces invalid GLSL is a real failure mode,
		// so a few are exercised below as well.
		{"forceSpring", quadVert, forceSpringFrag(8)},
		{"quadtree", quadVert, quadtreeFrag(0, 8)},
		{"trackPositions", quadVert, trackPositionsFrag},
		{"updatePosition", quadVert, updatePositionFrag},
	}
}

// Every shader pair the package builds has to compile and link. This is the
// test that a GLSL edit cannot be checked by the Go compiler at all — the
// shaders are strings, so a missing semicolon or a renamed uniform is a blank
// page rather than a build failure.
func TestEveryShaderProgramCompilesAndLinks(t *testing.T) {
	c := glContext(t)
	for _, p := range shaderPrograms() {
		prog, err := c.program(p.vert, p.frag)
		if err != nil {
			t.Errorf("%s: %v", p.name, err)
			continue
		}
		if !prog.prog.Truthy() {
			t.Errorf("%s: linked but has no program object", p.name)
		}
	}
}

// The count is checked separately so that a pair added to the package without
// being added here is noticed, rather than silently going untested.
func TestEveryProgramInThePackageIsListed(t *testing.T) {
	if got, want := len(shaderPrograms()), 23; got != want {
		t.Errorf("%d programs are listed, the package builds %d — update the table", got, want)
	}
}

// A shader that does not compile has to be reported as an error rather than
// returning a broken program that draws nothing.
func TestABrokenShaderIsReported(t *testing.T) {
	c := glContext(t)
	if _, err := c.program(quadVert, "this is not glsl"); err == nil {
		t.Error("a fragment shader that is not GLSL linked without complaint")
	}
	if _, err := c.program("neither is this", clearFrag); err == nil {
		t.Error("a vertex shader that is not GLSL linked without complaint")
	}
}

// The error has to say what went wrong, or a shader edit is debugged blind.
func TestAShaderErrorCarriesTheCompilerMessage(t *testing.T) {
	c := glContext(t)
	_, err := c.program(quadVert, `
		precision mediump float;
		void main() { gl_FragColor = undefinedSymbol; }
	`)
	if err == nil {
		t.Fatal("a shader referencing an undefined symbol compiled")
	}
	if len(err.Error()) < 20 {
		t.Errorf("the error is too short to be the compiler's: %q", err)
	}
}

// ── data round trips ─────────────────────────────────────────────────────────

// Positions and forces live in float textures, so what comes back out has to
// be what went in — this is the one place a driver difference would show.
func TestFloatTextureAndFramebufferRoundTrip(t *testing.T) {
	c := glContext(t)
	const w, h = 4, 4
	data := make([]float32, w*h*4)
	for i := range data {
		data[i] = float32(i) / 4
	}

	fb := c.newFramebuffer(w, h, data)
	if fb == nil {
		t.Fatal("no framebuffer")
	}
	defer fb.destroy()

	got := fb.readPixels()
	if len(got) != len(data) {
		t.Fatalf("read back %d floats, wrote %d", len(got), len(data))
	}
	for i := range data {
		if math.Abs(float64(got[i]-data[i])) > 1e-3 {
			t.Fatalf("float %d came back as %v, wrote %v", i, got[i], data[i])
		}
	}
}

func TestFloatConversionRoundTripsThroughJS(t *testing.T) {
	if !js.Global().Get("Float32Array").Truthy() {
		t.Skip("no Float32Array")
	}
	in := []float32{0, 1, -1, 0.5, 1e6, -1e-6}
	got := f32FromJS(f32ToJS(in))
	if len(got) != len(in) {
		t.Fatalf("got %d floats, sent %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("float %d came back as %v, sent %v", i, got[i], in[i])
		}
	}
}

func TestFloatConversionOnAnEmptySlice(t *testing.T) {
	if got := f32FromJS(f32ToJS(nil)); len(got) != 0 {
		t.Errorf("an empty slice came back with %d floats", len(got))
	}
}

func TestNewBuffer(t *testing.T) {
	c := glContext(t)
	b := c.newBuffer([]float32{0, 0, 1, 0, 1, 1})
	if b == nil || !b.buf.Truthy() {
		t.Fatal("no buffer object")
	}
	b.destroy()
}

func TestNewUint8Texture(t *testing.T) {
	c := glContext(t)
	tex := c.newUint8Texture(2, 2, make([]byte, 2*2*4))
	if tex == nil || !tex.tex.Truthy() {
		t.Fatal("no texture object")
	}
	tex.destroy()
}

// Clearing a target to a known color and reading it back is the smallest
// end-to-end check that the pipeline actually draws.
func TestClearTargetWritesTheColor(t *testing.T) {
	c := glContext(t)
	const w, h = 2, 2
	fb := c.newFramebuffer(w, h, make([]float32, w*h*4))
	defer fb.destroy()

	c.clearTarget(fb, 0.25, 0.5, 0.75, 1)
	got := fb.readPixels()
	if len(got) < 4 {
		t.Fatalf("read back %d floats", len(got))
	}
	for i, want := range []float32{0.25, 0.5, 0.75, 1} {
		if math.Abs(float64(got[i]-want)) > 1e-2 {
			t.Errorf("channel %d is %v, want %v", i, got[i], want)
		}
	}
}

// A uniform the shader does not declare has no location, and asking for one
// must come back empty rather than panicking — shaders share code paths and
// not every one uses every uniform.
func TestUniformLocationOfSomethingNotDeclared(t *testing.T) {
	c := glContext(t)
	p, err := c.program(quadVert, clearFrag)
	if err != nil {
		t.Fatalf("clear program: %v", err)
	}
	if loc := p.uniformLoc("noSuchUniformAnywhere"); loc.Truthy() {
		t.Errorf("an undeclared uniform reported the location %v", loc)
	}
}

func TestAttribLocationOfSomethingNotDeclared(t *testing.T) {
	c := glContext(t)
	p, err := c.program(quadVert, clearFrag)
	if err != nil {
		t.Fatalf("clear program: %v", err)
	}
	if loc := p.attribLoc("noSuchAttributeAnywhere"); loc >= 0 {
		t.Errorf("an undeclared attribute reported the location %d", loc)
	}
}

// The spring shader unrolls a loop over the highest degree in the graph, so
// the source is different for every graph. A degree that generates invalid
// GLSL is a page that does not draw, and only a browser can catch it.
func TestForceSpringCompilesAtEveryDegreeItWillSee(t *testing.T) {
	c := glContext(t)
	for _, degree := range []int{0, 1, 2, 8, 32, 100} {
		if _, err := c.program(quadVert, forceSpringFrag(degree)); err != nil {
			t.Errorf("max degree %d: %v", degree, err)
		}
	}
}

// The quadtree shader unrolls over its levels, and is built once per level, so
// every starting level has to produce valid GLSL.
func TestQuadtreeCompilesAtEveryLevel(t *testing.T) {
	c := glContext(t)
	const levels = 8
	for start := 0; start < levels; start++ {
		if _, err := c.program(quadVert, quadtreeFrag(start, levels)); err != nil {
			t.Errorf("start level %d of %d: %v", start, levels, err)
		}
	}
}
