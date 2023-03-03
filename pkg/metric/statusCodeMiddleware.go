package metric

import (
	"net/http"

	"k8s.io/klog"
)

type ResponseRecorder struct {
	http.ResponseWriter
	Status int
}

func InitializeMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rr := NewResponseRecorder(w)
		next.ServeHTTP(rr, r)

		statusCode := rr.Status
		klog.Info("%d %s", statusCode, http.StatusText(statusCode))

	})
}

func NewResponseRecorder(w http.ResponseWriter) *ResponseRecorder {
	return &ResponseRecorder{w, http.StatusOK}
}

func (rr *ResponseRecorder) WriteHeader(statusCode int) {
	rr.Status = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}
