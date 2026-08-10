//go:build js && wasm

// Demo for cosmos-go: a clustered random graph with hover, click, drag,
// selection, point shapes and zoom controls.
package main

import (
	"fmt"
	"math/rand"
	"syscall/js"

	cosmos "github.com/0magnet/cosmos-go"
)

var document = js.Global().Get("document")

// cluster colors as normalized 0..1 RGBA (the cosmos color format,
// see GetRgbaColor)
var clusterColors = [][4]float32{
	{253. / 255, 127. / 255, 111. / 255, 1}, {126. / 255, 176. / 255, 213. / 255, 1},
	{178. / 255, 224. / 255, 97. / 255, 1}, {189. / 255, 126. / 255, 190. / 255, 1},
	{255. / 255, 181. / 255, 90. / 255, 1}, {255. / 255, 238. / 255, 101. / 255, 1},
	{190. / 255, 185. / 255, 219. / 255, 1}, {253. / 255, 204. / 255, 229. / 255, 1},
}

const (
	clusters         = 8
	pointsPerCluster = 220
)

func generateClusteredGraph() (positions, colors, sizes, shapes []float32, links []float32, hubs []int) {
	r := rand.New(rand.NewSource(42))
	total := clusters * (pointsPerCluster + 1)
	positions = make([]float32, 0, total*2)
	colors = make([]float32, 0, total*4)
	sizes = make([]float32, 0, total)
	shapes = make([]float32, 0, total)

	// random initial positions near the space center (the simulation takes
	// over from there; unlike v1, cosmos v2 uses the given positions as-is)
	const space = 8192.0
	randPos := func() float32 { return float32(space * (0.45 + 0.1*r.Float64())) }

	index := 0
	for c := 0; c < clusters; c++ {
		color := clusterColors[c%len(clusterColors)]
		shape := cosmos.PointShape(c % 8)
		// hub
		hubs = append(hubs, index)
		positions = append(positions, randPos(), randPos())
		colors = append(colors, color[0], color[1], color[2], color[3])
		sizes = append(sizes, 12)
		shapes = append(shapes, shape)
		hubIndex := index
		index++
		for n := 0; n < pointsPerCluster; n++ {
			positions = append(positions, randPos(), randPos())
			colors = append(colors, color[0], color[1], color[2], color[3])
			sizes = append(sizes, 3+r.Float32()*4)
			shapes = append(shapes, shape)
			links = append(links, float32(index), float32(hubIndex))
			if n > 0 && r.Float64() < 0.2 {
				other := hubIndex + 1 + r.Intn(n)
				links = append(links, float32(index), float32(other))
			}
			index++
		}
	}
	// connect the cluster hubs in a ring plus some random chords
	for c := 0; c < clusters; c++ {
		links = append(links, float32(hubs[c]), float32(hubs[(c+1)%clusters]))
		if r.Float64() < 0.5 {
			links = append(links, float32(hubs[c]), float32(hubs[r.Intn(clusters)]))
		}
	}
	return
}

func setStatus(text string) {
	document.Call("getElementById", "status").Set("textContent", text)
}

func onClick(id string, fn func()) {
	document.Call("getElementById", id).Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fn()
		return nil
	}))
}

func main() {
	div := document.Call("getElementById", "cosmos")

	cfg := cosmos.NewConfig()
	cfg.BackgroundColor = "#0d1117"
	cfg.SpaceSize = 8192
	cfg.PointDefaultSize = 4
	cfg.LinkDefaultColor = "#5F74C2"
	cfg.RenderHoveredPointRing = true
	cfg.HoveredPointRingColor = "white"
	cfg.EnableDrag = true
	cfg.EnableRightClickRepulsion = true
	cfg.SimulationRepulsion = 1.0
	cfg.SimulationLinkSpring = 1.2
	cfg.SimulationLinkDistance = 10
	cfg.SimulationGravity = 0.25
	cfg.SimulationDecay = 5000
	cfg.RandomSeed = "skywire"
	cfg.OnPointClick = func(index int, pos [2]float64, event js.Value) {
		setStatus(fmt.Sprintf("clicked point %d", index))
	}
	cfg.OnBackgroundClick = func(event js.Value) {
		setStatus("clicked background")
	}
	cfg.OnPointMouseOver = func(index int, pos [2]float64, event js.Value) {
		setStatus(fmt.Sprintf("hovered point %d", index))
	}
	cfg.OnPointMouseOut = func(event js.Value) {
		setStatus("")
	}
	cfg.OnLinkMouseOver = func(linkIndex int) {
		setStatus(fmt.Sprintf("hovered link %d", linkIndex))
	}
	cfg.OnLinkMouseOut = func(event js.Value) {
		setStatus("")
	}
	cfg.OnSimulationEnd = func() {
		setStatus("simulation settled")
	}

	graph, err := cosmos.New(div, cfg)
	if err != nil {
		js.Global().Get("console").Call("error", err.Error())
		return
	}

	positions, colors, sizes, shapes, links, hubs := generateClusteredGraph()
	graph.SetPointPositions(positions)
	graph.SetPointColors(colors)
	graph.SetPointSizes(sizes)
	graph.SetPointShapes(shapes)
	graph.SetLinks(links)
	graph.Render()
	setStatus(fmt.Sprintf("%d points, %d links", len(positions)/2, len(links)/2))

	// expose for automated tests / console poking
	js.Global().Set("cosmosReady", true)
	js.Global().Set("cosmosZoom", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return graph.GetZoomLevel()
	}))
	js.Global().Set("cosmosPointScreen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		index := args[0].Int()
		positions := graph.GetPointPositions()
		if index*2+1 >= len(positions) {
			return nil
		}
		screen := graph.SpaceToScreenPosition([2]float64{positions[index*2], positions[index*2+1]})
		return []interface{}{screen[0], screen[1]}
	}))

	onClick("btn-fit", func() { graph.FitView(400, 0.1) })
	onClick("btn-restart", func() { graph.Start(1) })
	onClick("btn-pause", func() {
		if graph.IsSimulationRunning() {
			graph.Pause()
			setStatus("paused")
		} else {
			graph.Unpause()
			setStatus("running")
		}
	})
	onClick("btn-select", func() {
		graph.SelectPointByIndex(hubs[0], true)
		setStatus("selected cluster 0")
	})
	onClick("btn-unselect", func() { graph.UnselectPoints(); setStatus("") })
	onClick("btn-zoomto", func() { graph.ZoomToPointByIndex(hubs[3], 700, 3, true) })

	select {}
}
