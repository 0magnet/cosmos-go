//go:build js && wasm

package cosmos

import "math"

// points is the port of the Points module.
type points struct {
	ctx  *glCtx
	cfg  *Config
	st   *store
	data *graphData

	currentPositionFbo  *framebuffer
	previousPositionFbo *framebuffer
	velocityFbo         *framebuffer
	selectedFbo         *framebuffer
	colorFbo            *framebuffer
	hoveredFbo          *framebuffer
	greyoutStatusFbo    *framebuffer
	sizeFbo             *framebuffer
	trackedIndicesFbo   *framebuffer
	trackedPositionsFbo *framebuffer
	sampledNodesFbo     *framebuffer

	drawCommand                      *command
	drawHighlightedCommand           *command
	updatePositionCommand            *command
	findPointsOnAreaSelectionCommand *command
	findHoveredPointCommand          *command
	clearHoveredFboCommand           *command
	clearSampledNodesFboCommand      *command
	fillSampledNodesFboCommand       *command
	trackPointsCommand               *command

	trackedIds           []string
	trackedPositionsByID map[string][2]float64
	sizeByIndex          []float64

	// props for the highlighted-ring command
	hlColor      [4]float64
	hlWidth      float64
	hlPointIndex int

	quadBuffer    *buffer
	indexesBuffer *buffer
}

func newPoints(ctx *glCtx, cfg *Config, st *store, data *graphData) *points {
	return &points{
		ctx:                  ctx,
		cfg:                  cfg,
		st:                   st,
		data:                 data,
		trackedPositionsByID: map[string][2]float64{},
		quadBuffer:           ctx.newBuffer([]float32{-1, -1, 1, -1, -1, 1, 1, 1}),
	}
}

func (p *points) createIndexesBuffer() {
	textureSize := p.st.pointsTextureSize
	indexes := make([]float32, textureSize*textureSize*2)
	for y := 0; y < textureSize; y++ {
		for x := 0; x < textureSize; x++ {
			i := y*textureSize*2 + x*2
			indexes[i] = float32(x)
			indexes[i+1] = float32(y)
		}
	}
	p.indexesBuffer.destroy()
	p.indexesBuffer = p.ctx.newBuffer(indexes)
}

func (p *points) create() {
	st, data, cfg := p.st, p.data, p.cfg
	pointsTextureSize := st.pointsTextureSize
	if pointsTextureSize == 0 {
		return
	}
	p.createIndexesBuffer()

	initialState := make([]float32, pointsTextureSize*pointsTextureSize*4)
	if !cfg.DisableSimulation {
		p.rescaleInitialNodePositions()
	}
	for i := range data.nodes {
		sortedIndex := data.getSortedIndexByInputIndex(i)
		node := &data.nodes[i]
		if sortedIndex >= 0 {
			x, y := node.X, node.Y
			if !node.HasPosition {
				x = st.adjustedSpaceSize * st.getRandomFloat(0.495, 0.505)
				y = st.adjustedSpaceSize * st.getRandomFloat(0.495, 0.505)
			}
			initialState[sortedIndex*4+0] = float32(x)
			initialState[sortedIndex*4+1] = float32(y)
		}
	}

	p.currentPositionFbo = p.ctx.newFramebuffer(pointsTextureSize, pointsTextureSize, initialState)

	if !cfg.DisableSimulation {
		p.previousPositionFbo = p.ctx.newFramebuffer(pointsTextureSize, pointsTextureSize, initialState)
		p.velocityFbo = p.ctx.newFramebuffer(pointsTextureSize, pointsTextureSize, nil)
	}

	p.selectedFbo = p.ctx.newFramebuffer(pointsTextureSize, pointsTextureSize, initialState)
	p.hoveredFbo = p.ctx.newFramebuffer(2, 2, nil)

	p.updateSize()
	p.updateColor()
	p.updateGreyoutStatus()
	p.updateSampledNodesGrid()
}

