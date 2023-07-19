package graph

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/alecthomas/assert/v2"
	"golang.org/x/exp/constraints"
)

func TestEdge(t *testing.T) {
	g := &Graph{Vertices: []string{"A", "B", "C"}}
	a, b, c := 0, 1, 2

	// cols - A, B, C, AB, AC, BA, BC, CA, CB
	//        0  1  2  3   4   5   6   7   8

	assert.Equal(t, 3, g.edge(a, b))
	assert.Equal(t, 4, g.edge(a, c))
	assert.Equal(t, 5, g.edge(b, a))
	assert.Equal(t, 6, g.edge(b, c))
	assert.Equal(t, 7, g.edge(c, a))
	assert.Equal(t, 8, g.edge(c, b))
}

func TestGraphMatchesBruteForce(t *testing.T) {
	const maxN = 20

	vertices, edgeCosts, weights := testData(maxN)
	printMatrix("edge costs", edgeCosts, 2)
	fmt.Printf("weights\n%v\n\n", weights)
	wec := (&BruteForcer{Vertices: vertices, EdgeCosts: edgeCosts}).weightedEdgeCosts(weights)
	printMatrix("weighted edge costs", wec, 2)

	for n := 2; n <= maxN; n++ {
		bf := &BruteForcer{Vertices: vertices[:n], EdgeCosts: edgeCosts[:n-1]}
		g, err := NewGraph(vertices[:n], edgeCosts[:n-1])
		assert.NoError(t, err)

		for k := 1; k < n; k++ {
			t.Run(fmt.Sprintf("%d-choose-%d", n, k), func(t *testing.T) {
				bfCost, bfPicks, err := bf.Solve(k, weights[:n])
				assert.NoError(t, err)

				gCost, gPicks, err := g.Solve(k, weights[:n])
				assert.NoError(t, err)

				t.Logf("bf     - %10.5f %v", bfCost, bfPicks)
				t.Logf("graph  - %10.5f %v", gCost, gPicks)

				assert.True(t, (bfCost-gCost)/bfCost < 0.0001, "expected %f to be near %f", gCost, bfCost)
				assert.Equal(t, bfPicks, gPicks)
			})
		}
	}
}

func BenchmarkIncreasingNK1(b *testing.B) {
	const (
		maxN = 10
		k    = 1
	)

	vertices, edgeCosts, weights := testData(maxN)

	for n := 2; n <= maxN; n++ {
		benchGraph(b, n, k, vertices[:n], edgeCosts[:n-1], weights[:n])
	}

	for n := 2; n < maxN; n++ {
		benchBruteForce(b, n, k, vertices[:n], edgeCosts[:n-1], weights[:n])
	}
}

func BenchmarkIncreasingKN35(b *testing.B) {
	const n = 35

	vertices, edgeCosts, weights := testData(n)
	g, err := NewGraph(vertices, edgeCosts)
	assert.NoError(b, err)

	for k := 1; k < 5; k++ {
		benchGraphK(b, g, k, vertices, edgeCosts, weights)
	}
	for k := 1; k < 5; k++ {
		benchGraph(b, n, k, vertices, edgeCosts, weights)
	}
	for k := 1; k < 5; k++ {
		benchBruteForce(b, n, k, vertices, edgeCosts, weights)
	}
}

func benchGraph(b *testing.B, n, k int, vertices []string, edgeCosts [][]float64, weights []float64) {
	b.Run(fmt.Sprintf("graph-%d-choose-%d", n, k), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			g, err := NewGraph(vertices, edgeCosts)
			assert.NoError(b, err)
			_, _, err = g.Solve(k, weights)
			assert.NoError(b, err)
		}
	})
}

func benchGraphK(b *testing.B, g *Graph, k int, vertices []string, edgeCosts [][]float64, weights []float64) {
	b.Run(fmt.Sprintf("graph-choose-%d", k), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := g.Solve(k, weights)
			assert.NoError(b, err)
		}
	})
}

func benchBruteForce(b *testing.B, n, k int, vertices []string, edgeCosts [][]float64, weights []float64) {
	b.Run(fmt.Sprintf("bf-%d-choose-%d", n, k), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bf := &BruteForcer{Vertices: vertices, EdgeCosts: edgeCosts}
			_, _, err := bf.Solve(k, weights)
			assert.NoError(b, err)
		}
	})
}

func testData(n int) ([]string, [][]float64, []float64) {
	vertices := make([]string, n)
	for i := range vertices {
		vertices[i] = fmt.Sprintf("%02x", i)
	}

	edgeCosts := make([][]float64, n-1)
	for i := range edgeCosts {
		edgeCosts[i] = make([]float64, i+1)
		for j := range edgeCosts[i] {
			edgeCosts[i][j] = rand.Float64() * 200
		}
	}

	weights := make([]float64, n)
	for i := range weights {
		weights[i] = rand.Float64()
	}

	return vertices, edgeCosts, weights
}

func printMatrix[T constraints.Float](label string, m [][]T, precision int) {
	fmt.Println(label)

	var max T

	for _, row := range m {
		for _, col := range row {
			if col > max {
				max = col
			}
		}
	}

	format := fmt.Sprintf(`%%0%d.%df `, int(math.Ceil(math.Log10(float64(max))))+precision+1, precision)

	for _, row := range m {
		for _, col := range row {
			fmt.Printf(format, col)
		}
		fmt.Println()
	}
	fmt.Println()
}
