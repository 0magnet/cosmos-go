//go:build js && wasm

// Package cosmos is a Go/WebAssembly port of @cosmograph/cosmos 1.6.1
// (https://github.com/cosmograph-org/cosmos), a GPU-accelerated
// force-directed graph layout and rendering library. Both the force
// simulation and the rendering run on the GPU in WebGL shaders (carried
// over verbatim from the original); this package drives them through
// syscall/js and compiles with the standard Go toolchain
// (GOOS=js GOARCH=wasm) as well as TinyGo (-target wasm).
package cosmos

import (
	"math"
	"syscall/js"
)

// Graph is the port of the cosmos Graph class.
type Graph struct {
	cfg    *Config
	data   *graphData
	canvas js.Value
	ctx    *glCtx

	st     *store
	points *points
	lines  *lines

	forceGravity      *forceGravity
	forceCenter       *forceCenter
	forceManyBody     manyBody
	forceLinkIncoming *forceLink
	forceLinkOutgoing *forceLink
	forceMouse        *forceMouse
	zoom              *zoomState
	fps               *fpsMonitor

	rafID                      js.Value
	rafCb                      js.Func
	isRightClickMouse          bool
	hasParticleSystemDestroyed bool
	currentEvent               js.Value

	findHoveredPointExecutionCount int
	isMouseOnCanvas                bool
	isFirstDataAfterInit           bool
	fitViewOnInitTimeoutID         js.Value

	funcs []js.Func
}

// New creates a cosmos Graph on the given canvas element. A nil config
// uses the defaults.
func New(canvas js.Value, cfg *Config) (*Graph, error) {
	if cfg == nil {
		cfg = NewConfig()
	}
	g := &Graph{
		cfg:                  cfg,
		data:                 newGraphData(),
		canvas:               canvas,
		st:                   newStore(),
		isFirstDataAfterInit: true,
	}

	w := canvas.Get("clientWidth").Float()
	h := canvas.Get("clientHeight").Float()

	canvas.Set("width", w*cfg.PixelRatio)
	canvas.Set("height", h*cfg.PixelRatio)
	// If the canvas element has no CSS width and height style, clientWidth /
	// clientHeight will always equal the width/height attributes; assume a
	// canvas CSS size of 100% to prevent resize feedback loops.
	style := canvas.Get("style")
	if style.Get("width").String() == "" && style.Get("height").String() == "" {
		style.Call("setProperty", "width", "100%")
		style.Call("setProperty", "height", "100%")
	}

	ctx, err := newGLCtx(canvas)
	if err != nil {
		return nil, err
	}
	g.ctx = ctx

	g.st.maxPointSize = ctx.maxPointSize / cfg.PixelRatio
	g.st.adjustSpaceSize(cfg.SpaceSize, ctx.maxTextureSize)
	g.st.updateScreenSize(w, h)

	g.zoom = newZoomState(g.st, cfg)
	g.zoom.onStart = func(sourceEvent js.Value) {
		g.currentEvent = sourceEvent
		userDriven := sourceEvent.Truthy()
		if cfg.Events.OnZoomStart != nil {
			cfg.Events.OnZoomStart(userDriven)
		}
	}
	g.zoom.onZoom = func(sourceEvent js.Value) {
		userDriven := sourceEvent.Truthy()
		if userDriven && sourceEvent.Get("offsetX").Truthy() {
			g.updateMousePosition(sourceEvent)
		}
		g.currentEvent = sourceEvent
		if cfg.Events.OnZoom != nil {
			cfg.Events.OnZoom(userDriven)
		}
	}
	g.zoom.onEnd = func(sourceEvent js.Value) {
		g.currentEvent = sourceEvent
		userDriven := sourceEvent.Truthy()
		if cfg.Events.OnZoomEnd != nil {
			cfg.Events.OnZoomEnd(userDriven)
		}
	}
	g.zoom.attach(canvas)
	if cfg.DisableZoom {
		g.zoom.wheelEnabled = false
	}
	initialZoom := cfg.InitialZoomLevel
	if initialZoom == 0 {
		initialZoom = 1
	}
	g.zoom.scaleTo(initialZoom, 0)

	g.listen(canvas, "mouseenter", func(js.Value) { g.isMouseOnCanvas = true })
	g.listen(canvas, "mouseleave", func(js.Value) { g.isMouseOnCanvas = false })
	g.listen(canvas, "click", g.onClick)
	g.listen(canvas, "mousemove", g.onMouseMove)
	g.listen(canvas, "contextmenu", func(event js.Value) { event.Call("preventDefault") })

	g.points = newPoints(ctx, cfg, g.st, g.data)
	g.lines = newLines(ctx, cfg, g.st, g.data, g.points)
	if !cfg.DisableSimulation {
		g.forceGravity = newForceGravity()
		g.forceCenter = newForceCenter()
		if cfg.UseQuadtree {
			g.forceManyBody = newForceManyBodyQuadtree()
		} else {
			g.forceManyBody = newForceManyBody()
		}
		g.forceLinkIncoming = newForceLink()
		g.forceLinkOutgoing = newForceLink()
		g.forceMouse = newForceMouse()
	}

	g.st.backgroundColor = parseRGBA(cfg.BackgroundColor)
	if cfg.HoveredNodeRingColor != "" {
		g.st.setHoveredNodeRingColor(cfg.HoveredNodeRingColor)
	}
	if cfg.FocusedNodeRingColor != "" {
		g.st.setFocusedNodeRingColor(cfg.FocusedNodeRingColor)
	}

	if cfg.ShowFPSMonitor {
		g.fps = newFPSMonitor(canvas)
	}

	if cfg.RandomSeed != "" {
		g.st.addRandomSeed(cfg.RandomSeed)
	}

	g.rafCb = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		now := 0.0
		if len(args) > 0 {
			now = args[0].Float()
		}
		g.onFrame(now)
		return nil
	})

	return g, nil
}