func (p *points) initPrograms() error {
	ctx, cfg, st, data := p.ctx, p.cfg, p.st, p.data

	quadAttr := []attrBinding{{name: "quad", buffer: func() *buffer { return p.quadBuffer }, size: 2}}
	indexAttr := []attrBinding{{name: "indexes", buffer: func() *buffer { return p.indexesBuffer }, size: 2}}

	if !cfg.DisableSimulation {
		prog, err := ctx.program(quadVert, updatePositionFrag)
		if err != nil {
			return err
		}
		p.updatePositionCommand = &command{
			ctx: ctx, prog: prog,
			fbo:       func() *framebuffer { return p.currentPositionFbo },
			primitive: "triangle strip",
			count:     func() int { return 4 },
			attrs:     quadAttr,
			uniforms: map[string]func() uniformValue{
				"position":  func() uniformValue { return p.previousPositionFbo },
				"velocity":  func() uniformValue { return p.velocityFbo },
				"friction":  func() uniformValue { return cfg.Simulation.Friction },
				"spaceSize": func() uniformValue { return st.adjustedSpaceSize },
			},
		}
	}

	prog, err := ctx.program(drawPointsVert, drawPointsFrag)
	if err != nil {
		return err
	}
	p.drawCommand = &command{
		ctx: ctx, prog: prog,
		primitive: "points",
		count:     func() int { return len(data.nodes) },
		attrs:     indexAttr,
		uniforms: map[string]func() uniformValue{
			"positions":             func() uniformValue { return p.currentPositionFbo },
			"particleColor":         func() uniformValue { return p.colorFbo },
			"particleGreyoutStatus": func() uniformValue { return p.greyoutStatusFbo },
			"particleSize":          func() uniformValue { return p.sizeFbo },
			"ratio":                 func() uniformValue { return cfg.PixelRatio },
			"sizeScale":             func() uniformValue { return cfg.NodeSizeScale },
			"pointsTextureSize":     func() uniformValue { return float64(st.pointsTextureSize) },
			"transform":             func() uniformValue { return st.transform[:] },
			"spaceSize":             func() uniformValue { return st.adjustedSpaceSize },
			"screenSize":            func() uniformValue { return st.screenSize[:] },
			"greyoutOpacity":        func() uniformValue { return cfg.NodeGreyoutOpacity },
			"scaleNodesOnZoom":      func() uniformValue { return cfg.ScaleNodesOnZoom },
			"maxPointSize":          func() uniformValue { return st.maxPointSize },
		},
		blend: "alpha", depthOff: true,
	}

	prog, err = ctx.program(quadVert, findPointsOnAreaSelectionFrag)
	if err != nil {
		return err
	}
	p.findPointsOnAreaSelectionCommand = &command{
		ctx: ctx, prog: prog,
		fbo:       func() *framebuffer { return p.selectedFbo },
		primitive: "triangle strip",
		count:     func() int { return 4 },
		attrs:     quadAttr,
		uniforms: map[string]func() uniformValue{
			"position":          func() uniformValue { return p.currentPositionFbo },
			"particleSize":      func() uniformValue { return p.sizeFbo },
			"spaceSize":         func() uniformValue { return st.adjustedSpaceSize },
			"screenSize":        func() uniformValue { return st.screenSize[:] },
			"sizeScale":         func() uniformValue { return cfg.NodeSizeScale },
			"transform":         func() uniformValue { return st.transform[:] },
			"ratio":             func() uniformValue { return cfg.PixelRatio },
			"selection[0]":      func() uniformValue { return st.selectedArea[0][:] },
			"selection[1]":      func() uniformValue { return st.selectedArea[1][:] },
			"scaleNodesOnZoom":  func() uniformValue { return cfg.ScaleNodesOnZoom },
			"maxPointSize":      func() uniformValue { return st.maxPointSize },
			"pointsTextureSize": func() uniformValue { return float64(st.pointsTextureSize) },
		},
	}

	prog, err = ctx.program(quadVert, clearFrag)
	if err != nil {
		return err
	}
	p.clearHoveredFboCommand = &command{
		ctx: ctx, prog: prog,
		fbo:       func() *framebuffer { return p.hoveredFbo },
		primitive: "triangle strip",
		count:     func() int { return 4 },
		attrs:     quadAttr,
		uniforms:  map[string]func() uniformValue{},
	}
	p.clearSampledNodesFboCommand = &command{
		ctx: ctx, prog: prog,
		fbo:       func() *framebuffer { return p.sampledNodesFbo },
		primitive: "triangle strip",
		count:     func() int { return 4 },
		attrs:     quadAttr,
		uniforms:  map[string]func() uniformValue{},
	}

	prog, err = ctx.program(findHoveredPointVert, findHoveredPointFrag)
	if err != nil {
		return err
	}
	p.findHoveredPointCommand = &command{
		ctx: ctx, prog: prog,
		fbo:       func() *framebuffer { return p.hoveredFbo },
		primitive: "points",
		count:     func() int { return len(data.nodes) },
		attrs:     indexAttr,
		uniforms: map[string]func() uniformValue{
			"position":          func() uniformValue { return p.currentPositionFbo },
			"particleSize":      func() uniformValue { return p.sizeFbo },
			"ratio":             func() uniformValue { return cfg.PixelRatio },
			"sizeScale":         func() uniformValue { return cfg.NodeSizeScale },
			"pointsTextureSize": func() uniformValue { return float64(st.pointsTextureSize) },
			"transform":         func() uniformValue { return st.transform[:] },
			"spaceSize":         func() uniformValue { return st.adjustedSpaceSize },
			"screenSize":        func() uniformValue { return st.screenSize[:] },
			"scaleNodesOnZoom":  func() uniformValue { return cfg.ScaleNodesOnZoom },
			"mousePosition":     func() uniformValue { return st.screenMousePosition[:] },
			"maxPointSize":      func() uniformValue { return st.maxPointSize },
		},
		depthOff: true,
	}

	prog, err = ctx.program(fillSampledNodesVert, fillSampledNodesFrag)
	if err != nil {
		return err
	}
	p.fillSampledNodesFboCommand = &command{
		ctx: ctx, prog: prog,
		fbo:       func() *framebuffer { return p.sampledNodesFbo },
		primitive: "points",
		count:     func() int { return len(data.nodes) },
		attrs:     indexAttr,
		uniforms: map[string]func() uniformValue{
			"position":          func() uniformValue { return p.currentPositionFbo },
			"pointsTextureSize": func() uniformValue { return float64(st.pointsTextureSize) },
			"transform":         func() uniformValue { return st.transform[:] },
			"spaceSize":         func() uniformValue { return st.adjustedSpaceSize },
			"screenSize":        func() uniformValue { return st.screenSize[:] },
		},
		depthOff: true,
	}

	prog, err = ctx.program(drawHighlightedVert, drawHighlightedFrag)
	if err != nil {
		return err
	}
	p.drawHighlightedCommand = &command{
		ctx: ctx, prog: prog,
		primitive: "triangle strip",
		count:     func() int { return 4 },
		attrs:     quadAttr,
		uniforms: map[string]func() uniformValue{
			"color":                 func() uniformValue { return p.hlColor[:] },
			"width":                 func() uniformValue { return p.hlWidth },
			"pointIndex":            func() uniformValue { return float64(p.hlPointIndex) },
			"positions":             func() uniformValue { return p.currentPositionFbo },
			"particleColor":         func() uniformValue { return p.colorFbo },
			"particleSize":          func() uniformValue { return p.sizeFbo },
			"sizeScale":             func() uniformValue { return cfg.NodeSizeScale },
			"pointsTextureSize":     func() uniformValue { return float64(st.pointsTextureSize) },
			"transform":             func() uniformValue { return st.transform[:] },
			"spaceSize":             func() uniformValue { return st.adjustedSpaceSize },
			"screenSize":            func() uniformValue { return st.screenSize[:] },
			"scaleNodesOnZoom":      func() uniformValue { return cfg.ScaleNodesOnZoom },
			"maxPointSize":          func() uniformValue { return st.maxPointSize },
			"particleGreyoutStatus": func() uniformValue { return p.greyoutStatusFbo },
			"greyoutOpacity":        func() uniformValue { return cfg.NodeGreyoutOpacity },
		},
		blend: "alpha", depthOff: true,
	}

	prog, err = ctx.program(quadVert, trackPositionsFrag)
	if err != nil {
		return err
	}
	p.trackPointsCommand = &command{
		ctx: ctx, prog: prog,
		fbo:       func() *framebuffer { return p.trackedPositionsFbo },
		primitive: "triangle strip",
		count:     func() int { return 4 },
		attrs:     quadAttr,
		uniforms: map[string]func() uniformValue{
			"position":          func() uniformValue { return p.currentPositionFbo },
			"trackedIndices":    func() uniformValue { return p.trackedIndicesFbo },
			"pointsTextureSize": func() uniformValue { return float64(st.pointsTextureSize) },
		},
	}
	return nil
}

