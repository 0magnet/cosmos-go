//go:build js && wasm

package cosmos

import "syscall/js"

// Node is one graph node. The zero value of X/Y means "no position": the
// simulation places the node randomly (near the space center). Set
// HasPosition to use X/Y as the initial (or fixed, when the simulation is
// disabled) position.
//
// Per-node Color/Size override the config defaults when non-zero; this
// replaces the accessor-function idiom of the original library.
type Node struct {
	ID          string
	X, Y        float64
	HasPosition bool
	Color       string  // "" → Config.NodeColor
	Size        float64 // 0 → Config.NodeSize
}

// ArrowMode controls the arrow of a single link.
type ArrowMode uint8

const (
	ArrowDefault ArrowMode = iota // use Config.LinkArrows
	ArrowOn
	ArrowOff
)

// Link is one graph link. Per-link Color/Width override the config
// defaults when non-zero.
type Link struct {
	Source, Target string
	Color          string  // "" → Config.LinkColor
	Width          float64 // 0 → Config.LinkWidth
	Arrow          ArrowMode
}

// SimulationSettings mirrors GraphSimulationSettings of the original.
type SimulationSettings struct {
	// Decay coefficient. Use smaller values if you want the simulation to
	// "cool down" slower. Default: 1000
	Decay float64
	// Gravity force coefficient. Default: 0
	Gravity float64
	// Centering to center mass force coefficient. Default: 0
	Center float64
	// Repulsion force coefficient. Default: 0.1
	Repulsion float64
	// Decreases / increases the detalization of the Many-Body force
	// calculations. When UseQuadtree is set, corresponds to the Barnes–Hut
	// approximation criterion. Default: 1.7
	RepulsionTheta float64
	// Barnes–Hut approximation depth (only with UseQuadtree). Default: 12
	RepulsionQuadtreeLevels int
	// Link spring force coefficient. Default: 1
	LinkSpring float64
	// Minimum link distance. Default: 2
	LinkDistance float64
	// Range of random link distance values. Default: [1, 1.2]
	LinkDistRandomVariationRange [2]float64
	// Repulsion from the mouse position (activated by holding the right
	// mouse button). Default: 2
	RepulsionFromMouse float64
	// Friction coefficient. Default: 0.85
	Friction float64

	OnStart   func()
	OnTick    func(alpha float64, hovered *Node, index int, position [2]float64)
	OnEnd     func()
	OnPause   func()
	OnRestart func()
}

// Events mirrors GraphEvents of the original. Node arguments are nil when
// no node is involved; index is the input (original) node index or -1.
type Events struct {
	OnClick         func(node *Node, index int, position [2]float64, event js.Value)
	OnMouseMove     func(node *Node, index int, position [2]float64, event js.Value)
	OnNodeMouseOver func(node *Node, index int, position [2]float64, event js.Value)
	OnNodeMouseOut  func(event js.Value)
	OnZoomStart     func(userDriven bool)
	OnZoom          func(userDriven bool)
	OnZoomEnd       func(userDriven bool)
}

