package watchops

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

// Watcher contains the data and methods for an individual url wather service
type Watcher struct {
	// the url that the watcher checks for liveness
	URL string
	// how long to allow an http request to take before terminating
	Timeout time.Duration
	// how long to wait in between checks
	Interval time.Duration
	// a channel that lets the manager cancel a watcher service
	Cancel chan bool
	// a tool for ensuring all watchers close before a manager exits
	WaitGroup *sync.WaitGroup
	// the channel where metrics can be sent for serving
	Metrics chan CheckResult
}

// CheckResult models the data that is returned from checking a url
type CheckResult struct {
	URL string
	// http response code
	Code int
	// total request time
	Duration time.Duration
	// extras
	// time the dns lookup took
	DNS time.Duration
	// time the tls handshake took
	TLS time.Duration
	// how long the http client took to establich a connection
	Conn time.Duration
}

// NewWatcher returns a new Watcher struct
func NewWatcher(url string, interval, timeout time.Duration, wg *sync.WaitGroup, metrics chan CheckResult) *Watcher {
	return &Watcher{
		URL:       url,
		Interval:  interval,
		Timeout:   timeout,
		WaitGroup: wg,
		Cancel:    make(chan bool),
		Metrics:   metrics,
	}
}

// CheckURL performs a GET request on the given url and returns a response code and time
func (w *Watcher) CheckURL(pctx context.Context) (*CheckResult, error) {

	// make sure the connection doesn't take longer than the timeout
	ctx, cancel := context.WithTimeout(pctx, w.Timeout)
	defer cancel()

	req, _ := http.NewRequest("GET", w.URL, nil)

	// track start and duration of some metrics
	var start, connect, dns, tlsHandshake time.Time
	var connDone, dnsDone, tlsDone time.Duration

	// use http tracing to get insight into how long different parts of the request took
	trace := &httptrace.ClientTrace{
		DNSStart: func(dsi httptrace.DNSStartInfo) { dns = time.Now() },
		DNSDone: func(ddi httptrace.DNSDoneInfo) {
			dnsDone = time.Since(dns)
		},

		TLSHandshakeStart: func() { tlsHandshake = time.Now() },
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			tlsDone = time.Since(tlsHandshake)
		},

		ConnectStart: func(network, addr string) { connect = time.Now() },
		ConnectDone: func(network, addr string, err error) {
			connDone = time.Since(connect)

		},
	}

	// at the trace and cancellable context to the request
	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	start = time.Now()
	// send request
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		if fmt.Sprintf("%T", err) == "context.deadlineExceededError" {
			log.Println(w.URL, "check exceeded timeout duration")
			return &CheckResult{URL: w.URL}, nil
		}
		return nil, err
	}

	return &CheckResult{
		URL:      w.URL,
		Code:     res.StatusCode,
		Duration: time.Since(start),
		DNS:      dnsDone,
		TLS:      tlsDone,
		Conn:     connDone,
	}, nil
}

// Run is the loop where the actual work of the watcher takes place
func (w *Watcher) Run(pctx context.Context) error {
	// use a time.Ticker with the provided poll interval to ensure timely checking of the url
	ticker := time.NewTicker(w.Interval)
	// allow the context used by the url checker to be cancelled
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()
	defer w.WaitGroup.Done()

	for {
		select {
		case <-w.Cancel:
			log.Println("cancel called...stopping watcher for", w.URL)
			return nil
		case <-ticker.C:
			log.Println("checking url: ", w.URL)
			cres, err := w.CheckURL(ctx)
			if err != nil {
				//@todo handle errs that might occur so we don't exit
				return err
			}

			// send metrics to the catching goroutine
			w.Metrics <- *cres
		}
	}
}