func (g *Graph) listen(target js.Value, event string, fn func(js.Value)) {
	f := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var ev js.Value
		if len(args) > 0 {
			ev = args[0]
		}
		fn(ev)
		return nil
	})
	g.funcs = append(g.funcs, f)
	target.Call("addEventListener", event, f)
}

// Progress is the simulation progress from 0 to 1.
func (g *Graph) Progress() float64 { return g.st.simulationProgress }

// IsSimulationRunning reports whether the simulation is running.
func (g *Graph) IsSimulationRunning() bool { return g.st.isSimulationRunning }

// MaxPointSize is the maximum gl.POINTS size the hardware can render.
func (g *Graph) MaxPointSize() float64 { return g.st.maxPointSize }

// Config returns the active configuration. Mutating returned values takes
// effect on the next frame for most rendering parameters; structural
// parameters (colors, sizes, widths, curve geometry) need the matching
// Update* method or SetData to be re-applied.
func (g *Graph) Config() *Config { return g.cfg }

// UpdateNodeColor re-applies node colors from the config and node data.
func (g *Graph) UpdateNodeColor() { g.points.updateColor() }

// UpdateNodeSize re-applies node sizes from the config and node data.
func (g *Graph) UpdateNodeSize() { g.points.updateSize() }

// UpdateLinkColor re-applies link colors from the config and link data.
func (g *Graph) UpdateLinkColor() { g.lines.updateColor() }

// UpdateLinkWidth re-applies link widths from the config and link data.
func (g *Graph) UpdateLinkWidth() { g.lines.updateWidth() }

// UpdateLinkArrows re-applies link arrow settings.
func (g *Graph) UpdateLinkArrows() { g.lines.updateArrow() }

// UpdateCurveLineGeometry re-applies the curved link geometry settings.
func (g *Graph) UpdateCurveLineGeometry() { g.lines.updateCurveLineGeometry() }

// UpdateBackgroundColor re-applies Config.BackgroundColor.
func (g *Graph) UpdateBackgroundColor() { g.st.backgroundColor = parseRGBA(g.cfg.BackgroundColor) }

