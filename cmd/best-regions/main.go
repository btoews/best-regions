package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	regions "github.com/btoews/best-regions"
	"github.com/btoews/best-regions/graph"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

func main() {
	mux := new(http.ServeMux)

	s := regions.NewServer(0, 0, mux)
	s.LogOutput(os.Stderr)

	go func() {
		if err := s.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}()

	m := &model{s: s, stop: make(chan struct{})}
	go m.run()

	mux.Handle("/", handler(m))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// wait for first signal
	<-ctx.Done()

	close(m.stop)

	// abort graceful shutdown on second signal
	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if s.Shutdown(ctx) != nil {
		s.Close()
	}
}

func handler(m *model) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(Index)
			return
		case http.MethodPost:
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.Error(w, "🤦‍♂️", http.StatusNotFound)
			return
		}

		m.m.RLock()
		bf := m.bf
		g := m.g
		m.m.RUnlock()

		w.Header().Set("Content-Type", "application/json")

		pd, err := readPromData(r.Body)
		if errJSON(w, "readPromData", err) {
			return
		}
		weights := pd.weights(bf.Vertices)

		results := Results{}

		if ur := pd.unknownRegions(bf.Vertices); len(ur) != 0 {
			results.Error = fmt.Sprintf("unknown regions: %s", strings.Join(ur, ", "))
		}

		if paramK := r.URL.Query().Get("k"); paramK != "" {
			k64, err := strconv.ParseInt(paramK, 10, 8)
			if errJSON(w, "parse k", err) {
				return
			}
			k := int(k64)

			if nv := len(bf.Vertices); k < 1 || k > nv {
				errJSON(w, "", fmt.Errorf("k must be in [1 %d]", nv))
				return
			}

			var (
				cost  float64
				combo []string
			)

			if k < 4 {
				if cost, combo, err = bf.Solve(k, weights); errJSON(w, "solve (bf)", err) {
					return
				}
			} else {
				if cost, combo, err = g.Solve(k, weights); errJSON(w, "solve (graph)", err) {
					return
				}
			}

			results.Results = append(results.Results, Result{Regions: combo, Cost: cost})
		}

		for _, paramCompare := range r.URL.Query()["compare"] {
			combo := strings.Split(paramCompare, ",")
			for i := range combo {
				combo[i] = strings.TrimSpace(combo[i])
			}
			combo = slices.DeleteFunc(combo, func(c string) bool { return c == "" })
			if len(combo) == 0 {
				continue
			}

			cost, err := bf.CombinationCost(combo, weights)
			if errJSON(w, "CombinationCost", err) {
				return
			}
			results.Results = append(results.Results, Result{Regions: combo, Cost: cost})
		}

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")

		if err := enc.Encode(results); err != nil {
			logrus.WithError(err).Warn("writing results")
			return
		}
	})
}

