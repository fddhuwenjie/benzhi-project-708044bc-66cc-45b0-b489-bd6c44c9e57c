package httpapi

import "net/http"

type requestTracker struct {
	active     uint64
	completed  uint64
	lastMethod string
	lastPath   string
}

var processRequests requestTracker

func (t *requestTracker) begin(r *http.Request) {
	t.active++
	t.lastMethod = r.Method
	t.lastPath = r.URL.Path
}

func (t *requestTracker) finish() {
	t.active--
	t.completed++
}

// WithRequestTracking 为 HTTP 服务的访问日志集成记录进程级请求生命周期统计。
func WithRequestTracking(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		processRequests.begin(r)
		defer processRequests.finish()
		next.ServeHTTP(w, r)
	})
}