// SetData passes the graph data. When runSimulation is false the
// simulation won't be started automatically.
func (g *Graph) SetData(nodes []Node, links []Link, runSimulation bool) {
	if len(nodes) == 0 && len(links) == 0 {
		g.destroyParticleSystem()
		bg := g.st.backgroundColor
		g.ctx.clearTarget(nil, bg[0], bg[1], bg[2], bg[3])
		return
	}
	g.data.setData(nodes, links)
	// if InitialZoomLevel is set there is no need to fit the view
	if g.isFirstDataAfterInit && g.cfg.FitViewOnInit && g.cfg.InitialZoomLevel == 0 {
		g.fitViewOnInitTimeoutID = setTimeout(func() {
			if g.cfg.FitViewByNodesInRect != nil {
				positions := make([][2]float64, len(g.cfg.FitViewByNodesInRect))
				copy(positions, g.cfg.FitViewByNodesInRect)
				g.setZoomTransformByNodePositions(positions, 0, math.NaN(), 0.1)
			} else {
				g.FitView(250, 0.1)
			}
		}, g.cfg.FitViewDelay)
	}
	g.isFirstDataAfterInit = false

	g.update(runSimulation)
}

// ZoomToNodeByID centers the view on a node and zooms in.
// Defaults: duration 700ms, scale 3, canZoomOut true.
func (g *Graph) ZoomToNodeByID(id string, duration float64, scale float64, canZoomOut bool) {
	node := g.data.getNodeByID(id)
	if node == nil {
		return
	}
	g.zoomToNode(node, duration, scale, canZoomOut)
}

// ZoomToNodeByIndex centers the view on a node by index and zooms in.
func (g *Graph) ZoomToNodeByIndex(index int, duration float64, scale float64, canZoomOut bool) {
	node := g.data.getNodeByIndex(index)
	if node == nil {
		return
	}
	g.zoomToNode(node, duration, scale, canZoomOut)
}

// Zoom zooms the view to the given zoom level.
func (g *Graph) Zoom(value float64, duration float64) {
	g.SetZoomLevel(value, duration)
}

// SetZoomLevel zooms the view to the given zoom level.
func (g *Graph) SetZoomLevel(value float64, duration float64) {
	g.zoom.scaleTo(value, duration)
}

// GetZoomLevel returns the zoom level of the view.
func (g *Graph) GetZoomLevel() float64 { return g.zoom.eventTransform.k }

// GetNodePositions returns the current X and Y coordinates of all nodes,
// keyed by node id.
func (g *Graph) GetNodePositions() map[string][2]float64 {
	result := map[string][2]float64{}
	if g.hasParticleSystemDestroyed {
		return result
	}
	pixels := g.points.currentPositionFbo.readPixels()
	for i := range g.data.nodes {
		index := g.data.getSortedIndexByInputIndex(i)
		if index >= 0 && index*4+1 < len(pixels) {
			result[g.data.nodes[i].ID] = [2]float64{float64(pixels[index*4]), float64(pixels[index*4+1])}
		}
	}
	return result
}

// GetNodePositionsArray returns the coordinates of all nodes in input order.
func (g *Graph) GetNodePositionsArray() [][2]float64 {
	if g.hasParticleSystemDestroyed {
		return nil
	}
	pixels := g.points.currentPositionFbo.readPixels()
	positions := make([][2]float64, len(g.data.nodes))
	for i := range g.data.nodes {
		index := g.data.getSortedIndexByInputIndex(i)
		if index >= 0 && index*4+1 < len(pixels) {
			positions[i] = [2]float64{float64(pixels[index*4]), float64(pixels[index*4+1])}
		}
	}
	return positions
}

// FitView centers and zooms the view to fit all nodes in the scene.
// Defaults: duration 250ms, padding 0.1.
func (g *Graph) FitView(duration float64, padding float64) {
	g.setZoomTransformByNodePositions(g.GetNodePositionsArray(), duration, math.NaN(), padding)
}

// FitViewByNodeIDs centers and zooms the view to fit the given nodes.
func (g *Graph) FitViewByNodeIDs(ids []string, duration float64, padding float64) {
	positionsMap := g.GetNodePositions()
	var positions [][2]float64
	for _, id := range ids {
		if p, ok := positionsMap[id]; ok {
			positions = append(positions, p)
		}
	}
	g.setZoomTransformByNodePositions(positions, duration, math.NaN(), padding)
}

