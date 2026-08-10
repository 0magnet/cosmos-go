//go:build js && wasm

package cosmos

import "sort"

// graphData is the port of the GraphData module: node/link bookkeeping and
// the degree-sorted index mappings (higher-degree nodes render on top).
type graphData struct {
	nodes []Node
	links []Link

	// links whose source and target both exist, in input order
	completeLinks []int // indices into links
	degree        []int

	// sorted-index adjacency: source sorted index → target sorted indices
	groupedSourceToTarget map[int][]int
	groupedTargetToSource map[int][]int
	// insertion order of keys for deterministic iteration (JS Maps preserve
	// insertion order and the link force layout depends on stable iteration)
	groupedSourceKeys []int
	groupedTargetKeys []int

	idToInputIndex     map[string]int
	sortedToInputIndex []int
	inputToSortedIndex []int
	idToSortedIndex    map[string]int
}

func newGraphData() *graphData {
	return &graphData{
		groupedSourceToTarget: map[int][]int{},
		groupedTargetToSource: map[int][]int{},
		idToInputIndex:        map[string]int{},
		idToSortedIndex:       map[string]int{},
	}
}

func (g *graphData) linksNumber() int { return len(g.completeLinks) }

func (g *graphData) setData(nodes []Node, links []Link) {
	g.nodes = nodes
	g.links = links
	g.idToInputIndex = make(map[string]int, len(nodes))
	indegree := make([]int, len(nodes))
	outdegree := make([]int, len(nodes))

	for i := range nodes {
		g.idToInputIndex[nodes[i].ID] = i
	}

	// filter links whose source/target exist and count degrees
	g.completeLinks = g.completeLinks[:0]
	for li := range links {
		l := &links[li]
		si, sok := g.idToInputIndex[l.Source]
		ti, tok := g.idToInputIndex[l.Target]
		if sok && tok {
			g.completeLinks = append(g.completeLinks, li)
			outdegree[si]++
			indegree[ti]++
		}
	}

	g.degree = make([]int, len(nodes))
	for i := range nodes {
		g.degree[i] = indegree[i] + outdegree[i]
	}

	// sort node indices by degree (stable to keep input order on ties,
	// like the JS Object.entries sort)
	order := make([]int, len(nodes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return g.degree[order[a]] < g.degree[order[b]]
	})

	g.sortedToInputIndex = order
	g.inputToSortedIndex = make([]int, len(nodes))
	g.idToSortedIndex = make(map[string]int, len(nodes))
	for sortedIndex, inputIndex := range order {
		g.inputToSortedIndex[inputIndex] = sortedIndex
		g.idToSortedIndex[nodes[inputIndex].ID] = sortedIndex
	}

	// group links by sorted indices, preserving insertion order
	g.groupedSourceToTarget = map[int][]int{}
	g.groupedTargetToSource = map[int][]int{}
	g.groupedSourceKeys = g.groupedSourceKeys[:0]
	g.groupedTargetKeys = g.groupedTargetKeys[:0]
	for li := range links {
		l := &links[li]
		sourceIndex, sok := g.idToSortedIndex[l.Source]
		targetIndex, tok := g.idToSortedIndex[l.Target]
		if !sok || !tok {
			continue
		}
		if _, exists := g.groupedSourceToTarget[sourceIndex]; !exists {
			g.groupedSourceKeys = append(g.groupedSourceKeys, sourceIndex)
		}
		if !containsInt(g.groupedSourceToTarget[sourceIndex], targetIndex) {
			g.groupedSourceToTarget[sourceIndex] = append(g.groupedSourceToTarget[sourceIndex], targetIndex)
		}
		if _, exists := g.groupedTargetToSource[targetIndex]; !exists {
			g.groupedTargetKeys = append(g.groupedTargetKeys, targetIndex)
		}
		if !containsInt(g.groupedTargetToSource[targetIndex], sourceIndex) {
			g.groupedTargetToSource[targetIndex] = append(g.groupedTargetToSource[targetIndex], sourceIndex)
		}
	}
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (g *graphData) getNodeByID(id string) *Node {
	if i, ok := g.idToInputIndex[id]; ok {
		return &g.nodes[i]
	}
	return nil
}

func (g *graphData) getNodeByIndex(index int) *Node {
	if index >= 0 && index < len(g.nodes) {
		return &g.nodes[index]
	}
	return nil
}

// getSortedIndexByInputIndex returns -1 when out of range.
func (g *graphData) getSortedIndexByInputIndex(index int) int {
	if index < 0 || index >= len(g.inputToSortedIndex) {
		return -1
	}
	return g.inputToSortedIndex[index]
}

func (g *graphData) getInputIndexBySortedIndex(index int) int {
	if index < 0 || index >= len(g.sortedToInputIndex) {
		return -1
	}
	return g.sortedToInputIndex[index]
}

func (g *graphData) getSortedIndexByID(id string) int {
	if i, ok := g.idToSortedIndex[id]; ok {
		return i
	}
	return -1
}

func (g *graphData) getInputIndexByID(id string) int {
	if i, ok := g.idToInputIndex[id]; ok {
		return i
	}
	return -1
}

// getAdjacentNodes returns the nodes connected to id (nil if unknown).
func (g *graphData) getAdjacentNodes(id string) []*Node {
	index := g.getSortedIndexByID(id)
	if index == -1 {
		return nil
	}
	seen := map[int]bool{}
	var result []*Node
	for _, set := range [][]int{g.groupedSourceToTarget[index], g.groupedTargetToSource[index]} {
		for _, sortedIndex := range set {
			if !seen[sortedIndex] {
				seen[sortedIndex] = true
				result = append(result, g.getNodeByIndex(g.getInputIndexBySortedIndex(sortedIndex)))
			}
		}
	}
	return result
}
