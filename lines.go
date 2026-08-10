//go:build js && wasm

package cosmos

import "math"

// lines is the port of the Lines module (instanced curve/line rendering).
type lines struct {
	ctx    *glCtx
	cfg    *Config
	st     *store
	data   *graphData
	points *points

	drawCurveCommand  *command
	colorBuffer       *buffer
	widthBuffer       *buffer
	arrowBuffer       *buffer
	pointsBuffer      *buffer
	curveLineGeometry [][2]float64
	curveLineBuffer   *buffer
}

func newLines(ctx *glCtx, cfg *Config, st *store, data *graphData, pts *points) *lines {
	return &lines{ctx: ctx, cfg: cfg, st: st, data: data, points: pts}
}

func (l *lines) create() {
	l.updateColor()
	l.updateWidth()
	l.updateArrow()
	l.updateCurveLineGeometry()
}

func (l *lines) initPrograms() error {
	ctx, cfg, st, data := l.ctx, l.cfg, l.st, l.data
	pointsTextureSize := st.pointsTextureSize

	instancePoints := make([]float32, 0, data.linksNumber()*4)
	for _, li := range data.completeLinks {
		link := &data.links[li]
		toIndex := data.getSortedIndexByID(link.Target)
		fromIndex := data.getSortedIndexByID(link.Source)
		fromX := fromIndex % pointsTextureSize
		fromY := fromIndex / pointsTextureSize
		toX := toIndex % pointsTextureSize
		toY := toIndex / pointsTextureSize
		instancePoints = append(instancePoints, float32(fromX), float32(fromY), float32(toX), float32(toY))
	}
	l.pointsBuffer.destroy()
	l.pointsBuffer = ctx.newBuffer(instancePoints)

	prog, err := ctx.program(drawLineVert, drawLineFrag)
	if err != nil {
		return err
	}
	l.drawCurveCommand = &command{
		ctx: ctx, prog: prog,
		primitive: "triangle strip",
		count:     func() int { return len(l.curveLineGeometry) },
		instances: func() int { return data.linksNumber() },
		attrs: []attrBinding{
			{name: "position", buffer: func() *buffer { return l.curveLineBuffer }, size: 2},
			{name: "pointA", buffer: func() *buffer { return l.pointsBuffer }, size: 2, offset: 0, stride: 16, divisor: 1},
			{name: "pointB", buffer: func() *buffer { return l.pointsBuffer }, size: 2, offset: 8, stride: 16, divisor: 1},
			{name: "color", buffer: func() *buffer { return l.colorBuffer }, size: 4, offset: 0, stride: 16, divisor: 1},
			{name: "width", buffer: func() *buffer { return l.widthBuffer }, size: 1, offset: 0, stride: 4, divisor: 1},
			{name: "arrow", buffer: func() *buffer { return l.arrowBuffer }, size: 1, offset: 0, stride: 4, divisor: 1},
		},
		uniforms: map[string]func() uniformValue{
			"positions":                      func() uniformValue { return l.points.currentPositionFbo },
			"particleGreyoutStatus":          func() uniformValue { return l.points.greyoutStatusFbo },
			"transform":                      func() uniformValue { return st.transform[:] },
			"pointsTextureSize":              func() uniformValue { return float64(st.pointsTextureSize) },
			"nodeSizeScale":                  func() uniformValue { return cfg.NodeSizeScale },
			"widthScale":                     func() uniformValue { return cfg.LinkWidthScale },
			"arrowSizeScale":                 func() uniformValue { return cfg.LinkArrowsSizeScale },
			"spaceSize":                      func() uniformValue { return st.adjustedSpaceSize },
			"screenSize":                     func() uniformValue { return st.screenSize[:] },
			"ratio":                          func() uniformValue { return cfg.PixelRatio },
			"linkVisibilityDistanceRange":    func() uniformValue { return cfg.LinkVisibilityDistanceRange[:] },
			"linkVisibilityMinTransparency":  func() uniformValue { return cfg.LinkVisibilityMinTransparency },
			"greyoutOpacity":                 func() uniformValue { return cfg.LinkGreyoutOpacity },
			"scaleNodesOnZoom":               func() uniformValue { return cfg.ScaleNodesOnZoom },
			"curvedWeight":                   func() uniformValue { return cfg.CurvedLinkWeight },
			"curvedLinkControlPointDistance": func() uniformValue { return cfg.CurvedLinkControlPointDistance },
			"curvedLinkSegments": func() uniformValue {
				if cfg.CurvedLinks {
					return float64(cfg.CurvedLinkSegments)
				}
				return 1.0
			},
		},
		blend: "alpha", depthOff: true, cullBack: true,
	}
	return nil
}

