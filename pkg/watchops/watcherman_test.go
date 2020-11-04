package watchops

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestWatcherManagerValidation checks that invalid time durations cannot be passed to watcher managers
func TestWatcherManagerValidation(t *testing.T) {
	_, err := NewWatcherManager("asdf", "1s", 80)
	assert.Error(t, err, "NewWatcherManager() should have failed with interval validation error")

	_, err = NewWatcherManager("4ms", "ss43", 80)
	assert.Error(t, err, "NewWatcherManager() should have failed with timeout validation error")
}

// TestAddWatcher ensures adding a watcher results in a new watcher in the Watchers list
func TestAddWatcher(t *testing.T) {
	watcherman, err := NewWatcherManager("1s", "1s", 8080)
	assert.NoError(t, err, "NewWatcherManager() should not have failed")

	watcherman.AddWatcher(context.Background(), "http://example.com")
	assert.Len(t, watcherman.Watchers, 1, "incorrect number of watchers in slice after add")
}

// TestWaitAndWatch ensures watchers running in a watcher manager properly report their results
// and that those results can be accessed at the /metrics endpoint
func TestWaitAndWatch(t *testing.T) {
	ctx := context.Background()
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		hits++
		// w.WriteHeader(http.StatusBadGazteway)
	}))
	defer ts.Close()

	watcherman, err := NewWatcherManager("200ms", "2s", 8080)
	assert.NoError(t, err, "NewWatcherManager() should not have failed")

	watcherman.AddWatcher(ctx, ts.URL)

	go watcherman.WaitAndWatch(ctx)
	time.Sleep(500 * time.Millisecond)

	res, err := http.Get("http://localhost:8080/metrics")
	assert.NoError(t, err, "failed requesting metrics endpoint")

	data, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	assert.NoError(t, err, "failed opening metrics response body")

	needle := `sample_external_url_response_ms{url="http:\/\/127.0.0.1:\d+"} 10\d`
	assert.Regexp(t, needle, string(data))

	needle = fmt.Sprintf(`sample_external_url_up{url="%s"} 1`, ts.URL)
	assert.Contains(t, string(data), needle, "metrics response did not contain expected data")

	// t.Log(string(data))

	if res.StatusCode != 200 {
		t.Errorf("unexpected response code from metrics server: %d", res.StatusCode)
	}

	watcherman.Stop()

	assert.Equal(t, hits, 2, "only 2 hits should have occured")

	res, err = http.Get("http://localhost:8080/metrics")
	assert.NoError(t, err, "failed requesting metrics endpoint")
	t.Log(res.StatusCode)

}
