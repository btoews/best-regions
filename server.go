package regions

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultSMAWindow = 100
	defaultInterval  = 30 * time.Second
)

type Server struct {
	srv       *http.Server
	rlt       *RegionLatencyTracker
	data      map[string][]byte
	reqCounts map[string]*uint64
	stopOnce  sync.Once
	stop      chan struct{}
	log       *log.Logger
	m         sync.RWMutex
}

func NewServer(smaWindow int, interval time.Duration, mux *http.ServeMux) *Server {
	if smaWindow == 0 {
		smaWindow = defaultSMAWindow
	}
	if interval == 0 {
		interval = defaultInterval
	}
	if mux == nil {
		mux = http.DefaultServeMux
	}

	s := &Server{
		rlt:  NewRegionLatencyTracker(smaWindow, interval),
		data: map[string][]byte{},
		reqCounts: map[string]*uint64{
			LatenciesPath: new(uint64),
			LatencyPath:   new(uint64),
			StatsPath:     new(uint64),
		},
		stop: make(chan struct{}),
		log:  log.New(io.Discard, "", 0),
	}

	mux.Handle(LatenciesPath, s.serveData(LatenciesPath))
	mux.Handle(LatencyPath, s.serveData(LatencyPath))
	mux.Handle(StatsPath, s.serveData(StatsPath))

	s.srv = &http.Server{Addr: ":80", Handler: mux}

	return s
}

func (s *Server) serveData(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.m.RLock()
		data, ok := s.data[path]
		s.m.RUnlock()

		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		s.incrReqCount(r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
}

func (s *Server) LogOutput(w io.Writer) {
	s.log.SetOutput(w)
}

func (s *Server) Latencies() map[string]map[string]int {
	return s.rlt.Latencies()
}

func (s *Server) Run() error {
	go s.runRLT()
	go s.updateData()
	if err := s.srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Println("graceful shutdown")
	s.stopOnce.Do(func() { close(s.stop) })
	if err := s.srv.Shutdown(ctx); err != nil {
		s.log.Println(err)
		return err
	}
	return nil
}

func (s *Server) Close() error {
	s.log.Println("immediate shutdown")
	s.stopOnce.Do(func() { close(s.stop) })
	if err := s.srv.Close(); err != nil {
		s.log.Println(err)
		return err
	}
	return nil
}

func (s *Server) updateData() {
	tkr := time.NewTicker(time.Second)
	defer tkr.Stop()

	for {
		latencies := s.rlt.Latencies()

		if j, err := json.MarshalIndent(latencies, "", "  "); err != nil {
			s.log.Printf("json: %s", err)
		} else {
			s.m.Lock()
			s.data[LatenciesPath] = j
			s.m.Unlock()
		}

		if j, err := json.MarshalIndent(latencies[EnvFlyRegion], "", "  "); err != nil {
			s.log.Printf("json: %s", err)
		} else {
			s.m.Lock()
			s.data[LatencyPath] = j
			s.m.Unlock()
		}

		stats := map[string]uint64{}
		for path, ptr := range s.reqCounts {
			stats[path] = atomic.LoadUint64(ptr)
		}
		if j, err := json.MarshalIndent(stats, "", "  "); err != nil {
			s.log.Printf("json: %s", err)
		} else {
			s.m.Lock()
			s.data[StatsPath] = j
			s.m.Unlock()
		}

		select {
		case <-tkr.C:
		case <-s.stop:
			return
		}
	}
}

func (s *Server) runRLT() {
	errc := s.rlt.Run()
	defer s.rlt.Stop()

	for {
		select {
		case err := <-errc:
			s.log.Println(err)
		case <-s.stop:
			return
		}
	}
}

func (s *Server) incrReqCount(path string) {
	atomic.AddUint64(s.reqCounts[path], 1)
}
