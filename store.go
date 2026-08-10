//go:build js && wasm

package cosmos

import "math"

const (
	alphaMin         = 0.001
	maxPointSizeCore = 64.0
)

type hoveredNode struct {
	node     *Node
	index    int // sorted index
	position [2]float64
}

type focusedNode struct {
	node  *Node
	index int // sorted index
}

// store is the port of the Store module: shared render/simulation state.
type store struct {
	pointsTextureSize   int
	linksTextureSize    int
	alpha               float64
	transform           mat3
	backgroundColor     [4]float64
	screenSize          [2]float64
	mousePosition       [2]float64
	screenMousePosition [2]float64
	selectedArea        [2][2]float64
	isSimulationRunning bool
	simulationProgress  float64
	selectedIndices     []int // nil = no selection
	hasSelection        bool
	maxPointSize        float64
	hoveredNode         *hoveredNode
	focusedNode         *focusedNode
	adjustedSpaceSize   float64

	hoveredNodeRingColor [4]float64
	focusedNodeRingColor [4]float64
	alphaTarget          float64
	scaleNodeX           linearScale
	scaleNodeY           linearScale
	random               *rng
}

func newStore() *store {
	return &store{
		alpha:                1,
		transform:            mat3Identity(),
		maxPointSize:         maxPointSizeCore,
		adjustedSpaceSize:    4096,
		hoveredNodeRingColor: [4]float64{1, 1, 1, hoveredNodeRingOpacity},
		focusedNodeRingColor: [4]float64{1, 1, 1, focusedNodeRingOpacity},
		random:               newRng(0),
	}
}

func (s *store) addRandomSeed(seed string) {
	s.random = newRng(seedFromString(seed))
}

func (s *store) getRandomFloat(min, max float64) float64 {
	return s.random.float(min, max)
}

// adjustSpaceSize reduces the space size if it exceeds the WebGL limits,
// without changing the config parameter.
func (s *store) adjustSpaceSize(configSpaceSize int, webglMaxTextureSize int) {
	if configSpaceSize >= webglMaxTextureSize {
		s.adjustedSpaceSize = float64(webglMaxTextureSize) / 2
		consoleWarn("The `SpaceSize` has been reduced due to WebGL limits")
	} else {
		s.adjustedSpaceSize = float64(configSpaceSize)
	}
}

func (s *store) updateScreenSize(width, height float64) {
	space := s.adjustedSpaceSize
	s.screenSize = [2]float64{width, height}
	s.scaleNodeX = linearScale{d0: 0, d1: space, r0: (width - space) / 2, r1: (width + space) / 2}
	s.scaleNodeY = linearScale{d0: space, d1: 0, r0: (height - space) / 2, r1: (height + space) / 2}
}

func (s *store) scaleX(x float64) float64 { return s.scaleNodeX.scale(x) }
func (s *store) scaleY(y float64) float64 { return s.scaleNodeY.scale(y) }

func (s *store) setHoveredNodeRingColor(color string) {
	c := parseRGBA(color)
	s.hoveredNodeRingColor[0] = c[0]
	s.hoveredNodeRingColor[1] = c[1]
	s.hoveredNodeRingColor[2] = c[2]
}

func (s *store) setFocusedNodeRingColor(color string) {
	c := parseRGBA(color)
	s.focusedNodeRingColor[0] = c[0]
	s.focusedNodeRingColor[1] = c[1]
	s.focusedNodeRingColor[2] = c[2]
}

func (s *store) setFocusedNode(node *Node, index int) {
	if node != nil && index >= 0 {
		s.focusedNode = &focusedNode{node: node, index: index}
	} else {
		s.focusedNode = nil
	}
}

func (s *store) addAlpha(decay float64) float64 {
	return (s.alphaTarget - s.alpha) * s.alphaDecay(decay)
}

func (s *store) alphaDecay(decay float64) float64 {
	return 1 - math.Pow(alphaMin, 1/decay)
}
