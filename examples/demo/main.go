//go:build js && wasm

// Demo for cosmos-go: a clustered random graph with hover, click,
// selection and zoom controls.
package main

import (
	"fmt"
	"math/rand"
	"syscall/js"

	cosmos "github.com/0magnet/cosmos-go"
)

var document = js.Global().Get("document")

var clusterColors = []string{
	"#fd7f6f", "#7eb0d5", "#b2e061", "#bd7ebe",
	"#ffb55a", "#ffee65", "#beb9db", "#fdcce5", "#8bd3c7",
}

func generateClusteredGraph(clusters, nodesPerCluster int) ([]cosmos.Node, []cosmos.Link) {
	var nodes []cosmos.Node
	var links []cosmos.Link
	r := rand.New(rand.NewSource(42))

	hubID := func(c int) string { return fmt.Sprintf("hub-%d", c) }
	for c := 0; c < clusters; c++ {
		color := clusterColors[c%len(clusterColors)]
		nodes = append(nodes, cosmos.Node{ID: hubID(c), Color: color, Size: 12})
		for n := 0; n < nodesPerCluster; n++ {
			id := fmt.Sprintf("n-%d-%d", c, n)
			nodes = append(nodes, cosmos.Node{ID: id, Color: color, Size: 3 + r.Float64()*4})
			links = append(links, cosmos.Link{Source: id, Target: hubID(c)})
			// a few intra-cluster cross links
			if n > 0 && r.Float64() < 0.2 {
				other := fmt.Sprintf("n-%d-%d", c, r.Intn(n))
				links = append(links, cosmos.Link{Source: id, Target: other})
			}
		}
	}
	// connect the cluster hubs in a ring plus some random chords
	for c := 0; c < clusters; c++ {
		links = append(links, cosmos.Link{
			Source: hubID(c),
			Target: hubID((c + 1) % clusters),
			Width:  2.5,
			Color:  "#aaaaaa",
		})
		if r.Float64() < 0.5 {
			links = append(links, cosmos.Link{
				Source: hubID(c),
				Target: hubID(r.Intn(clusters)),
				Width:  2,
				Color:  "#888888",
			})
		}
	}
	return nodes, links
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
	canvas := document.Call("getElementById", "cosmos")

	cfg := cosmos.NewConfig()
	cfg.BackgroundColor = "#0d1117"
	cfg.SpaceSize = 4096
	cfg.NodeSize = 4
	cfg.LinkWidth = 1
	cfg.LinkColor = "#5F74C2"
	cfg.LinkArrows = false
	cfg.Simulation.Repulsion = 1.0
	cfg.Simulation.RepulsionTheta = 1.7
	cfg.Simulation.LinkSpring = 1.2
	cfg.Simulation.LinkDistance = 10
	cfg.Simulation.Gravity = 0.15
	cfg.Simulation.Friction = 0.85
	cfg.Simulation.Decay = 2000
	cfg.RandomSeed = "skywire"
	cfg.Events.OnClick = func(node *cosmos.Node, index int, pos [2]float64, event js.Value) {
		if node != nil {
			setStatus(fmt.Sprintf("clicked: %s (index %d)", node.ID, index))
		} else {
			setStatus("clicked: background")
		}
	}
	cfg.Events.OnNodeMouseOver = func(node *cosmos.Node, index int, pos [2]float64, event js.Value) {
		setStatus("hovered: " + node.ID)
	}
	cfg.Events.OnNodeMouseOut = func(event js.Value) {
		setStatus("")
	}
	cfg.Simulation.OnEnd = func() {
		setStatus("simulation settled")
	}

	graph, err := cosmos.New(canvas, cfg)
	if err != nil {
		js.Global().Get("console").Call("error", err.Error())
		return
	}
	// expose for automated tests / console poking
	js.Global().Set("cosmosReady", true)
	js.Global().Set("cosmosZoom", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return graph.GetZoomLevel()
	}))
	js.Global().Set("cosmosNodeScreen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		positions := graph.GetNodePositions()
		p, ok := positions[args[0].String()]
		if !ok {
			return nil
		}
		screen := graph.SpaceToScreenPosition(p)
		return []interface{}{screen[0], screen[1]}
	}))

	nodes, links := generateClusteredGraph(8, 220)
	graph.SetData(nodes, links, true)
	setStatus(fmt.Sprintf("%d nodes, %d links", len(nodes), len(links)))

	onClick("btn-fit", func() { graph.FitView(400, 0.1) })
	onClick("btn-restart", func() { graph.Start(1) })
	onClick("btn-pause", func() {
		if graph.IsSimulationRunning() {
			graph.Pause()
			setStatus("paused")
		} else {
			graph.Restart()
			setStatus("running")
		}
	})
	onClick("btn-select", func() {
		graph.SelectNodeByID("hub-0", true)
		setStatus("selected hub-0 + neighbors")
	})
	onClick("btn-unselect", func() { graph.UnselectNodes(); setStatus("") })
	onClick("btn-zoomto", func() { graph.ZoomToNodeByID("hub-3", 700, 3, true) })

	select {}
}
