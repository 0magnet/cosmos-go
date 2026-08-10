// Generates data.json with the same clustered graph as examples/demo, so
// the Go port and the original cosmos.gl can be compared on identical data.
package main

import (
	"encoding/json"
	"math/rand"
	"os"
)

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

func main() {
	r := rand.New(rand.NewSource(42))
	var positions, colors, sizes, shapes, links []float32
	var hubs []int

	const space = 8192.0
	randPos := func() float32 { return float32(space * (0.45 + 0.1*r.Float64())) }

	index := 0
	for c := 0; c < clusters; c++ {
		color := clusterColors[c%len(clusterColors)]
		shape := float32(c % 8)
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
	for c := 0; c < clusters; c++ {
		links = append(links, float32(hubs[c]), float32(hubs[(c+1)%clusters]))
		if r.Float64() < 0.5 {
			links = append(links, float32(hubs[c]), float32(hubs[r.Intn(clusters)]))
		}
	}

	out, err := os.Create("data.json")
	if err != nil {
		panic(err)
	}
	defer out.Close()
	if err := json.NewEncoder(out).Encode(map[string]interface{}{
		"positions": positions,
		"colors":    colors,
		"sizes":     sizes,
		"shapes":    shapes,
		"links":     links,
	}); err != nil {
		panic(err)
	}
}
