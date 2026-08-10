//go:build js && wasm

// Comparison page for cosmos-go: loads the same data.json as compare-js.html
// (which runs the original cosmos.gl 2.6.3) with an identical configuration,
// and exposes the same programmatic hooks for automated equivalence testing.
//
// URL parameters: ?sim=0 disables the simulation (deterministic layout).
package main

import (
	"syscall/js"

	cosmos "github.com/0magnet/cosmos-go"
)

func float32sFromJS(arr js.Value) []float32 {
	n := arr.Get("length").Int()
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = float32(arr.Index(i).Float())
	}
	return out
}

func main() {
	global := js.Global()
	document := global.Get("document")
	div := document.Call("getElementById", "cosmos")

	params := global.Get("URLSearchParams").New(global.Get("location").Get("search"))
	sim := params.Call("get", "sim").String() != "0"
	det := params.Call("get", "det").String() == "1"

	cfg := cosmos.NewConfig()
	cfg.EnableSimulation = sim
	cfg.BackgroundColor = "#0d1117"
	cfg.SpaceSize = 8192
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
	if det {
		cfg.SimulationLinkDistRandomVariationRange = [2]float64{1, 1}
	}

	graph, err := cosmos.New(div, cfg)
	if err != nil {
		global.Get("console").Call("error", err.Error())
		return
	}

	// fetch data.json, then set data and render
	fetchThen := global.Call("fetch", "data.json").Call("then",
		js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			return args[0].Call("json")
		}))
	fetchThen.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data := args[0]
		graph.SetPointPositions(float32sFromJS(data.Get("positions")))
		graph.SetPointColors(float32sFromJS(data.Get("colors")))
		graph.SetPointSizes(float32sFromJS(data.Get("sizes")))
		graph.SetPointShapes(float32sFromJS(data.Get("shapes")))
		graph.SetLinks(float32sFromJS(data.Get("links")))
		if params.Call("get", "alpha0").String() == "1" {
			graph.Render(0)
		} else {
			graph.Render()
		}
		global.Set("graphReady", true)
		return nil
	}))

	// hooks (same names as in compare-js.html)
	global.Set("hookZoom", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return graph.GetZoomLevel()
	}))
	global.Set("hookFit", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		graph.FitView(0, 0.1)
		return nil
	}))
	global.Set("hookRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		indices := graph.GetPointsInRect([2][2]float64{
			{args[0].Float(), args[1].Float()},
			{args[2].Float(), args[3].Float()},
		})
		out := make([]interface{}, len(indices))
		for i, v := range indices {
			out[i] = v
		}
		return out
	}))
	global.Set("hookPositions", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		positions := graph.GetPointPositions()
		out := make([]interface{}, len(positions))
		for i, v := range positions {
			out[i] = v
		}
		return out
	}))
	global.Set("hookPause", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		graph.Pause()
		return nil
	}))

	global.Set("hookS2S", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		p := graph.SpaceToScreenPosition([2]float64{args[0].Float(), args[1].Float()})
		return []interface{}{p[0], p[1]}
	}))

	global.Set("hookPrep", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		graph.Start(1)
		graph.Pause()
		return nil
	}))
	global.Set("hookStep", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		n := args[0].Int()
		for i := 0; i < n; i++ {
			graph.Step()
		}
		return nil
	}))

	select {}
}
