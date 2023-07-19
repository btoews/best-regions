package regions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func TestLatencyTracker(t *testing.T) {
	var app func(w http.ResponseWriter, r *http.Request)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app(w, r) }))
	t.Cleanup(srv.Close)

	t.Run("base case", func(t *testing.T) {
		lt := NewLatencyTracker(srv.URL, 10, time.Second)

		assert.Equal(t, 0, lt.sma)
		assert.Equal(t, 0, lt.nLocked())

		app = func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }

		assert.NoError(t, lt.doRequest(context.Background()))
		assert.True(t, lt.sma < time.Millisecond)
		assert.Equal(t, 1, lt.nLocked())

		firstSMA := lt.sma
		t.Logf("first sma=%v", firstSMA)

		app = func(w http.ResponseWriter, r *http.Request) { time.Sleep(6 * time.Millisecond); w.Write([]byte("{}")) }

		assert.NoError(t, lt.doRequest(context.Background()))
		assert.True(t, lt.sma < 4*time.Millisecond)
		assert.True(t, lt.sma > 6*time.Millisecond/2)
		assert.Equal(t, 2, lt.nLocked())

		app = func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }

		for i := 3; i <= 10; i++ {
			assert.NoError(t, lt.doRequest(context.Background()))
			assert.Equal(t, i, lt.nLocked())
		}
		expected := time.Duration(float64(6*time.Millisecond)*0.1 + float64(firstSMA)*0.9)
		assert.True(t, lt.sma > expected/2, "sma=%v expected=%v", lt.sma, expected)
		assert.True(t, lt.sma < expected*2, "sma=%v expected=%v", lt.sma, expected)

		for i := 0; i < 1000; i++ {
			assert.NoError(t, lt.doRequest(context.Background()))
			assert.Equal(t, 10, lt.nLocked())
		}
		assert.True(t, lt.sma < expected*2, "sma=%v expected=%v", lt.sma, firstSMA)
	})

	t.Run("control", func(t *testing.T) {
		lt := NewLatencyTracker(srv.URL, 10, 2*time.Millisecond)

		app = func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }
		errc := lt.Run()

		done := make(chan struct{})
		go func() {
			defer close(done)
			err, closed := <-errc
			assert.False(t, closed, "unexpected err on errc: %v", err)
		}()

		time.Sleep(30 * time.Millisecond)

		assert.True(t, lt.sma < time.Millisecond, "%v>time.Millisecond", lt.sma)
		assert.Equal(t, 10, lt.nLocked())

		lt.Stop()
		select {
		case <-done:
		case <-time.After(time.Millisecond):
			t.Fatal("stop didn't close errc")
		}

		assert.True(t, lt.sma < time.Millisecond, "%v>time.Millisecond", lt.sma)
		assert.Equal(t, 10, lt.nLocked())
	})

	t.Run("slow server", func(t *testing.T) {
		lt := NewLatencyTracker(srv.URL, 10, 2*time.Millisecond)

		app = func(w http.ResponseWriter, r *http.Request) { time.Sleep(3 * time.Millisecond) }
		errc := lt.Run()

		for i := 0; i < 10; i++ {
			select {
			case err, ok := <-errc:
				assert.True(t, ok)
				assert.Error(t, err)
			case <-time.After(10 * time.Millisecond):
				t.Fatal("slow errc")
			}
		}

		lt.Stop()
		select {
		case err, ok := <-errc:
			assert.False(t, ok, "unexpected err on errc: %v", err)
		case <-time.After(time.Millisecond):
			t.Fatal("stop didn't close errc")
		}
	})

	t.Run("server error", func(t *testing.T) {
		lt := NewLatencyTracker(srv.URL, 10, 2*time.Millisecond)

		app = func(w http.ResponseWriter, r *http.Request) {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		}

		errc := lt.Run()

		for i := 0; i < 10; i++ {
			select {
			case err, ok := <-errc:
				assert.True(t, ok)
				assert.Error(t, err)
			case <-time.After(10 * time.Millisecond):
				t.Fatal("slow errc")
			}
		}

		lt.Stop()
		select {
		case err, ok := <-errc:
			assert.False(t, ok, "unexpected err on errc: %v", err)
		case <-time.After(time.Millisecond):
			t.Fatal("stop didn't close errc")
		}
	})
}