// SelectNodesInRange selects nodes inside a rectangular area defined by two
// corner points [[left, top], [right, bottom]] in screen coordinates.
// Passing ok=false clears the selection.
func (g *Graph) SelectNodesInRange(selection [2][2]float64, ok bool) {
	if ok {
		h := g.st.screenSize[1]
		g.st.selectedArea = [2][2]float64{
			{selection[0][0], h - selection[1][1]},
			{selection[1][0], h - selection[0][1]},
		}
		g.points.findPointsOnAreaSelection()
		pixels := g.points.selectedFbo.readPixels()
		g.st.selectedIndices = g.st.selectedIndices[:0]
		for i := 0; i < len(pixels); i += 4 {
			if pixels[i] != 0 {
				g.st.selectedIndices = append(g.st.selectedIndices, i/4)
			}
		}
		g.st.hasSelection = true
	} else {
		g.st.selectedIndices = nil
		g.st.hasSelection = false
	}
	g.points.updateGreyoutStatus()
}

// SelectNodeByID selects a node; optionally its adjacent nodes too.
func (g *Graph) SelectNodeByID(id string, selectAdjacentNodes bool) {
	if selectAdjacentNodes {
		adjacent := g.data.getAdjacentNodes(id)
		ids := []string{id}
		for _, n := range adjacent {
			ids = append(ids, n.ID)
		}
		g.SelectNodesByIDs(ids)
	} else {
		g.SelectNodesByIDs([]string{id})
	}
}

// SelectNodeByIndex selects a node by index; optionally adjacent nodes too.
func (g *Graph) SelectNodeByIndex(index int, selectAdjacentNodes bool) {
	node := g.data.getNodeByIndex(index)
	if node != nil {
		g.SelectNodeByID(node.ID, selectAdjacentNodes)
	}
}

// SelectNodesByIDs selects multiple nodes by id (nil clears the selection).
func (g *Graph) SelectNodesByIDs(ids []string) {
	if ids == nil {
		g.st.selectedIndices = nil
		g.st.hasSelection = false
	} else {
		g.st.selectedIndices = g.st.selectedIndices[:0]
		for _, id := range ids {
			if i := g.data.getSortedIndexByID(id); i >= 0 {
				g.st.selectedIndices = append(g.st.selectedIndices, i)
			}
		}
		g.st.hasSelection = true
	}
	g.points.updateGreyoutStatus()
}

// SelectNodesByIndices selects multiple nodes by input index (nil clears).
func (g *Graph) SelectNodesByIndices(indices []int) {
	if indices == nil {
		g.st.selectedIndices = nil
		g.st.hasSelection = false
	} else {
		g.st.selectedIndices = g.st.selectedIndices[:0]
		for _, index := range indices {
			if i := g.data.getSortedIndexByInputIndex(index); i >= 0 {
				g.st.selectedIndices = append(g.st.selectedIndices, i)
			}
		}
		g.st.hasSelection = true
	}
	g.points.updateGreyoutStatus()
}

// UnselectNodes unselects all nodes.
func (g *Graph) UnselectNodes() {
	g.st.selectedIndices = nil
	g.st.hasSelection = false
	g.points.updateGreyoutStatus()
}

