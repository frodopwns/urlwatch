package watchops

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// prometheus metrics interfaces

	// upGauge tracks if a url is up or down based on http status code and response time
	upGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sample_external_url_up",
			Help: "binary indication of url liveness",
		},
		[]string{"url"},
	)

	// responseTime is a gague that tracks the most recent response time
	responseTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sample_external_url_response_ms",
			Help: "response time in ms for last check",
		},
		[]string{"url"},
	)
)

// WatcherManager handles multiple wathcer services allowing them to be run concurrently
type WatcherManager struct {
	Watchers  []*Watcher
	Timeout   time.Duration
	Interval  time.Duration
	WaitGroup *sync.WaitGroup
	Metrics   chan CheckResult
	Port      int
}

// NewWatcherManager validates inputs and returns a new manager
func NewWatcherManager(interval, timeout string, port int) (*WatcherManager, error) {
	// converts strings like 10s for 5m to time.Duration objects
	intervalDuration, err := time.ParseDuration(interval)
	if err != nil {
		return nil, fmt.Errorf("could not parse poll interval: %s", interval)
	}

	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		return nil, fmt.Errorf("could not parse poll timeout: %s", timeout)
	}

	return &WatcherManager{
		Watchers:  []*Watcher{},
		Metrics:   make(chan CheckResult),
		Interval:  intervalDuration,
		Timeout:   timeoutDuration,
		WaitGroup: &sync.WaitGroup{},
		Port:      port,
	}, nil
}

// AddWatcher creates a new Watcher from a url and registers it
func (wm *WatcherManager) AddWatcher(ctx context.Context, url string) error {
	wm.Watchers = append(wm.Watchers, NewWatcher(
		url,
		wm.Interval,
		wm.Timeout,
		wm.WaitGroup,
		wm.Metrics,
	))

	return nil
}

// WaitAndWatch starts the watchers that have been registered and waits until they all close.
func (wm *WatcherManager) WaitAndWatch(ctx context.Context) error {

	// start prometheus server in the background
	wm.WaitGroup.Add(1)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", wm.Port),
		Handler: nil,
	}
	go func() {
		srv.RegisterOnShutdown(func() {
			wm.WaitGroup.Done()
		})
		// NewRegistry clears the default go metrics
		r := prometheus.NewRegistry()
		// tell prometheus about the metrics defined above
		r.MustRegister(upGauge, responseTime)
		handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

		http.Handle("/metrics", handler)
		err := srv.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()

	// start all the registered watchers
	for _, w := range wm.Watchers {
		// increment wait group to track the thread
		wm.WaitGroup.Add(1)
		go w.Run(ctx)
	}

	// start the loop that receives metrics from the watchers
	wm.WaitGroup.Add(1)
	go wm.MetricsCatcher()

	// listen to signals so we can clean up processes before closing...possibly unnecessary
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			// sig is a ^C, handle it
			log.Println(sig, "signal received...stopping watchers")
			wm.Stop()
			srv.Shutdown(ctx)
		}
	}()

	// wait here until all watchers have called WaitGroup.Done()
	wm.WaitGroup.Wait()

	return nil
}

// Stop cancels all the watchers and closes the metrics channel
func (wm *WatcherManager) Stop() {
	for _, w := range wm.Watchers {
		w.Cancel <- true
	}
	close(wm.Metrics)
}

// MetricsCatcher watches the metrics channel for data sent from watchers.
// When it receives data it updates the metrics accordingly.
func (wm *WatcherManager) MetricsCatcher() {
	defer wm.WaitGroup.Done()
	for {
		select {
		case m, ok := <-wm.Metrics:
			// exit when the channel closes
			if !ok {
				return
			}

			// if the code is 200 we set the urlUp gauge to 1, otherwise 0
			if m.Code == 200 {
				upGauge.WithLabelValues(m.URL).Set(1)
			} else {
				upGauge.WithLabelValues(m.URL).Set(0)
			}

			// set the response time
			responseTime.WithLabelValues(m.URL).Set(float64(m.Duration.Milliseconds()))
		}
	}
}