func (p *points) updateColor() {
	textureSize := p.st.pointsTextureSize
	if textureSize == 0 {
		return
	}
	initialState := make([]float32, textureSize*textureSize*4)
	for i := range p.data.nodes {
		sortedIndex := p.data.getSortedIndexByInputIndex(i)
		node := &p.data.nodes[i]
		if sortedIndex >= 0 {
			colorStr := node.Color
			if colorStr == "" {
				colorStr = p.cfg.NodeColor
			}
			rgba := parseRGBA(colorStr)
			initialState[sortedIndex*4+0] = float32(rgba[0])
			initialState[sortedIndex*4+1] = float32(rgba[1])
			initialState[sortedIndex*4+2] = float32(rgba[2])
			initialState[sortedIndex*4+3] = float32(rgba[3])
		}
	}
	p.colorFbo.destroy()
	p.colorFbo = p.ctx.newFramebuffer(textureSize, textureSize, initialState)
}

func (p *points) updateGreyoutStatus() {
	textureSize := p.st.pointsTextureSize
	if textureSize == 0 {
		return
	}
	// Greyout status: 0 - false, highlighted or normal point; 1 - true
	initialState := make([]float32, textureSize*textureSize*4)
	if p.st.hasSelection {
		for i := range initialState {
			initialState[i] = 1
		}
		for _, selectedIndex := range p.st.selectedIndices {
			if selectedIndex >= 0 && selectedIndex*4 < len(initialState) {
				initialState[selectedIndex*4] = 0
			}
		}
	}
	p.greyoutStatusFbo.destroy()
	p.greyoutStatusFbo = p.ctx.newFramebuffer(textureSize, textureSize, initialState)
}

