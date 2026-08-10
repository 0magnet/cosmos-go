<p align="center">
  <h1 align="center">🌌 cosmos-go</h1>
</p>
<p align="center">GPU-accelerated Force Graph — <a href="https://github.com/cosmograph-org/cosmos">Cosmos</a> ported to Go/WebAssembly</p>

cosmos-go is a full Go port of [@cosmograph/cosmos](https://github.com/cosmograph-org/cosmos) 1.6.1, the WebGL force graph layout algorithm and rendering engine used by (among others) the Skywire network visualizer. All computations and drawing happen on the GPU in fragment and vertex shaders (carried over verbatim from the original), avoiding expensive memory operations. It enables real-time simulation of network graphs consisting of hundreds of thousands of nodes and edges on modern hardware.

The Go port drives the shaders through `syscall/js` with a raw-WebGL command layer (replacing `regl`), reimplements the d3-zoom pan/zoom behavior — including d3's smooth van Wijk–Nuij zoom transitions — and compiles with both the **standard Go toolchain** (`GOOS=js GOARCH=wasm`, ~3.3 MB) and **TinyGo** (`-target wasm`, ~550 KB).

[🎮 Live demo](https://0magnet.github.io/cosmos-go/) (compiled with TinyGo)

### Quick Start

```bash
go get github.com/0magnet/cosmos-go
```

Get the data, configure the graph and run the simulation:

```go
package main

import (
    "syscall/js"

    cosmos "github.com/0magnet/cosmos-go"
)

func main() {
    canvas := js.Global().Get("document").Call("querySelector", "canvas")

    cfg := cosmos.NewConfig()
    cfg.Simulation.Repulsion = 0.5
    cfg.Events.OnClick = func(node *cosmos.Node, index int, pos [2]float64, event js.Value) {
        if node != nil {
            println("Clicked node:", node.ID)
        }
    }

    graph, err := cosmos.New(canvas, cfg)
    if err != nil {
        panic(err)
    }

    nodes := []cosmos.Node{
        {ID: "a", Color: "#fd7f6f"},
        {ID: "b", Color: "#7eb0d5"},
        {ID: "c", Color: "#b2e061"},
    }
    links := []cosmos.Link{
        {Source: "a", Target: "b"},
        {Source: "b", Target: "c"},
        {Source: "c", Target: "a"},
    }
    graph.SetData(nodes, links, true)

    select {} // keep the Go runtime alive for the render loop
}
```

> **Note**
> If your canvas element has no width and height styles (either CSS or inline), cosmos-go will automatically set them to 100%.
>
> WebGL 1 with the `OES_texture_float` and `ANGLE_instanced_arrays` extensions is required (available in all modern desktop browsers). Like the original, the Many-Body force also relies on float blending (`EXT_float_blend`), which iOS stopped exposing in 15.4.

Build and serve:

```bash
GOOS=js GOARCH=wasm go build -o app.wasm . && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
# or, for a ~6x smaller binary:
tinygo build -o app.wasm -target wasm -no-debug . && cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" .
```

See <a href="examples/demo">examples/demo</a> for a complete example with pan/zoom, hover, selection and a build script for both toolchains.

### Data model

Instead of the accessor functions of the JS original, nodes and links carry their optional per-item values directly (zero values fall back to the config defaults):

```go
type Node struct {
    ID          string
    X, Y        float64 // initial / fixed position (used when HasPosition is true)
    HasPosition bool
    Color       string  // "" → Config.NodeColor
    Size        float64 // 0  → Config.NodeSize
}

type Link struct {
    Source, Target string
    Color          string    // "" → Config.LinkColor
    Width          float64   // 0  → Config.LinkWidth
    Arrow          ArrowMode // ArrowDefault | ArrowOn | ArrowOff
}
```

### Configuration

`cosmos.NewConfig()` returns a `*Config` with the same defaults as the original library. All configuration parameters of cosmos 1.6.1 are mirrored 1:1 with Go naming:

| Go field | JS equivalent | Default |
|---|---|---|
| `DisableSimulation` | `disableSimulation` | `false` |
| `BackgroundColor` | `backgroundColor` | `#222222` |
| `SpaceSize` | `spaceSize` | `4096` |
| `NodeColor`, `NodeSize`, `NodeSizeScale` | `nodeColor`, `nodeSize`, `nodeSizeScale` | `#b3b3b3`, `4`, `1` |
| `NodeGreyoutOpacity` | `nodeGreyoutOpacity` | `0.1` |
| `RenderHoveredNodeRing`, `HoveredNodeRingColor`, `FocusedNodeRingColor` | `renderHoveredNodeRing`, ... | `true`, `white`, `white` |
| `RenderLinks`, `LinkColor`, `LinkWidth`, `LinkWidthScale` | `renderLinks`, `linkColor`, ... | `true`, `#666666`, `1`, `1` |
| `LinkGreyoutOpacity` | `linkGreyoutOpacity` | `0.1` |
| `CurvedLinks`, `CurvedLinkSegments`, `CurvedLinkWeight`, `CurvedLinkControlPointDistance` | `curvedLinks`, ... | `false`, `19`, `0.8`, `0.5` |
| `LinkArrows`, `LinkArrowsSizeScale` | `linkArrows`, `linkArrowsSizeScale` | `true`, `1` |
| `LinkVisibilityDistanceRange`, `LinkVisibilityMinTransparency` | `linkVisibilityDistanceRange`, ... | `[50, 150]`, `0.25` |
| `UseQuadtree` | `useQuadtree` | `false` |
| `ShowFPSMonitor` | `showFPSMonitor` | `false` |
| `PixelRatio` | `pixelRatio` | `2` |
| `ScaleNodesOnZoom` | `scaleNodesOnZoom` | `true` |
| `InitialZoomLevel` | `initialZoomLevel` | unset (`0`) |
| `DisableZoom` | `disableZoom` | `false` |
| `FitViewOnInit`, `FitViewDelay`, `FitViewByNodesInRect` | `fitViewOnInit`, ... | `true`, `250`, unset |
| `RandomSeed` | `randomSeed` | `""` |
| `NodeSamplingDistance` | `nodeSamplingDistance` | `150` |

Simulation parameters live under `Config.Simulation` (`Decay`, `Gravity`, `Center`, `Repulsion`, `RepulsionTheta`, `RepulsionQuadtreeLevels`, `LinkSpring`, `LinkDistance`, `LinkDistRandomVariationRange`, `RepulsionFromMouse`, `Friction`) with callbacks `OnStart`, `OnTick`, `OnEnd`, `OnPause`, `OnRestart` — and event callbacks under `Config.Events` (`OnClick`, `OnMouseMove`, `OnNodeMouseOver`, `OnNodeMouseOut`, `OnZoomStart`, `OnZoom`, `OnZoomEnd`).

### API

Mirroring the [cosmos API](https://github.com/cosmograph-org/cosmos/wiki/API-Reference):

- `New(canvas js.Value, cfg *Config) (*Graph, error)`
- `SetData(nodes []Node, links []Link, runSimulation bool)`
- `Start(alpha float64)` / `Pause()` / `Restart()` / `Step()` / `Destroy()`
- `Progress()`, `IsSimulationRunning()`, `MaxPointSize()`
- `Zoom(value, duration)` / `SetZoomLevel(value, duration)` / `GetZoomLevel()`
- `FitView(duration, padding)` / `FitViewByNodeIDs(ids, duration, padding)`
- `ZoomToNodeByID(id, duration, scale, canZoomOut)` / `ZoomToNodeByIndex(...)`
- `GetNodePositions()`, `GetNodePositionsArray()`
- `SelectNodesInRange(area, ok)`, `SelectNodeByID(id, withAdjacent)`, `SelectNodesByIDs(ids)`, `SelectNodesByIndices(indices)`, `UnselectNodes()`, `GetSelectedNodes()`
- `GetAdjacentNodes(id)`
- `SetFocusedNodeByID(id)` / `SetFocusedNodeByIndex(index)`
- `SpaceToScreenPosition(pos)`, `SpaceToScreenRadius(r)`
- `GetNodeRadiusByID(id)` / `GetNodeRadiusByIndex(index)`
- `TrackNodePositionsByIDs(ids)` / `TrackNodePositionsByIndices(...)` / `GetTrackedNodePositionsMap()`
- `GetSampledNodePositionsMap()`
- `DisableZoom()` / `EnableZoom()`
- `UpdateNodeColor()`, `UpdateNodeSize()`, `UpdateLinkColor()`, `UpdateLinkWidth()`, `UpdateLinkArrows()`, `UpdateCurveLineGeometry()`, `UpdateBackgroundColor()` — re-apply config/data-derived buffers after mutating `Config()` or the data in place (the Go equivalent of `setConfig` diffing)

Interactions match the original: drag to pan, mouse wheel to zoom, double-click to zoom in (shift + double-click to zoom out), hold the right mouse button to repel nodes with the mouse force.

### Differences from cosmos.js

- Per-node/per-link fields replace the accessor-function idiom (see Data model above)
- `Config` is a plain struct created by `NewConfig()`; instead of `setConfig` diffing, call the matching `Update*` method after changing visual parameters (simulation parameters are read live every frame)
- The FPS monitor is a small built-in overlay instead of the `gl-bench` dependency
- The random jitter PRNG is deterministic per `RandomSeed` but produces a different sequence than the JS `random` package, so layouts are reproducible within cosmos-go but not pixel-identical to cosmos.js runs
- `renderHighlightedNodeRing` / `highlightedNodeRingColor` (deprecated in 1.6) are omitted; use `RenderHoveredNodeRing` / `HoveredNodeRingColor` / `FocusedNodeRingColor`

### Known Issues

Inherited from the original: starting from version 15.4, iOS has stopped supporting the key WebGL extension powering the Many-Body force implementation (`EXT_float_blend`).

### License

CC-BY-NC-4.0, same as the ported cosmos 1.6.1 source (see LICENCE).

cosmos-go is adapted material derived from [Cosmos](https://github.com/cosmograph-org/cosmos) by the cosmograph-org team, © cosmograph.app. Like the original, it is free for non-commercial usage; for commercial use please [reach out to the Cosmos authors](mailto:hi@cosmograph.app). Note that Cosmos 2.x (cosmos.gl) has since been relicensed under MIT — a future re-port based on the 2.x codebase could carry the MIT license.