type Results struct {
	Results []Result `json:"results,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type Result struct {
	Regions []string `json:"regions"`
	Cost    float64  `json:"cost"`
}

func errJSON(w http.ResponseWriter, logMsg string, err error) bool {
	if err == nil {
		return false
	}
	if len(logMsg) > 0 {
		logrus.WithError(err).Warn(logMsg)
	}

	w.WriteHeader(http.StatusInternalServerError)

	if werr := json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); werr != nil {
		logrus.WithError(err).Warn("writing error response")
	}

	return true
}

type promData map[string]int

func readPromData(r io.Reader) (promData, error) {
	var pdj promDataJson
	if err := json.NewDecoder(r).Decode(&pdj); err != nil {
		return nil, err
	}

	pd := make(promData, len(pdj.Data.Result))

	for _, res := range pdj.Data.Result {
		if res.Metric.Region == "" {
			logrus.Warn("bad prom data: no region")
			continue
		}

		if l := len(res.Value); l != 2 {
			logrus.Warnf("bad prom data: %d fields in value", l)
			continue
		}

		sv, ok := res.Value[1].(string)
		if !ok {
			logrus.Warnf("bad prom data: %T val", sv)
			continue
		}

		iv, err := strconv.ParseInt(sv, 10, 64)
		if err != nil {
			logrus.WithError(err).Warn("bad prom data: parse val")
			continue
		}

		pd[res.Metric.Region] = int(iv)
	}

	return pd, nil
}

func (pd promData) weights(regions []string) []float64 {
	sum := 0
	for _, r := range regions {
		sum += pd[r]
	}

	ret := make([]float64, len(regions))
	if sum > 0 {
		for i, r := range regions {
			ret[i] = float64(pd[r]) / float64(sum)
		}
	}

	return ret
}

func (pd promData) unknownRegions(knownRegions []string) []string {
	kr := make(map[string]bool, len(knownRegions))
	for _, r := range knownRegions {
		kr[r] = true
	}

	ret := []string{}
	for r, _ := range pd {
		if _, known := kr[r]; !known {
			ret = append(ret, r)
		}
	}

	return ret
}

// data from fly.io prometheus query:
//
//	query=sum(increase(fly_edge_http_responses_count)) by (region)
type promDataJson struct {
	Data struct {
		Result []struct {
			Metric struct {
				Region string `json:"region"`
			} `json:"metric"`
			Value []any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type model struct {
	s    *regions.Server
	g    *graph.Graph
	bf   *graph.BruteForcer
	m    sync.RWMutex
	stop chan struct{}
}

func (m *model) run() {
	tkr := time.NewTicker(time.Second)
	defer tkr.Stop()

runLoop:
	for {
		regionNames, linkCosts := modelParams(m.s.Latencies())
		g, err := graph.NewGraph(regionNames, linkCosts)
		if err != nil {
			logrus.WithError(err).Warn("building graph")
			continue runLoop
		}
		bf := graph.NewBruteForcer(regionNames, linkCosts)

		m.m.Lock()
		m.g = g
		m.bf = bf
		m.m.Unlock()

		select {
		case <-tkr.C:
		case <-m.stop:
			return
		}
	}
}

func modelParams(latencies map[string]map[string]int) ([]string, [][]float64) {
	// collection list of regions from combination of all regions' data in case
	// we're missing any locally
	regionMap := make(map[string]bool, len(latencies))
	for regionName, regionData := range latencies {
		regionMap[regionName] = true
		for regionName := range regionData {
			regionMap[regionName] = true
		}
	}

	regions := maps.Keys(regionMap)
	slices.Sort(regions)

	linkCosts := make([][]float64, len(regions)-1)
	for i := 1; i < len(regions); i++ {
		for j := 0; j < i; j++ {
			ij, haveIJ := latencies[regions[i]][regions[j]]
			ji, haveJI := latencies[regions[j]][regions[i]]
			switch {
			case haveIJ && haveJI:
				linkCosts[i-1] = append(linkCosts[i-1], (float64(ij)+float64(ji))/2)
			case haveIJ:
				linkCosts[i-1] = append(linkCosts[i-1], float64(ij))
			case haveJI:
				linkCosts[i-1] = append(linkCosts[i-1], float64(ji))
			default:
				// no data about cost. assume it's expensive
				linkCosts[i-1] = append(linkCosts[i-1], math.MaxFloat64)
			}
		}
	}

	return regions, linkCosts
}

var (
	ReadMeB64, ScriptB64 string
	Index                = []byte("hello")
)

func init() {
	if ReadMeB64 != "" {
		if rm, err := base64.StdEncoding.DecodeString(ReadMeB64); err != nil {
			panic(err)
		} else {
			Index = []byte("# " + strings.Join(strings.Split(string(rm), "\n"), "\n# ") + "\n\n")
		}
	}
	if ScriptB64 != "" {
		if rm, err := base64.StdEncoding.DecodeString(ScriptB64); err != nil {
			panic(err)
		} else {
			Index = append(Index, rm...)
		}
	}
}