func (l *lines) draw() {
	if l.colorBuffer == nil || l.widthBuffer == nil || l.curveLineBuffer == nil {
		return
	}
	l.drawCurveCommand.run()
}

func (l *lines) updateColor() {
	data := make([]float32, 0, l.data.linksNumber()*4)
	for _, li := range l.data.completeLinks {
		link := &l.data.links[li]
		colorStr := link.Color
		if colorStr == "" {
			colorStr = l.cfg.LinkColor
		}
		rgba := parseRGBA(colorStr)
		data = append(data, float32(rgba[0]), float32(rgba[1]), float32(rgba[2]), float32(rgba[3]))
	}
	l.colorBuffer.destroy()
	l.colorBuffer = l.ctx.newBuffer(data)
}

func (l *lines) updateWidth() {
	data := make([]float32, 0, l.data.linksNumber())
	for _, li := range l.data.completeLinks {
		link := &l.data.links[li]
		width := link.Width
		if width == 0 {
			width = l.cfg.LinkWidth
		}
		data = append(data, float32(width))
	}
	l.widthBuffer.destroy()
	l.widthBuffer = l.ctx.newBuffer(data)
}

func (l *lines) updateArrow() {
	data := make([]float32, 0, l.data.linksNumber())
	for _, li := range l.data.completeLinks {
		link := &l.data.links[li]
		useArrow := l.cfg.LinkArrows
		switch link.Arrow {
		case ArrowOn:
			useArrow = true
		case ArrowOff:
			useArrow = false
		}
		if useArrow {
			data = append(data, 1)
		} else {
			data = append(data, 0)
		}
	}
	l.arrowBuffer.destroy()
	l.arrowBuffer = l.ctx.newBuffer(data)
}

// getCurveLineGeometry is the port of Lines/geometry.ts: a power-scale
// distributed triangle strip along the hodograph of the curve.
func getCurveLineGeometry(segments int) [][2]float64 {
	// d3 scalePow().exponent(2).range([0,1]).domain([-1,1]) maps
	// v → (sign(v)*|v|² + 1) / 2
	scale := func(v float64) float64 {
		p := math.Abs(v) * math.Abs(v)
		if v < 0 {
			p = -p
		}
		return (p + 1) / 2
	}
	hodographValues := make([]float64, 0, segments+1)
	for d := 0; d < segments; d++ {
		hodographValues = append(hodographValues, -0.5+float64(d)/float64(segments))
	}
	hodographValues = append(hodographValues, 0.5)
	result := make([][2]float64, len(hodographValues)*2)
	for i, d := range hodographValues {
		result[i*2] = [2]float64{scale(d * 2), 0.5}
		result[i*2+1] = [2]float64{scale(d * 2), -0.5}
	}
	return result
}

func (l *lines) updateCurveLineGeometry() {
	segments := 1
	if l.cfg.CurvedLinks {
		segments = l.cfg.CurvedLinkSegments
	}
	l.curveLineGeometry = getCurveLineGeometry(segments)
	flat := make([]float32, 0, len(l.curveLineGeometry)*2)
	for _, v := range l.curveLineGeometry {
		flat = append(flat, float32(v[0]), float32(v[1]))
	}
	l.curveLineBuffer.destroy()
	l.curveLineBuffer = l.ctx.newBuffer(flat)
}

func (l *lines) destroy() {
	l.colorBuffer.destroy()
	l.widthBuffer.destroy()
	l.arrowBuffer.destroy()
	l.curveLineBuffer.destroy()
	l.pointsBuffer.destroy()
}
