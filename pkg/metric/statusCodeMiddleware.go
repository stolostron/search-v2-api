package metric

import (
	"net/http"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{w, 0}
}

func (rr *responseRecorder) WriteHeader(status int) {
	rr.statusCode = status
	rr.ResponseWriter.WriteHeader(status)
}

// func InitializeMetrics(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

// 		rr := NewResponseRecorder(w)
// 		next.ServeHTTP(rr, r)

// 		statusCode := rr.statusCode
// 		klog.Info("%d %s", statusCode, http.StatusText(statusCode))

// 	})
// }
