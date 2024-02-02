package daemon

type graphColor uint8

const (
	graphColorWhite graphColor = iota
	graphColorGray
	graphColorBlack
)

type graph[id ID] interface {
	nodes() []id
	neighbors(id) []id
}

type dagVisitor[id ID] struct {
	Graph  graph[id]
	Colors map[id]graphColor
}

func newDAGVisitor[id ID](graph graph[id]) *dagVisitor[id] {
	return &dagVisitor[id]{Graph: graph, Colors: map[id]graphColor{}}
}

func (dvr *dagVisitor[id]) Single(node id) ([]id, bool) {
	return dvr.visit(node, nil)
}

func (dvr *dagVisitor[id]) All() (queue []id, isDAG bool) {
	for _, n := range dvr.Graph.nodes() {
		if c, _ := dvr.Colors[n]; c != graphColorWhite {
			continue
		}
		if queue, isDAG = dvr.visit(n, queue); !isDAG {
			return nil, false
		}
	}
	return queue, true
}

func (dvr *dagVisitor[id]) visit(node id, queue []id) ([]id, bool) {
	switch c, _ := dvr.Colors[node]; c {
	default:
		fallthrough
	case graphColorGray:
		return nil, false
	case graphColorBlack:
		return nil, true
	case graphColorWhite:
		dvr.Colors[node] = graphColorGray
		var isDAG bool
		for _, w := range dvr.Graph.neighbors(node) {
			if queue, isDAG = dvr.visit(w, queue); !isDAG {
				return nil, false
			}
		}
		dvr.Colors[node] = graphColorBlack
		return append(queue, node), true
	}
}
