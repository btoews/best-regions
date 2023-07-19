package regions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	LatencyPath   = "/latency.json"
	LatenciesPath = "/latencies.json"
	StatsPath     = "/stats.json"
)

var (
	EnvFlyApp    = os.Getenv("FLY_APP_NAME")
	EnvFlyRegion = os.Getenv("FLY_REGION")
)

func DeployedRegions(ctx context.Context) ([]string, error) {
	records, err := dns.LookupTXT(ctx, name("regions", EnvFlyApp, "internal"))
	if err != nil {
		return nil, err
	}

	ret := []string{}
	for _, record := range records {
		ret = append(ret, strings.Split(record, ",")...)
	}

	return ret, nil
}

type RegionLatencyTracker struct {
	trackers  map[string]*LatencyTracker
	smaWindow int
	interval  time.Duration
	stop      chan struct{}
	m         sync.Mutex
}

func NewRegionLatencyTracker(smaWindow int, interval time.Duration) *RegionLatencyTracker {
	return &RegionLatencyTracker{
		trackers:  map[string]*LatencyTracker{},
		smaWindow: smaWindow,
		interval:  interval,
		stop:      make(chan struct{}),
	}
}

func (rlt *RegionLatencyTracker) Run() <-chan error {
	errc := make(chan error)

	go func() {
		defer close(errc)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-rlt.stop
			cancel()
		}()

		tkr := time.NewTicker(rlt.interval)
		defer tkr.Stop()

		for {
			rlt.updateRegions(ctx, errc)
			select {
			case <-tkr.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	return errc
}

func (rlt *RegionLatencyTracker) updateRegions(ctx context.Context, errc chan error) {
	ctx, cancel := context.WithTimeout(ctx, rlt.interval)
	defer cancel()

	regions, err := DeployedRegions(ctx)
	if errors.Is(err, context.Canceled) {
		return
	} else if err != nil {
		sendErr(errc, fmt.Errorf("region tracker: %w", err))
		return
	}

	rlt.m.Lock()
	defer rlt.m.Unlock()

	// check that context didn't close while waiting for mutex
	if err := ctx.Err(); errors.Is(err, context.Canceled) {
		return
	} else if err != nil {
		sendErr(errc, fmt.Errorf("region tracker: %w", err))
		return
	}

	rmap := make(map[string]bool, len(regions))
	for _, region := range regions {
		if region == EnvFlyRegion {
			continue
		}

		// new region?
		if _, exists := rlt.trackers[region]; !exists {
			url := "http://" + name(region, EnvFlyApp, "internal")
			tracker := NewLatencyTracker(url, rlt.smaWindow, rlt.interval)
			rlt.trackers[region] = tracker

			go func() {
				for err := range tracker.Run() {
					errc <- fmt.Errorf("%s tracker: %w", region, err)
				}
			}()
		}
		rmap[region] = true
	}

	for region, tracker := range rlt.trackers {
		// removed region?
		if _, exists := rmap[region]; !exists {
			tracker.Stop()
			delete(rlt.trackers, region)
		}
	}
}

func (rlt *RegionLatencyTracker) Latencies() map[string]map[string]int {
	rlt.m.Lock()
	defer rlt.m.Unlock()

	ret := make(map[string]map[string]int, len(rlt.trackers)+1)

	for region, tracker := range rlt.trackers {
		ret[region] = tracker.Latencies()
	}

	ret[EnvFlyRegion] = rlt.latencyLocked()

	return ret
}

func (rlt *RegionLatencyTracker) Latency() map[string]int {
	rlt.m.Lock()
	defer rlt.m.Unlock()
	return rlt.latencyLocked()
}

func (rlt *RegionLatencyTracker) latencyLocked() map[string]int {
	ret := make(map[string]int, len(rlt.trackers))

	for region, tracker := range rlt.trackers {
		ret[region] = tracker.Latency()
	}

	return ret
}

func (rlt *RegionLatencyTracker) Stop() {
	rlt.m.Lock()
	defer rlt.m.Unlock()

	close(rlt.stop)

	for region, tracker := range rlt.trackers {
		tracker.Stop()
		delete(rlt.trackers, region)
	}
}

func sendErr(errc chan error, err error) {
	select {
	case errc <- err:
	default:
	}
}

func name(parts ...string) string {
	return strings.Join(parts, ".")
}
