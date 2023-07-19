package graph

import (
	"fmt"
	"math"
	"sync"

	"github.com/btoews/golp"
	"golang.org/x/exp/slices"
)

type Solver interface {
	Solve(k int, vertexWeights []float64) (cost float64, picks []string, err error)
}

type Graph struct {
	Vertices  []string
	EdgeCosts [][]float64
	lp        *golp.LP
}

var _ Solver = (*Graph)(nil)

// Create a new graph with named vertices and edge costs. Edge costs are
// symmetrical, so only half the matrix is specified. For example, the cells
// marked x should be specified for a graph with 3 vertices.
//
//	  |A|B|C|
//	A | | | |
//	B |x| | |
//	C |x|x| |
func NewGraph(vertices []string, edgeCosts [][]float64) (*Graph, error) {
	g := &Graph{Vertices: vertices, EdgeCosts: edgeCosts}
	if err := g.initLP(); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *Graph) Solve(k int, vertexWeights []float64) (float64, []string, error) {
	nVertices := len(g.Vertices)
	nEdges := nVertices * (nVertices - 1) / 2
	nCols := nVertices + nEdges*2

	lp := g.lp.Copy()

	// constraint: must choose k sinks
	//   A+B+C=k
	row := make([]golp.Entry, 0, nVertices)
	for sink := 0; sink < nVertices; sink++ {
		row = append(row, g.entry(sink))
	}
	if err := lp.AddConstraintSparse(row, golp.EQ, float64(k)); err != nil {
		return 0, nil, err
	}

	objRow := make([]float64, nCols)
	for ri, row := range g.EdgeCosts {
		a := ri + 1
		for b, cost := range row {
			objRow[g.edge(a, b)] = cost * vertexWeights[a]
			objRow[g.edge(b, a)] = cost * vertexWeights[b]
		}
	}
	lp.SetObjFn(objRow)

	if st := lp.Solve(); st != golp.OPTIMAL {
		return 0, nil, fmt.Errorf("%s solution", st)
	}

	vars := lp.Variables()
	ret := make([]string, 0, k)
	for i, vertex := range g.Vertices {
		if vars[i] != 0 {
			ret = append(ret, vertex)
		}
	}
	slices.Sort(ret)

	return lp.Objective(), ret, nil
}

func (g *Graph) initLP() error {
	nVertices := len(g.Vertices)
	nEdges := nVertices * (nVertices - 1) / 2
	nCols := nVertices + nEdges*2

	lp := golp.NewLP(0, nCols)

	for c := 0; c < nCols; c++ {
		lp.SetBinary(c, true)
	}

	for source := 0; source < nVertices; source++ {
		lp.SetColName(source, g.Vertices[source])
		sourceOrSink := append(make([]golp.Entry, 0, nVertices), g.entry(source))

		for sink := 0; sink < nVertices; sink++ {
			if source == sink {
				continue
			}

			lp.SetColName(g.edge(source, sink), g.Vertices[source]+"_"+g.Vertices[sink])
			sourceOrSink = append(sourceOrSink, g.entry(source, sink))

			// O(n^2) constraints: only sinks have incoming edges
			//   A - BA >= 0
			//   A - CA >= 0
			lp.AddConstraintSparse([]golp.Entry{
				g.entry(sink),
				g.entryVal(-1, source, sink),
			}, golp.GE, 0)
		}

		// O(n) constraints: each vertex must have 1 sink or be a source
		//   A+AB+AC = 1
		if err := lp.AddConstraintSparse(sourceOrSink, golp.EQ, 1); err != nil {
			return err
		}
	}

	g.lp = lp

	return nil
}

func (g *Graph) entry(source int, sink ...int) golp.Entry {
	return g.entryVal(1, source, sink...)
}

func (g *Graph) entryVal(val float64, source int, sink ...int) golp.Entry {
	switch len(sink) {
	case 0:
		return golp.Entry{Col: source, Val: val}
	case 1:
		return golp.Entry{Col: g.edge(source, sink[0]), Val: val}
	default:
		panic("bad entry call")
	}
}