func (p *points) updateSize() {
	textureSize := p.st.pointsTextureSize
	if textureSize == 0 {
		return
	}
	p.sizeByIndex = make([]float64, len(p.data.nodes))
	initialState := make([]float32, textureSize*textureSize*4)
	for i := range p.data.nodes {
		sortedIndex := p.data.getSortedIndexByInputIndex(i)
		node := &p.data.nodes[i]
		if sortedIndex >= 0 {
			size := node.Size
			if size == 0 {
				size = p.cfg.NodeSize
			}
			initialState[sortedIndex*4] = float32(size)
			p.sizeByIndex[i] = size
		}
	}
	p.sizeFbo.destroy()
	p.sizeFbo = p.ctx.newFramebuffer(textureSize, textureSize, initialState)
}

func (p *points) updateSampledNodesGrid() {
	dist := p.cfg.NodeSamplingDistance
	if dist == 0 {
		dist = math.Min(p.st.screenSize[0], p.st.screenSize[1]) / 2
	}
	w := int(math.Ceil(p.st.screenSize[0] / dist))
	h := int(math.Ceil(p.st.screenSize[1] / dist))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	p.sampledNodesFbo.destroy()
	p.sampledNodesFbo = p.ctx.newFramebuffer(w, h, nil)
}

func (p *points) trackPoints() {
	if p.trackedIndicesFbo == nil || p.trackedPositionsFbo == nil {
		return
	}
	p.trackPointsCommand.run()
}

func (p *points) draw() {
	p.drawCommand.run()
	if p.cfg.RenderHoveredNodeRing && p.st.hoveredNode != nil {
		p.hlWidth = 0.85
		p.hlColor = p.st.hoveredNodeRingColor
		p.hlPointIndex = p.st.hoveredNode.index
		p.drawHighlightedCommand.run()
	}
	if p.st.focusedNode != nil {
		p.hlWidth = 0.75
		p.hlColor = p.st.focusedNodeRingColor
		p.hlPointIndex = p.st.focusedNode.index
		p.drawHighlightedCommand.run()
	}
}

func (p *points) updatePosition() {
	p.updatePositionCommand.run()
	p.swapFbo()
}

func (p *points) findPointsOnAreaSelection() {
	p.findPointsOnAreaSelectionCommand.run()
}

func (p *points) findHoveredPoint() {
	p.clearHoveredFboCommand.run()
	p.findHoveredPointCommand.run()
}

// getNodeRadiusByIndex returns the node radius by input index, or NaN.
func (p *points) getNodeRadiusByIndex(index int) float64 {
	if index < 0 || index >= len(p.sizeByIndex) {
		return math.NaN()
	}
	return p.sizeByIndex[index]
}