// Config mirrors GraphConfigInterface of the original library.
// Use NewConfig to get one with default values.
type Config struct {
	// Do not run the simulation, just render the graph. Applied only on
	// initialization. Default: false
	DisableSimulation bool
	// Canvas background color. Default: "#222222"
	BackgroundColor string
	// Simulation space size (max 8192). Default: 4096
	SpaceSize int
	// Default node color. Default: "#b3b3b3"
	NodeColor string
	// Greyed out node opacity when a selection is active. Default: 0.1
	NodeGreyoutOpacity float64
	// Default node size in pixels. Default: 4
	NodeSize float64
	// Scale factor for node sizes. Default: 1
	NodeSizeScale float64
	// Turns ring rendering around a hovered node on / off. Default: true
	RenderHoveredNodeRing bool
	// Hovered node ring color. Default: "white"
	HoveredNodeRingColor string
	// Focused node ring color. Default: "white"
	FocusedNodeRingColor string
	// Turns link rendering on / off. Default: true
	RenderLinks bool
	// Default link color. Default: "#666666"
	LinkColor string
	// Greyed out link opacity when a selection is active. Default: 0.1
	LinkGreyoutOpacity float64
	// Default link width in pixels. Default: 1
	LinkWidth float64
	// Scale factor for link widths. Default: 1
	LinkWidthScale float64
	// Render links as curved lines. Default: false
	CurvedLinks bool
	// Number of segments in a curved line. Default: 19
	CurvedLinkSegments int
	// Weight affects the shape of the curve. Default: 0.8
	CurvedLinkWeight float64
	// Position of the control point of the curve on the normal from the
	// center of the line. Default: 0.5
	CurvedLinkControlPointDistance float64
	// Whether to display link arrows by default. Default: true
	LinkArrows bool
	// Scale factor for link arrow size. Default: 1
	LinkArrowsSizeScale float64
	// Minimum and maximum link visibility distance in pixels. Default: [50, 150]
	LinkVisibilityDistanceRange [2]float64
	// Transparency of a link at the maximum visibility distance. Default: 0.25
	LinkVisibilityMinTransparency float64
	// Use the classic quadtree algorithm for the Many-Body force. Applied
	// only on initialization. Default: false
	UseQuadtree bool

	Simulation SimulationSettings
	Events     Events

	// Show an FPS counter in the top right corner. Default: false
	ShowFPSMonitor bool
	// Canvas pixel ratio. Default: 2
	PixelRatio float64
	// Scale the nodes when zooming in or out. Default: true
	ScaleNodesOnZoom bool
	// Initial zoom level (0 = unset). Applied once on initialization.
	InitialZoomLevel float64
	// Disable zooming and panning. Default: false
	DisableZoom bool
	// Center and zoom the view to fit all nodes on first data. Default: true
	FitViewOnInit bool
	// Delay in milliseconds before fitting the view. Default: 250
	FitViewDelay float64
	// When FitViewOnInit is set, fit the view to show the nodes within this
	// rect [[left, bottom], [right, top]] in space coordinates (nil = all).
	FitViewByNodesInRect [][2]float64
	// Random seed for layout reproducibility ("" = non-deterministic).
	// Applied only on initialization.
	RandomSeed string
	// Node sampling distance in pixels for GetSampledNodePositionsMap.
	// Default: 150
	NodeSamplingDistance float64
}

// NewConfig returns a Config with the same defaults as the original library.
func NewConfig() *Config {
	return &Config{
		DisableSimulation:              false,
		BackgroundColor:                "#222222",
		SpaceSize:                      4096,
		NodeColor:                      "#b3b3b3",
		NodeGreyoutOpacity:             0.1,
		NodeSize:                       4,
		NodeSizeScale:                  1,
		RenderHoveredNodeRing:          true,
		HoveredNodeRingColor:           "white",
		FocusedNodeRingColor:           "white",
		RenderLinks:                    true,
		LinkColor:                      "#666666",
		LinkGreyoutOpacity:             0.1,
		LinkWidth:                      1,
		LinkWidthScale:                 1,
		CurvedLinks:                    false,
		CurvedLinkSegments:             19,
		CurvedLinkWeight:               0.8,
		CurvedLinkControlPointDistance: 0.5,
		LinkArrows:                     true,
		LinkArrowsSizeScale:            1,
		LinkVisibilityDistanceRange:    [2]float64{50, 150},
		LinkVisibilityMinTransparency:  0.25,
		UseQuadtree:                    false,
		Simulation: SimulationSettings{
			Decay:                        1000,
			Gravity:                      0,
			Center:                       0,
			Repulsion:                    0.1,
			RepulsionTheta:               1.7,
			RepulsionQuadtreeLevels:      12,
			LinkSpring:                   1,
			LinkDistance:                 2,
			LinkDistRandomVariationRange: [2]float64{1, 1.2},
			RepulsionFromMouse:           2,
			Friction:                     0.85,
		},
		ShowFPSMonitor:       false,
		PixelRatio:           2,
		ScaleNodesOnZoom:     true,
		DisableZoom:          false,
		FitViewOnInit:        true,
		FitViewDelay:         250,
		NodeSamplingDistance: 150,
	}
}

const (
	hoveredNodeRingOpacity = 0.7
	focusedNodeRingOpacity = 0.95
	defaultScaleToZoom     = 3.0
)
