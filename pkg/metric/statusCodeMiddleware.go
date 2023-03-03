package metric

import (
	"net/http"

	"k8s.io/klog"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseRecorder(w http.ResponseWriter, r *http.Request) *responseRecorder {
	return &responseRecorder{w, r.Response.StatusCode}
}

func (rr *responseRecorder) WriteHeader(status int) {
	rr.statusCode = status
	rr.ResponseWriter.WriteHeader(status)
}

func InitializeMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rr := NewResponseRecorder(w, r)
		next.ServeHTTP(rr, r)

		statusCode := rr.statusCode
		klog.Info("%d %s", statusCode, http.StatusText(statusCode))

	})
}
