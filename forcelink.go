//go:build js && wasm

package cosmos

import "math"

type linkDirection int

const (
	linkIncoming linkDirection = iota
	linkOutgoing
)

// forceLink is the port of the ForceLink module (spring force).
type forceLink struct {
	linkFirstIndicesAndAmountFbo *framebuffer
	indicesFbo                   *framebuffer
	biasAndStrengthFbo           *framebuffer
	randomDistanceFbo            *framebuffer
	maxPointDegree               int
	runCommand                   *command
}

func newForceLink() *forceLink { return &forceLink{} }

func (f *forceLink) create(ctx *glCtx, st *store, data *graphData, direction linkDirection) {
	pointsTextureSize, linksTextureSize := st.pointsTextureSize, st.linksTextureSize
	if pointsTextureSize == 0 || linksTextureSize == 0 {
		return
	}
	linkFirstIndicesAndAmount := make([]float32, pointsTextureSize*pointsTextureSize*4)
	indices := make([]float32, linksTextureSize*linksTextureSize*4)
	linkBiasAndStrengthState := make([]float32, linksTextureSize*linksTextureSize*4)
	linkDistanceState := make([]float32, linksTextureSize*linksTextureSize*4)

	grouped := data.groupedSourceToTarget
	keys := data.groupedSourceKeys
	if direction == linkOutgoing {
		grouped = data.groupedTargetToSource
		keys = data.groupedTargetKeys
	}
	f.maxPointDegree = 0
	linkIndex := 0
	for _, nodeIndex := range keys {
		connectedNodeIndices := grouped[nodeIndex]
		linkFirstIndicesAndAmount[nodeIndex*4+0] = float32(linkIndex % linksTextureSize)
		linkFirstIndicesAndAmount[nodeIndex*4+1] = float32(linkIndex / linksTextureSize)
		linkFirstIndicesAndAmount[nodeIndex*4+2] = float32(len(connectedNodeIndices))

		for _, connectedNodeIndex := range connectedNodeIndices {
			indices[linkIndex*4+0] = float32(connectedNodeIndex % pointsTextureSize)
			indices[linkIndex*4+1] = float32(connectedNodeIndex / pointsTextureSize)
			degree := data.degree[data.getInputIndexBySortedIndex(connectedNodeIndex)]
			connectedDegree := data.degree[data.getInputIndexBySortedIndex(nodeIndex)]
			bias := float64(degree) / float64(degree+connectedDegree)
			strength := 1 / math.Min(float64(degree), float64(connectedDegree))
			strength = math.Sqrt(strength)
			linkBiasAndStrengthState[linkIndex*4+0] = float32(bias)
			linkBiasAndStrengthState[linkIndex*4+1] = float32(strength)
			linkDistanceState[linkIndex*4] = float32(st.getRandomFloat(0, 1))

			linkIndex++
		}

		if len(connectedNodeIndices) > f.maxPointDegree {
			f.maxPointDegree = len(connectedNodeIndices)
		}
	}

	f.linkFirstIndicesAndAmountFbo = ctx.newFramebuffer(pointsTextureSize, pointsTextureSize, linkFirstIndicesAndAmount)
	f.indicesFbo = ctx.newFramebuffer(linksTextureSize, linksTextureSize, indices)
	f.biasAndStrengthFbo = ctx.newFramebuffer(linksTextureSize, linksTextureSize, linkBiasAndStrengthState)
	f.randomDistanceFbo = ctx.newFramebuffer(linksTextureSize, linksTextureSize, linkDistanceState)
}

func (f *forceLink) initPrograms(ctx *glCtx, cfg *Config, st *store, pts *points, quadAttr []attrBinding) error {
	prog, err := ctx.program(quadVert, forceSpringFrag(f.maxPointDegree))
	if err != nil {
		return err
	}
	f.runCommand = &command{
		ctx: ctx, prog: prog,
		fbo:       func() *framebuffer { return pts.velocityFbo },
		primitive: "triangle strip",
		count:     func() int { return 4 },
		attrs:     quadAttr,
		uniforms: map[string]func() uniformValue{
			"position":     func() uniformValue { return pts.previousPositionFbo },
			"linkSpring":   func() uniformValue { return cfg.Simulation.LinkSpring },
			"linkDistance": func() uniformValue { return cfg.Simulation.LinkDistance },
			"linkDistRandomVariationRange": func() uniformValue {
				return cfg.Simulation.LinkDistRandomVariationRange[:]
			},
			"linkFirstIndicesAndAmount": func() uniformValue { return f.linkFirstIndicesAndAmountFbo },
			"linkIndices":               func() uniformValue { return f.indicesFbo },
			"linkBiasAndStrength":       func() uniformValue { return f.biasAndStrengthFbo },
			"linkRandomDistanceFbo":     func() uniformValue { return f.randomDistanceFbo },
			"pointsTextureSize":         func() uniformValue { return float64(st.pointsTextureSize) },
			"linksTextureSize":          func() uniformValue { return float64(st.linksTextureSize) },
			"alpha":                     func() uniformValue { return st.alpha },
		},
	}
	return nil
}

func (f *forceLink) run() { f.runCommand.run() }

func (f *forceLink) destroy() {
	f.linkFirstIndicesAndAmountFbo.destroy()
	f.indicesFbo.destroy()
	f.biasAndStrengthFbo.destroy()
	f.randomDistanceFbo.destroy()
	f.linkFirstIndicesAndAmountFbo = nil
	f.indicesFbo = nil
	f.biasAndStrengthFbo = nil
	f.randomDistanceFbo = nil
}
