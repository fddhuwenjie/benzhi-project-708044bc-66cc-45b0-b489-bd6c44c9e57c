package requesttrackerrace_test

import (
	"net/http"
	"net/http/httptest"
	"seedvault/internal/httpapi"
	"sync"
	"testing"
)

func TestConcurrentRequestsTrackLifecycleSafely(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	handler := httpapi.WithRequestTracking(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))

	var requests sync.WaitGroup
	requests.Add(2)
	for _, path := range []string{"/healthz", "/readyz"} {
		path := path
		go func() {
			defer requests.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		}()
	}

	<-entered
	<-entered
	close(release)
	requests.Wait()
}
