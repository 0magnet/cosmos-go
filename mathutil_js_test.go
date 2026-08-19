//go:build js && wasm

package cosmos

import (
	"math"
	"testing"
)

// ── linearScale ──────────────────────────────────────────────────────────────

func TestLinearScaleMapsTheEndsAndTheMiddle(t *testing.T) {
	s := linearScale{d0: 0, d1: 10, r0: 100, r1: 200}
	for _, tc := range []struct{ in, want float64 }{
		{0, 100}, {10, 200}, {5, 150}, {2.5, 125},
	} {
		if got := s.scale(tc.in); !near(got, tc.want) {
			t.Errorf("scale(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Outside the domain the scale keeps going rather than clamping, which is what
// a d3 linear scale does and what the callers rely on.
func TestLinearScaleExtrapolates(t *testing.T) {
	s := linearScale{d0: 0, d1: 10, r0: 0, r1: 100}
	if got := s.scale(20); !near(got, 200) {
		t.Errorf("scale(20) = %v, want 200", got)
	}
	if got := s.scale(-5); !near(got, -50) {
		t.Errorf("scale(-5) = %v, want -50", got)
	}
}

func TestLinearScaleHandlesAnInvertedRange(t *testing.T) {
	s := linearScale{d0: 0, d1: 1, r0: 100, r1: 0}
	if got := s.scale(0.25); !near(got, 75) {
		t.Errorf("scale(0.25) on an inverted range = %v, want 75", got)
	}
}

// A domain of zero width would divide by zero. Every value maps to the middle
// of the range instead, which is what keeps a graph where every node has the
// same value from collapsing to NaN.
func TestLinearScaleWithAnEmptyDomainIsTheMidpoint(t *testing.T) {
	s := linearScale{d0: 5, d1: 5, r0: 0, r1: 100}
	for _, in := range []float64{-1, 0, 5, 1000} {
		got := s.scale(in)
		if math.IsNaN(got) {
			t.Fatalf("scale(%v) with an empty domain is NaN", in)
		}
		if !near(got, 50) {
			t.Errorf("scale(%v) = %v, want the midpoint 50", in, got)
		}
	}
}

// ── extent ───────────────────────────────────────────────────────────────────

func TestExtent(t *testing.T) {
	for _, tc := range []struct {
		in               []float64
		wantMin, wantMax float64
	}{
		{[]float64{3, 1, 2}, 1, 3},
		{[]float64{5}, 5, 5},
		{[]float64{-2, -9, -1}, -9, -1},
		{[]float64{0, 0, 0}, 0, 0},
	} {
		min, max := extent(tc.in)
		if !near(min, tc.wantMin) || !near(max, tc.wantMax) {
			t.Errorf("extent(%v) = %v,%v want %v,%v", tc.in, min, max, tc.wantMin, tc.wantMax)
		}
	}
}

func TestExtentOfNothingIsZero(t *testing.T) {
	for _, in := range [][]float64{nil, {}} {
		if min, max := extent(in); min != 0 || max != 0 {
			t.Errorf("extent(%v) = %v,%v want 0,0", in, min, max)
		}
	}
}

// A NaN in the data must not become the extent, or every scale built from it
// produces NaN and the whole graph disappears.
func TestExtentIgnoresNaN(t *testing.T) {
	nan := math.NaN()
	min, max := extent([]float64{nan, 3, nan, 1, nan})
	if !near(min, 1) || !near(max, 3) {
		t.Errorf("extent with NaNs = %v,%v want 1,3", min, max)
	}
}

// All NaN leaves nothing to measure, and zero is a usable answer where NaN or
// an infinity would poison everything downstream.
func TestExtentOfAllNaNIsZero(t *testing.T) {
	nan := math.NaN()
	min, max := extent([]float64{nan, nan})
	if min != 0 || max != 0 {
		t.Errorf("extent of all NaN = %v,%v want 0,0", min, max)
	}
	if math.IsInf(min, 0) || math.IsInf(max, 0) {
		t.Error("extent leaked the sentinel infinities it starts from")
	}
}

// ── mat3 ─────────────────────────────────────────────────────────────────────

func TestMat3Identity(t *testing.T) {
	m := mat3Identity()
	want := mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}
	if m != want {
		t.Errorf("identity = %v, want %v", m, want)
	}
}

// The projection maps pixels to clip space: the top-left corner to (-1, 1) and
// the bottom-right to (1, -1). Getting the y flip wrong renders upside down.
func TestMat3ProjectionMapsPixelsToClipSpace(t *testing.T) {
	var m mat3
	m.projection(800, 600)

	apply := func(x, y float64) (float64, float64) {
		cx := float64(m[0])*x + float64(m[3])*y + float64(m[6])
		cy := float64(m[1])*x + float64(m[4])*y + float64(m[7])
		return cx, cy
	}

	for _, tc := range []struct{ x, y, wantX, wantY float64 }{
		{0, 0, -1, 1},     // top left
		{800, 600, 1, -1}, // bottom right
		{400, 300, 0, 0},  // center
	} {
		gx, gy := apply(tc.x, tc.y)
		if !near(gx, tc.wantX) || !near(gy, tc.wantY) {
			t.Errorf("(%v,%v) projected to (%v,%v), want (%v,%v)",
				tc.x, tc.y, gx, gy, tc.wantX, tc.wantY)
		}
	}
}

func TestMat3TranslateMovesTheOrigin(t *testing.T) {
	m := mat3Identity()
	m.translate(5, 7)
	if m[6] != 5 || m[7] != 7 {
		t.Errorf("after translate the offset is %v,%v want 5,7", m[6], m[7])
	}
}

func TestMat3ScaleScalesTheAxes(t *testing.T) {
	m := mat3Identity()
	m.scale(2, 3)
	if m[0] != 2 || m[4] != 3 {
		t.Errorf("after scale the axes are %v,%v want 2,3", m[0], m[4])
	}
}

// Translate then scale is not the same as scale then translate, and the order
// the callers use has to keep the translation in pixels.
func TestMat3TranslateThenScale(t *testing.T) {
	m := mat3Identity()
	m.translate(10, 20)
	m.scale(2, 2)
	if m[6] != 10 || m[7] != 20 {
		t.Errorf("scaling after translating moved the offset to %v,%v want 10,20", m[6], m[7])
	}
	if m[0] != 2 || m[4] != 2 {
		t.Errorf("the axes are %v,%v want 2,2", m[0], m[4])
	}
}

// ── easing ───────────────────────────────────────────────────────────────────

// Every easing runs from 0 to 1 over 0 to 1, or an animation starts or ends
// somewhere other than where it should.
func TestEasingsSpanZeroToOne(t *testing.T) {
	for name, fn := range map[string]func(float64) float64{
		"quadIn":    easeQuadIn,
		"quadOut":   easeQuadOut,
		"quadInOut": easeQuadInOut,
	} {
		if got := fn(0); !near(got, 0) {
			t.Errorf("%s(0) = %v, want 0", name, got)
		}
		if got := fn(1); !near(got, 1) {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
	}
}

func TestEasingsAreMonotonic(t *testing.T) {
	for name, fn := range map[string]func(float64) float64{
		"quadIn":    easeQuadIn,
		"quadOut":   easeQuadOut,
		"quadInOut": easeQuadInOut,
	} {
		prev := fn(0)
		for i := 1; i <= 100; i++ {
			v := fn(float64(i) / 100)
			if v < prev-1e-9 {
				t.Errorf("%s went backwards at t=%v: %v after %v", name, float64(i)/100, v, prev)
				break
			}
			prev = v
		}
	}
}

// The point of the three is that they are differently shaped: ease-in starts
// slow, ease-out starts fast.
func TestEasingsDifferInShape(t *testing.T) {
	in, out := easeQuadIn(0.25), easeQuadOut(0.25)
	if in >= out {
		t.Errorf("quadIn(0.25)=%v is not below quadOut(0.25)=%v", in, out)
	}
	if got := easeQuadInOut(0.5); !near(got, 0.5) {
		t.Errorf("quadInOut(0.5) = %v, want the halfway point 0.5", got)
	}
}

// ── rng ──────────────────────────────────────────────────────────────────────

// A fixed seed has to give a reproducible layout, which is the whole reason
// this exists rather than math/rand.
func TestRngIsReproducible(t *testing.T) {
	a, b := newRng(42), newRng(42)
	for i := 0; i < 100; i++ {
		if x, y := a.next(), b.next(); x != y {
			t.Fatalf("two generators with the same seed diverged at %d: %v vs %v", i, x, y)
		}
	}
}

func TestRngDiffersBySeed(t *testing.T) {
	a, b := newRng(1), newRng(2)
	same := 0
	for i := 0; i < 50; i++ {
		if a.next() == b.next() {
			same++
		}
	}
	if same > 1 {
		t.Errorf("two different seeds produced %d identical values in 50", same)
	}
}

// A zero seed would leave splitmix64 in a degenerate state, so it is replaced.
func TestRngZeroSeedIsReplaced(t *testing.T) {
	r := newRng(0)
	if r.state == 0 {
		t.Fatal("a zero seed was kept")
	}
	seen := map[uint64]bool{}
	for i := 0; i < 20; i++ {
		v := r.next()
		if seen[v] {
			t.Fatalf("a zero-seeded generator repeated %v within 20 draws", v)
		}
		seen[v] = true
	}
}

func TestRngFloatStaysInRange(t *testing.T) {
	r := newRng(7)
	for i := 0; i < 1000; i++ {
		v := r.float(-2, 5)
		if v < -2 || v >= 5 {
			t.Fatalf("float(-2, 5) gave %v", v)
		}
	}
}

func TestRngFloatWithAnEmptyRange(t *testing.T) {
	r := newRng(7)
	for i := 0; i < 10; i++ {
		if got := r.float(3, 3); !near(got, 3) {
			t.Errorf("float(3, 3) = %v, want 3", got)
		}
	}
}

func TestSeedFromStringIsStableAndDistinct(t *testing.T) {
	first, again := seedFromString("abc"), seedFromString("abc")
	if first != again {
		t.Errorf("the same string gave %v and then %v", first, again)
	}
	seeds := map[uint64]string{}
	for _, s := range []string{"", "a", "b", "abc", "abd", "graph-1", "graph-2"} {
		h := seedFromString(s)
		if prev, dup := seeds[h]; dup {
			t.Errorf("%q and %q hash to the same seed", prev, s)
		}
		seeds[h] = s
	}
}