func (g *Graph) edge(source, sink int) int {
	if source == sink {
		panic("source=sink")
	}

	nVertices := len(g.Vertices)

	i := nVertices
	i += source * (nVertices - 1)
	if sink > source {
		i += sink - 1
	} else {
		i += sink
	}

	return i
}

type lp struct {
	lp   *golp.LP
	k    int
	kRow int
}

type BruteForcer struct {
	Vertices  []string
	EdgeCosts [][]float64
	vmap      map[string]int
}

func NewBruteForcer(vertices []string, edgeCosts [][]float64) *BruteForcer {
	vmap := make(map[string]int, len(vertices))
	for i, v := range vertices {
		vmap[v] = i
	}
	return &BruteForcer{vertices, edgeCosts, vmap}
}

var _ Solver = (*BruteForcer)(nil)

func (g *BruteForcer) Solve(k int, vertexWeights []float64) (float64, []string, error) {
	wec := g.weightedEdgeCosts(vertexWeights)

	var (
		bestCombo     = make([]int, k)
		bestComboCost = math.MaxFloat64
	)

	combos := newCombinationEnumerator(len(g.Vertices), k)
	for combos.next() {
		if cc := g.comboCost(wec, combos.State); cc < bestComboCost {
			copy(bestCombo, combos.State)
			bestComboCost = cc
		}
	}

	ret := make([]string, k)
	for i := range ret {
		ret[i] = g.Vertices[bestCombo[i]]
	}
	slices.Sort(ret)

	return bestComboCost, ret, nil
}

func (g *BruteForcer) weightedEdgeCosts(vertexWeights []float64) [][]float64 {
	wec := make([][]float64, len(g.Vertices))
	for j := range wec {
		wec[j] = make([]float64, len(g.Vertices))
	}
	for j, row := range g.EdgeCosts {
		a := j + 1
		for b, cost := range row {
			wec[a][b] = vertexWeights[a] * cost
			wec[b][a] = vertexWeights[b] * cost
		}
	}
	return wec
}

func (g *BruteForcer) CombinationCost(combo []string, vertexWeights []float64) (float64, error) {
	icombo := make([]int, 0, len(combo))
	for _, c := range combo {
		i, ok := g.vmap[c]
		if !ok {
			return 0, fmt.Errorf("unknown vertex %q", c)
		}
		icombo = append(icombo, i)
	}

	wec := g.weightedEdgeCosts(vertexWeights)

	return g.comboCost(wec, icombo), nil
}

func (g *BruteForcer) comboCost(wec [][]float64, combo []int) float64 {
	var comboCost float64

	for source := range g.Vertices {
		bestSinkCost := math.MaxFloat64

	inner:
		for _, sink := range combo {
			if scost := wec[source][sink]; scost < bestSinkCost {
				bestSinkCost = scost
				if scost == 0 {
					break inner
				}
			}
		}
		comboCost += bestSinkCost
	}

	return comboCost
}

type combinationEnumerator struct {
	State            []int
	readable, resume chan struct{}
	once             sync.Once
}

func newCombinationEnumerator(n, k int) *combinationEnumerator {
	ce := &combinationEnumerator{
		State:    make([]int, k),
		resume:   make(chan struct{}),
		readable: make(chan struct{}),
	}

	go func() {
		defer close(ce.readable)
		ce.enumerateCombinations(n, k, 0)
	}()

	return ce
}

func (ce *combinationEnumerator) enumerateCombinations(n, k, start int) bool {
	if k == 0 {
		ce.readable <- struct{}{}
		if _, ok := <-ce.resume; !ok {
			return false
		}
		return true
	}
	for i := start; i <= n-k; i++ {
		ce.State[len(ce.State)-k] = i
		if !ce.enumerateCombinations(n, k-1, i+1) {
			return false
		}
	}
	return true
}

func (ce *combinationEnumerator) next() bool {
	var skipResume bool
	ce.once.Do(func() {
		skipResume = true
	})
	if !skipResume {
		ce.resume <- struct{}{}
	}
	_, ok := <-ce.readable
	return ok
}
