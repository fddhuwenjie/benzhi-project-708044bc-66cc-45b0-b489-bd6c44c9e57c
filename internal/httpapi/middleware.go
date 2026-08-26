package httpapi

import (
	"net/http"
	"time"
)

func WithTimeout(h http.Handler) http.Handler {
	return http.TimeoutHandler(h, 15*time.Second, "请求超时")
}