func (p *points) trackNodesByIds(ids []string) {
	p.trackedIds = ids
	p.trackedPositionsByID = map[string][2]float64{}
	var indices []int
	for _, id := range ids {
		if i := p.data.getSortedIndexByID(id); i >= 0 {
			indices = append(indices, i)
		}
	}
	p.trackedIndicesFbo.destroy()
	p.trackedIndicesFbo = nil
	p.trackedPositionsFbo.destroy()
	p.trackedPositionsFbo = nil
	if len(indices) > 0 {
		size := int(math.Ceil(math.Sqrt(float64(len(indices)))))
		initialState := make([]float32, size*size*4)
		for i := range initialState {
			initialState[i] = -1
		}
		for i, sortedIndex := range indices {
			initialState[i*4+0] = float32(sortedIndex % p.st.pointsTextureSize)
			initialState[i*4+1] = float32(sortedIndex / p.st.pointsTextureSize)
			initialState[i*4+2] = 0
			initialState[i*4+3] = 0
		}
		p.trackedIndicesFbo = p.ctx.newFramebuffer(size, size, initialState)
		p.trackedPositionsFbo = p.ctx.newFramebuffer(size, size, nil)
	}
	p.trackPoints()
}

func (p *points) getTrackedPositions() map[string][2]float64 {
	if len(p.trackedIds) == 0 {
		return p.trackedPositionsByID
	}
	pixels := p.trackedPositionsFbo.readPixels()
	for i, id := range p.trackedIds {
		if i*4+1 < len(pixels) {
			p.trackedPositionsByID[id] = [2]float64{float64(pixels[i*4]), float64(pixels[i*4+1])}
		}
	}
	return p.trackedPositionsByID
}

func (p *points) getSampledNodePositionsMap() map[string][2]float64 {
	positions := map[string][2]float64{}
	if p.sampledNodesFbo == nil {
		return positions
	}
	p.clearSampledNodesFboCommand.run()
	p.fillSampledNodesFboCommand.run()
	pixels := p.sampledNodesFbo.readPixels()
	for i := 0; i < len(pixels)/4; i++ {
		index := int(pixels[i*4])
		isNotEmpty := pixels[i*4+1] != 0
		x := float64(pixels[i*4+2])
		y := float64(pixels[i*4+3])
		if isNotEmpty {
			inputIndex := p.data.getInputIndexBySortedIndex(index)
			if node := p.data.getNodeByIndex(inputIndex); node != nil {
				positions[node.ID] = [2]float64{x, y}
			}
		}
	}
	return positions
}

func (p *points) destroy() {
	p.currentPositionFbo.destroy()
	p.previousPositionFbo.destroy()
	p.velocityFbo.destroy()
	p.selectedFbo.destroy()
	p.colorFbo.destroy()
	p.sizeFbo.destroy()
	p.greyoutStatusFbo.destroy()
	p.hoveredFbo.destroy()
	p.trackedIndicesFbo.destroy()
	p.trackedPositionsFbo.destroy()
}

func (p *points) swapFbo() {
	p.previousPositionFbo, p.currentPositionFbo = p.currentPositionFbo, p.previousPositionFbo
}

func (p *points) rescaleInitialNodePositions() {
	nodes := p.data.nodes
	spaceSize := float64(p.cfg.SpaceSize)
	if len(nodes) == 0 {
		return
	}
	var xs, ys []float64
	for i := range nodes {
		if nodes[i].HasPosition {
			xs = append(xs, nodes[i].X)
			ys = append(ys, nodes[i].Y)
		}
	}
	if len(xs) == 0 {
		return
	}
	minX, maxX, okX := extent(xs)
	minY, maxY, okY := extent(ys)
	if !okX || !okY {
		return
	}
	w := maxX - minX
	h := maxY - minY

	size := math.Max(w, h)
	dw := (size - w) / 2
	dh := (size - h) / 2

	scaleX := linearScale{d0: minX - dw, d1: maxX + dw, r0: 0, r1: spaceSize}
	scaleY := linearScale{d0: minY - dh, d1: maxY + dh, r0: 0, r1: spaceSize}
	for i := range nodes {
		if nodes[i].HasPosition {
			nodes[i].X = scaleX.scale(nodes[i].X)
			nodes[i].Y = scaleY.scale(nodes[i].Y)
		}
	}
}
