package metric

import (
	"net/http"
	"strconv"

	klog "k8s.io/klog/v2"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// type Query struct {
// 	Name       string
// 	StartedAt  time.Time
// 	FinishedAt time.Time
// 	Duration   float64
// }

// //HTTP application
// type HTTP struct {
// 	Handler    string
// 	Method     string
// 	StatusCode string
// 	StartedAt  time.Time
// 	FinishedAt time.Time
// 	Duration   float64
// }

// //Started start monitoring the request
// func (q *Query) Started() {
// 	q.StartedAt = time.Now()
// }

// // Finished request
// func (h *HTTP) Finished() {
// 	h.FinishedAt = time.Now()
// 	h.Duration = time.Since(h.StartedAt).Seconds()
// }

// prometheusMiddleware implements mux.MiddlewareFunc.
func ExposeMetrics(next http.Handler) http.Handler {

	return promhttp.InstrumentHandlerDuration(
		HttpDuration.MustCurryWith(prometheus.Labels{"status_code": "", "action": "serve_http_request"}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			route := mux.CurrentRoute(r)
			path, _ := route.GetPathTemplate()

			klog.V(4).Infof("URL Referrer: %s", r.Referer())
			klog.V(4).Infof("User Agent: %s", r.UserAgent())

			klog.Info("hello")
			//with status code:
			// rr := NewResponseRecorder(w, r)
			// status := rr.statusCode
			// timer := prometheus.NewTimer(HttpDuration.WithLabelValues(strconv.Itoa(200), "serve_http_request"))
			// defer timer.ObserveDuration()

			HttpRequestTotal.WithLabelValues(path, r.Method).Inc()
			// This will run at the end of the middleware chain.

			rr := NewResponseRecorder(w, r)

			defer func() {
				klog.Infof("At prom middleware AFTER processing the request. response_code  %+v", rr.statusCode)

				HttpDurationByQuery := HttpDurationByLabels(prometheus.Labels{"action": "serve_http_request"})
				HttpDurationByQuery.WithLabelValues(strconv.Itoa(rr.statusCode))
			}()

			next.ServeHTTP(rr, r)

			// rr := NewResponseRecorder(w, r)
			// status := rr.statusCode

		}))
}