// GetSelectedNodes returns the currently selected nodes (nil = none).
func (g *Graph) GetSelectedNodes() []*Node {
	if !g.st.hasSelection {
		return nil
	}
	nodes := make([]*Node, 0, len(g.st.selectedIndices))
	for _, selectedIndex := range g.st.selectedIndices {
		index := g.data.getInputIndexBySortedIndex(selectedIndex)
		if node := g.data.getNodeByIndex(index); node != nil {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// GetAdjacentNodes returns nodes adjacent to a node by its id.
func (g *Graph) GetAdjacentNodes(id string) []*Node {
	return g.data.getAdjacentNodes(id)
}

// SetFocusedNodeByID highlights a ring around the node ("" resets focus).
func (g *Graph) SetFocusedNodeByID(id string) {
	if id == "" {
		g.st.setFocusedNode(nil, -1)
	} else {
		g.st.setFocusedNode(g.data.getNodeByID(id), g.data.getSortedIndexByID(id))
	}
}

// SetFocusedNodeByIndex highlights a ring around the node by input index
// (negative resets focus).
func (g *Graph) SetFocusedNodeByIndex(index int) {
	if index < 0 {
		g.st.setFocusedNode(nil, -1)
	} else {
		g.st.setFocusedNode(g.data.getNodeByIndex(index), g.data.getSortedIndexByInputIndex(index))
	}
}

// SpaceToScreenPosition converts X, Y from space to screen coordinates.
func (g *Graph) SpaceToScreenPosition(spacePosition [2]float64) [2]float64 {
	return g.zoom.convertSpaceToScreenPosition(spacePosition)
}

// SpaceToScreenRadius converts a radius from space to screen units.
func (g *Graph) SpaceToScreenRadius(spaceRadius float64) float64 {
	return g.zoom.convertSpaceToScreenRadius(spaceRadius)
}

// GetNodeRadiusByIndex returns the node radius by input index (NaN if
// unknown).
func (g *Graph) GetNodeRadiusByIndex(index int) float64 {
	return g.points.getNodeRadiusByIndex(index)
}

// GetNodeRadiusByID returns the node radius by id (NaN if unknown).
func (g *Graph) GetNodeRadiusByID(id string) float64 {
	index := g.data.getInputIndexByID(id)
	if index < 0 {
		return math.NaN()
	}
	return g.points.getNodeRadiusByIndex(index)
}

// TrackNodePositionsByIDs tracks node positions by id on each tick.
func (g *Graph) TrackNodePositionsByIDs(ids []string) {
	g.points.trackNodesByIds(ids)
}

// TrackNodePositionsByIndices tracks node positions by input index.
func (g *Graph) TrackNodePositionsByIndices(indices []int) {
	var ids []string
	for _, index := range indices {
		if node := g.data.getNodeByIndex(index); node != nil {
			ids = append(ids, node.ID)
		}
	}
	g.points.trackNodesByIds(ids)
}

// GetTrackedNodePositionsMap returns the current tracked node positions.
func (g *Graph) GetTrackedNodePositionsMap() map[string][2]float64 {
	return g.points.getTrackedPositions()
}

// GetSampledNodePositionsMap returns a spatially sampled subset of the
// nodes currently visible on screen with their positions.
func (g *Graph) GetSampledNodePositionsMap() map[string][2]float64 {
	return g.points.getSampledNodePositionsMap()
}

// Start starts the simulation with the given alpha (0 to 1; higher =
// more initial energy).
func (g *Graph) Start(alpha float64) {
	if len(g.data.nodes) == 0 {
		return
	}
	g.st.isSimulationRunning = true
	g.st.alpha = alpha
	g.st.simulationProgress = 0
	if g.cfg.Simulation.OnStart != nil {
		g.cfg.Simulation.OnStart()
	}
	g.stopFrames()
	g.frame()
}

// Pause pauses the simulation.
func (g *Graph) Pause() {
	g.st.isSimulationRunning = false
	if g.cfg.Simulation.OnPause != nil {
		g.cfg.Simulation.OnPause()
	}
}

// Restart restarts the simulation.
func (g *Graph) Restart() {
	g.st.isSimulationRunning = true
	if g.cfg.Simulation.OnRestart != nil {
		g.cfg.Simulation.OnRestart()
	}
}

// Step renders one frame of the simulation and stops it.
func (g *Graph) Step() {
	g.st.isSimulationRunning = false
	g.stopFrames()
	g.frame()
}

// Destroy destroys the Graph instance.
func (g *Graph) Destroy() {
	if g.fitViewOnInitTimeoutID.Truthy() {
		js.Global().Call("clearTimeout", g.fitViewOnInitTimeoutID)
	}
	g.stopFrames()
	g.destroyParticleSystem()
	if g.fps != nil {
		g.fps.destroy()
		g.fps = nil
	}
}

func (g *Graph) create() {
	g.points.create()
	g.lines.create()
	if g.forceManyBody != nil {
		g.forceManyBody.create(g.ctx, g.st)
	}
	if g.forceLinkIncoming != nil {
		g.forceLinkIncoming.create(g.ctx, g.st, g.data, linkIncoming)
	}
	if g.forceLinkOutgoing != nil {
		g.forceLinkOutgoing.create(g.ctx, g.st, g.data, linkOutgoing)
	}
	if g.forceCenter != nil {
		g.forceCenter.create(g.ctx)
	}
	g.hasParticleSystemDestroyed = false
}

func (g *Graph) destroyParticleSystem() {
	if g.hasParticleSystemDestroyed {
		return
	}
	g.points.destroy()
	g.lines.destroy()
	if g.forceCenter != nil {
		g.forceCenter.destroy()
	}
	if g.forceLinkIncoming != nil {
		g.forceLinkIncoming.destroy()
	}
	if g.forceLinkOutgoing != nil {
		g.forceLinkOutgoing.destroy()
	}
	if g.forceManyBody != nil {
		g.forceManyBody.destroy()
	}
	g.hasParticleSystemDestroyed = true
}

func (g *Graph) update(runSimulation bool) {
	g.st.pointsTextureSize = int(math.Ceil(math.Sqrt(float64(len(g.data.nodes)))))
	g.st.linksTextureSize = int(math.Ceil(math.Sqrt(float64(g.data.linksNumber() * 2))))
	g.destroyParticleSystem()
	g.create()
	if err := g.initPrograms(); err != nil {
		consoleWarn(err.Error())
	}
	g.SetFocusedNodeByID("")
	g.st.hoveredNode = nil
	if runSimulation {
		g.Start(1)
	} else {
		g.Step()
	}
}

func (g *Graph) initPrograms() error {
	quadAttr := []attrBinding{{name: "quad", buffer: func() *buffer { return g.points.quadBuffer }, size: 2}}
	indexAttr := []attrBinding{{name: "indexes", buffer: func() *buffer { return g.points.indexesBuffer }, size: 2}}

	if err := g.points.initPrograms(); err != nil {
		return err
	}
	if err := g.lines.initPrograms(); err != nil {
		return err
	}
	if g.forceGravity != nil {
		if err := g.forceGravity.initPrograms(g.ctx, g.cfg, g.st, g.points, quadAttr); err != nil {
			return err
		}
	}
	if g.forceLinkIncoming != nil {
		if err := g.forceLinkIncoming.initPrograms(g.ctx, g.cfg, g.st, g.points, quadAttr); err != nil {
			return err
		}
	}
	if g.forceLinkOutgoing != nil {
		if err := g.forceLinkOutgoing.initPrograms(g.ctx, g.cfg, g.st, g.points, quadAttr); err != nil {
			return err
		}
	}
	if g.forceMouse != nil {
		if err := g.forceMouse.initPrograms(g.ctx, g.cfg, g.st, g.points, quadAttr); err != nil {
			return err
		}
	}
	if g.forceManyBody != nil {
		if err := g.forceManyBody.initPrograms(g.ctx, g.cfg, g.st, g.data, g.points, quadAttr, indexAttr); err != nil {
			return err
		}
	}
	if g.forceCenter != nil {
		if err := g.forceCenter.initPrograms(g.ctx, g.cfg, g.st, g.data, g.points, quadAttr, indexAttr); err != nil {
			return err
		}
	}
	return nil
}

func (g *Graph) frame() {
	if g.st.alpha < alphaMin && g.st.isSimulationRunning {
		g.end()
	}
	if g.st.pointsTextureSize == 0 {
		return
	}
	g.rafID = js.Global().Call("requestAnimationFrame", g.rafCb)
}

func (g *Graph) onFrame(now float64) {
	cfg, st := g.cfg, g.st
	if g.fps != nil {
		g.fps.frame(now)
	}
	g.resizeCanvas(false)
	g.zoom.tick(now)
	g.findHoveredPoint()

	if !cfg.DisableSimulation {
		if g.isRightClickMouse {
			if !st.isSimulationRunning {
				g.Start(0.1)
			}
			g.forceMouse.run()
			g.points.updatePosition()
		}

		if st.isSimulationRunning && !g.zoom.isRunning {
			if cfg.Simulation.Gravity != 0 {
				g.forceGravity.run()
				g.points.updatePosition()
			}

			if cfg.Simulation.Center != 0 {
				g.forceCenter.run()
				g.points.updatePosition()
			}

			g.forceManyBody.run()
			g.points.updatePosition()

			if st.linksTextureSize > 0 {
				g.forceLinkIncoming.run()
				g.points.updatePosition()
				g.forceLinkOutgoing.run()
				g.points.updatePosition()
			}

			st.alpha += st.addAlpha(cfg.Simulation.Decay)
			if g.isRightClickMouse {
				st.alpha = math.Max(st.alpha, 0.1)
			}
			st.simulationProgress = math.Sqrt(math.Min(1, alphaMin/st.alpha))
			if cfg.Simulation.OnTick != nil {
				var hovered *Node
				index := -1
				var position [2]float64
				if st.hoveredNode != nil {
					hovered = st.hoveredNode.node
					index = g.data.getInputIndexBySortedIndex(st.hoveredNode.index)
					position = st.hoveredNode.position
				}
				cfg.Simulation.OnTick(st.alpha, hovered, index, position)
			}
		}

		g.points.trackPoints()
	}

	// clear canvas
	bg := st.backgroundColor
	g.ctx.clearTarget(nil, bg[0], bg[1], bg[2], bg[3])

	if cfg.RenderLinks && st.linksTextureSize > 0 {
		g.lines.draw()
	}

	g.points.draw()

	g.currentEvent = js.Value{}
	g.frame()
}

func (g *Graph) stopFrames() {
	if g.rafID.Truthy() {
		js.Global().Call("cancelAnimationFrame", g.rafID)
	}
}

func (g *Graph) end() {
	g.st.isSimulationRunning = false
	g.st.simulationProgress = 1
	if g.cfg.Simulation.OnEnd != nil {
		g.cfg.Simulation.OnEnd()
	}
}

func (g *Graph) onClick(event js.Value) {
	if g.cfg.Events.OnClick != nil {
		var node *Node
		index := -1
		var position [2]float64
		if g.st.hoveredNode != nil {
			node = g.st.hoveredNode.node
			index = g.data.getInputIndexBySortedIndex(g.st.hoveredNode.index)
			position = g.st.hoveredNode.position
		}
		g.cfg.Events.OnClick(node, index, position, event)
	}
}

func (g *Graph) updateMousePosition(event js.Value) {
	if !event.Truthy() || event.Get("offsetX").IsUndefined() {
		return
	}
	mouseX := event.Get("offsetX").Float()
	mouseY := event.Get("offsetY").Float()
	g.st.mousePosition = g.zoom.convertScreenToSpacePosition([2]float64{mouseX, mouseY})
	g.st.screenMousePosition = [2]float64{mouseX, g.st.screenSize[1] - mouseY}
}

func (g *Graph) onMouseMove(event js.Value) {
	g.currentEvent = event
	g.updateMousePosition(event)
	g.isRightClickMouse = event.Get("which").Int() == 3
	if g.cfg.Events.OnMouseMove != nil {
		var node *Node
		index := -1
		var position [2]float64
		if g.st.hoveredNode != nil {
			node = g.st.hoveredNode.node
			index = g.data.getInputIndexBySortedIndex(g.st.hoveredNode.index)
			position = g.st.hoveredNode.position
		}
		g.cfg.Events.OnMouseMove(node, index, position, g.currentEvent)
	}
}

func (g *Graph) resizeCanvas(forceResize bool) {
	prevWidth := g.canvas.Get("width").Float()
	prevHeight := g.canvas.Get("height").Float()
	w := g.canvas.Get("clientWidth").Float()
	h := g.canvas.Get("clientHeight").Float()

	if forceResize || prevWidth != w*g.cfg.PixelRatio || prevHeight != h*g.cfg.PixelRatio {
		prevW := g.st.screenSize[0]
		prevH := g.st.screenSize[1]
		k := g.zoom.eventTransform.k
		centerPosition := g.zoom.convertScreenToSpacePosition([2]float64{prevW / 2, prevH / 2})

		g.st.updateScreenSize(w, h)
		g.canvas.Set("width", w*g.cfg.PixelRatio)
		g.canvas.Set("height", h*g.cfg.PixelRatio)
		transform := g.zoom.getTransform([][2]float64{centerPosition}, k, true, 0.1)
		g.zoom.transformTo(transform, 0, nil)
		g.points.updateSampledNodesGrid()
	}
}

func (g *Graph) setZoomTransformByNodePositions(positions [][2]float64, duration float64, scale float64, padding float64) {
	g.resizeCanvas(false)
	transform := g.zoom.getTransform(positions, scale, !math.IsNaN(scale), padding)
	g.zoom.transformTo(transform, duration, easeQuadInOut)
}

func (g *Graph) zoomToNode(node *Node, duration float64, scale float64, canZoomOut bool) {
	pixels := g.points.currentPositionFbo.readPixels()
	nodeIndex := g.data.getSortedIndexByID(node.ID)
	if nodeIndex < 0 || nodeIndex*4+1 >= len(pixels) {
		return
	}
	posX := float64(pixels[nodeIndex*4])
	posY := float64(pixels[nodeIndex*4+1])
	distance := g.zoom.getDistanceToPoint([2]float64{posX, posY})
	zoomLevel := scale
	if !canZoomOut {
		zoomLevel = math.Max(g.GetZoomLevel(), scale)
	}
	if distance < math.Min(g.st.screenSize[0], g.st.screenSize[1]) {
		g.setZoomTransformByNodePositions([][2]float64{{posX, posY}}, duration, zoomLevel, 0.1)
	} else {
		transform := g.zoom.getTransform([][2]float64{{posX, posY}}, zoomLevel, true, 0.1)
		middle := g.zoom.getMiddlePointTransform([2]float64{posX, posY})
		g.zoom.transformChain(
			[]zoomTransform{middle, transform},
			[]float64{duration / 2, duration / 2},
			[]func(float64) float64{easeQuadIn, easeQuadOut},
		)
	}
}

// DisableZoom turns wheel zooming off (panning stays available, matching
// the original behavior).
func (g *Graph) DisableZoom() { g.zoom.wheelEnabled = false }

// EnableZoom turns wheel zooming back on.
func (g *Graph) EnableZoom() { g.zoom.wheelEnabled = true }

func (g *Graph) findHoveredPoint() {
	if !g.isMouseOnCanvas {
		return
	}
	if g.findHoveredPointExecutionCount < 2 {
		g.findHoveredPointExecutionCount++
		return
	}
	g.findHoveredPointExecutionCount = 0
	g.points.findHoveredPoint()
	isMouseover := false
	isMouseout := false
	pixels := g.points.hoveredFbo.readPixels()
	nodeSize := float64(pixels[1])
	if nodeSize != 0 {
		index := int(pixels[0])
		inputIndex := g.data.getInputIndexBySortedIndex(index)
		hovered := g.data.getNodeByIndex(inputIndex)
		if g.st.hoveredNode == nil || g.st.hoveredNode.node != hovered {
			isMouseover = true
		}
		pointX := float64(pixels[2])
		pointY := float64(pixels[3])
		if hovered != nil {
			g.st.hoveredNode = &hoveredNode{node: hovered, index: index, position: [2]float64{pointX, pointY}}
		} else {
			g.st.hoveredNode = nil
		}
	} else {
		if g.st.hoveredNode != nil {
			isMouseout = true
		}
		g.st.hoveredNode = nil
	}

	if isMouseover && g.st.hoveredNode != nil {
		if g.cfg.Events.OnNodeMouseOver != nil {
			g.cfg.Events.OnNodeMouseOver(
				g.st.hoveredNode.node,
				g.data.getInputIndexBySortedIndex(g.st.hoveredNode.index),
				g.st.hoveredNode.position,
				g.currentEvent,
			)
		}
	}
	if isMouseout && g.cfg.Events.OnNodeMouseOut != nil {
		g.cfg.Events.OnNodeMouseOut(g.currentEvent)
	}
}

func setTimeout(fn func(), ms float64) js.Value {
	var f js.Func
	f = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fn()
		f.Release()
		return nil
	})
	return js.Global().Call("setTimeout", f, ms)
}
