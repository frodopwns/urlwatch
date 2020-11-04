package watchops

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestClientTimeout ensures if the GET to the url being checked takes longer than the timeout it will be handled gracefully
func TestClientTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(105 * time.Millisecond)
	}))
	defer ts.Close()

	wg := &sync.WaitGroup{}
	metrics := make(chan CheckResult)
	interval := 10 * time.Second
	timeout := 20 * time.Millisecond
	watcher := NewWatcher(ts.URL, interval, timeout, wg, metrics)

	res, err := watcher.CheckURL(context.Background())
	if err != nil {
		t.Error("Request error", err)
		return
	}

	if res.Code != 0 {
		t.Errorf("response code should be 0 in case of timeouts: %d", res.Code)
	}

}

// TestClientCheckURL ensures that checking a url returns expected output
func TestClientCheckURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(105 * time.Millisecond)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	wg := &sync.WaitGroup{}
	metrics := make(chan CheckResult)
	interval := 10 * time.Second
	timeout := 400 * time.Millisecond
	watcher := NewWatcher(ts.URL, interval, timeout, wg, metrics)

	res, err := watcher.CheckURL(context.Background())
	if err != nil {
		t.Error("Request error", err)
		return
	}

	if res.Duration < (100 * time.Millisecond) {
		t.Errorf("duration metric incorrect, should be greater than 100 miliseconds: %v", res.Duration)
	}

	if res.Code != http.StatusBadGateway {
		t.Errorf("response code should be %d: %d", http.StatusBadGateway, res.Code)
	}

}

// TestWatcherRun ensures a watcher will run properly in a goroutine and cancel as it should
func TestWatcherRun(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		hits++
	}))
	defer ts.Close()

	wg := &sync.WaitGroup{}
	metrics := make(chan CheckResult, 4)
	interval := 500 * time.Millisecond
	timeout := 2 * time.Second
	watcher := NewWatcher(ts.URL, interval, timeout, wg, metrics)

	watcher.WaitGroup.Add(1)
	go watcher.Run(context.Background())
	time.Sleep(2 * time.Second)
	watcher.Cancel <- true

	assert.Equalf(t, hits, 4, "watcher should have checked 4 times: %d", hits)
	assert.Len(t, metrics, 4, "should be 4 items in the metrics channel")
}
