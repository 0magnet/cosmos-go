//go:build js && wasm

package cosmos

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-3 }

func sameRGBA(got, want [4]float64) bool {
	for i := range got {
		if !near(got[i], want[i]) {
			return false
		}
	}
	return true
}

func TestParseHexColors(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want [4]float64
	}{
		{"#000000", [4]float64{0, 0, 0, 1}},
		{"#ffffff", [4]float64{1, 1, 1, 1}},
		{"#FF0000", [4]float64{1, 0, 0, 1}}, // case does not matter
		{"#00ff00", [4]float64{0, 1, 0, 1}},
		{"#0000ff", [4]float64{0, 0, 1, 1}},
		{"#808080", [4]float64{128.0 / 255, 128.0 / 255, 128.0 / 255, 1}},
		// Three digits are each doubled, so #f00 is #ff0000 rather than #0f0000.
		{"#f00", [4]float64{1, 0, 0, 1}},
		{"#fff", [4]float64{1, 1, 1, 1}},
		{"#abc", [4]float64{0xaa / 255.0, 0xbb / 255.0, 0xcc / 255.0, 1}},
		// Eight digits carry alpha.
		{"#ff000080", [4]float64{1, 0, 0, 128.0 / 255}},
		{"#00000000", [4]float64{0, 0, 0, 0}},
		// Whitespace is trimmed rather than making the color unparsable.
		{"  #ff0000  ", [4]float64{1, 0, 0, 1}},
	} {
		if got := parseRGBA(tc.in); !sameRGBA(got, tc.want) {
			t.Errorf("parseRGBA(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// A color that cannot be read is opaque black rather than an error: a graph
// with one bad color in its config should still draw.
func TestParseUnreadableColorsAreOpaqueBlack(t *testing.T) {
	for _, in := range []string{"", "   ", "#", "#zz", "#12345", "nonsense"} {
		got := parseRGBA(in)
		if !near(got[3], 1) {
			t.Errorf("parseRGBA(%q) = %v, want an opaque fallback", in, got)
		}
	}
}

func TestParseRGBFunctional(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want [4]float64
	}{
		{"rgb(255, 0, 0)", [4]float64{1, 0, 0, 1}},
		{"rgb(0,0,0)", [4]float64{0, 0, 0, 1}},
		{"rgba(255, 255, 255, 1)", [4]float64{1, 1, 1, 1}},
		{"rgba(255, 0, 0, 0.5)", [4]float64{1, 0, 0, 0.5}},
		{"rgba(0, 128, 255, 0)", [4]float64{0, 128.0 / 255, 1, 0}},
		{"RGB(255, 0, 0)", [4]float64{1, 0, 0, 1}}, // case does not matter
	} {
		if got := parseRGBA(tc.in); !sameRGBA(got, tc.want) {
			t.Errorf("parseRGBA(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseHSL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want [4]float64
	}{
		{"hsl(0, 100%, 50%)", [4]float64{1, 0, 0, 1}},
		{"hsl(120, 100%, 50%)", [4]float64{0, 1, 0, 1}},
		{"hsl(240, 100%, 50%)", [4]float64{0, 0, 1, 1}},
		{"hsl(0, 0%, 100%)", [4]float64{1, 1, 1, 1}},
		{"hsl(0, 0%, 0%)", [4]float64{0, 0, 0, 1}},
		{"hsla(0, 100%, 50%, 0.5)", [4]float64{1, 0, 0, 0.5}},
	} {
		got, ok := parseHSL(tc.in)
		if !ok {
			t.Errorf("parseHSL(%q) reported it could not read the color", tc.in)
			continue
		}
		if !sameRGBA(got, tc.want) {
			t.Errorf("parseHSL(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// parseHSL reads the parenthesized body and leaves the hsl prefix to its
// caller, so what it rejects is a body it cannot make three numbers out of —
// not a color that is some other notation.
func TestParseHSLRejectsABodyItCannotRead(t *testing.T) {
	for _, in := range []string{"", "#ff0000", "hsl(", "hsl()", "hsl(1)", "hsl(1,2)", "hsl(a,b,c)", "hsl)1,2,3("} {
		if _, ok := parseHSL(in); ok {
			t.Errorf("parseHSL(%q) claimed to read it", in)
		}
	}
}

// The space-separated form is what modern CSS writes, and the slash before
// alpha has to be treated as a separator rather than as part of a number.
func TestParseHSLAcceptsTheSpaceSeparatedForm(t *testing.T) {
	want := [4]float64{1, 0, 0, 1}
	for _, in := range []string{"hsl(0 100% 50%)", "hsl(0deg 100% 50%)"} {
		got, ok := parseHSL(in)
		if !ok {
			t.Errorf("parseHSL(%q) could not read it", in)
			continue
		}
		if !sameRGBA(got, want) {
			t.Errorf("parseHSL(%q) = %v, want %v", in, got, want)
		}
	}
	got, ok := parseHSL("hsl(0 100% 50% / 0.5)")
	if !ok {
		t.Fatal("the slash alpha form could not be read")
	}
	if !near(got[3], 0.5) {
		t.Errorf("alpha = %v, want 0.5", got[3])
	}
}

// Saturation zero is grey at the given lightness whatever the hue, which is
// the branch hslToRGB takes separately.
func TestHSLWithNoSaturationIsGrey(t *testing.T) {
	for _, hue := range []float64{0, 0.25, 0.5, 0.99} {
		r, g, b := hslToRGB(hue, 0, 0.5)
		if !near(r, 0.5) || !near(g, 0.5) || !near(b, 0.5) {
			t.Errorf("hue %v with no saturation gave %v,%v,%v, want grey", hue, r, g, b)
		}
	}
}

// Hue wraps: 360 degrees is the same color as 0.
func TestHueWrapsAround(t *testing.T) {
	a, _ := parseHSL("hsl(0, 100%, 50%)")
	b, _ := parseHSL("hsl(360, 100%, 50%)")
	if !sameRGBA(a, b) {
		t.Errorf("hue 0 gave %v and hue 360 gave %v", a, b)
	}
}

func TestMod(t *testing.T) {
	for _, tc := range []struct{ v, m, want float64 }{
		{5, 3, 2},
		{-1, 3, 2}, // negative wraps forward, which is what hue needs
		{3, 3, 0},
		{0, 3, 0},
		{-4, 3, 2},
	} {
		if got := mod(tc.v, tc.m); !near(got, tc.want) {
			t.Errorf("mod(%v, %v) = %v, want %v", tc.v, tc.m, got, tc.want)
		}
	}
}

func TestHexComponent(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want uint64
	}{
		{"00", 0}, {"ff", 255}, {"FF", 255}, {"80", 128}, {"0a", 10},
		{"zz", 0}, {"", 0}, {"-1", 0}, // unreadable is zero, as a browser does
	} {
		if got := hexComponent(tc.in); got != tc.want {
			t.Errorf("hexComponent(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// GetRgbaColor is the exported entry point; it must agree with the parser it
// wraps rather than drifting from it.
func TestGetRgbaColorMatchesTheParser(t *testing.T) {
	for _, in := range []string{"#ff0000", "rgb(0,255,0)", "hsl(240,100%,50%)", "nonsense"} {
		want := parseRGBA(in)
		got := GetRgbaColor(in)
		for i := range got {
			if !near(float64(got[i]), want[i]) {
				t.Errorf("GetRgbaColor(%q) = %v, parseRGBA gives %v", in, got, want)
				break
			}
		}
	}
}

// Every channel a color parser produces has to be inside 0..1, or the shader
// gets a value it cannot use.
func TestEveryParsedChannelIsInRange(t *testing.T) {
	for _, in := range []string{
		"#ff0000", "#abc", "#00000000", "rgb(255,255,255)", "rgba(0,0,0,0)",
		"hsl(180, 50%, 25%)", "hsla(300, 100%, 75%, 0.25)", "garbage", "",
	} {
		for i, v := range parseRGBA(in) {
			if v < 0 || v > 1 {
				t.Errorf("parseRGBA(%q) channel %d = %v, outside 0..1", in, i, v)
			}
		}
	}
}
