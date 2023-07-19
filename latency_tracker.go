package regions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

type LatencyTracker struct {
	url           string
	smaWindow     int
	sma           time.Duration
	smaPos        int
	smaData       []time.Duration
	hostLatencies map[string]int
	interval      time.Duration
	stop          chan struct{}
	m             sync.RWMutex
}

func NewLatencyTracker(baseURL string, smaWindow int, interval time.Duration) *LatencyTracker {
	return &LatencyTracker{
		url:       baseURL + LatencyPath,
		smaWindow: smaWindow,
		smaData:   make([]time.Duration, smaWindow),
		interval:  interval,
		stop:      make(chan struct{}),
	}
}

func (lt *LatencyTracker) Run() <-chan error {
	errc := make(chan error)

	go func() {
		defer close(errc)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-lt.stop
			cancel()
		}()

		tkr := time.NewTicker(lt.interval)
		defer tkr.Stop()

		for {
			if err := lt.doRequest(ctx); errors.Is(err, context.Canceled) {
				return
			} else if err != nil {
				sendErr(errc, err)
			}

			select {
			case <-tkr.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	return errc
}

func (lt *LatencyTracker) doRequest(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, lt.interval)
	defer cancel()

	// try to measure single round trip by looking at interval between
	// finishing sending request and starting to read response.
	var start, end time.Time
	tctx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteRequest:         func(wri httptrace.WroteRequestInfo) { start = time.Now() },
		GotFirstResponseByte: func() { end = time.Now() },
	})

	req, err := http.NewRequestWithContext(tctx, http.MethodGet, lt.url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	hl := map[string]int{}
	if err := json.NewDecoder(resp.Body).Decode(&hl); err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return err
	}

	switch {
	case start.IsZero():
		return errors.New("zero start")
	case end.IsZero():
		return errors.New("zero end")
	}

	lt.update(end.Sub(start), hl)

	return nil
}

func (lt *LatencyTracker) Latency() int {
	lt.m.RLock()
	defer lt.m.RUnlock()

	if lt.nLocked() == 0 {
		return math.MaxInt
	}

	return int(lt.sma / time.Millisecond)
}

func (lt *LatencyTracker) Latencies() map[string]int {
	lt.m.RLock()
	defer lt.m.RUnlock()

	return lt.hostLatencies
}

func (lt *LatencyTracker) update(dur time.Duration, hostLatencies map[string]int) {
	lt.m.Lock()
	defer lt.m.Unlock()

	lt.hostLatencies = hostLatencies

	lt.smaData[lt.smaPos%lt.smaWindow] = dur
	lt.smaPos += 1

	var (
		n   = lt.nLocked()
		sum time.Duration
	)
	for i := 0; i < n; i++ {
		sum += lt.smaData[i]
	}
	lt.sma = sum / time.Duration(n)
}

func (lt *LatencyTracker) nLocked() int {
	if lt.smaPos < lt.smaWindow {
		return lt.smaPos
	}
	return lt.smaWindow
}

func (lt *LatencyTracker) Stop() {
	close(lt.stop)
}
