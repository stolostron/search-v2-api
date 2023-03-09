package metric

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// prometheusMiddleware implements mux.MiddlewareFunc.
func ExposeMetrics(next http.Handler) http.Handler {
	queryType1 := "http_request_duration_total" // TODO: Need to extract this from the request.
	duration, _ := HttpDuration.CurryWith(prometheus.Labels{"query_type": queryType1})
	queryType2 := "http_requests_total" // TODO: Need to extract this from the request.
	requestTotal, _ := HttpRequestTotal.CurryWith(prometheus.Labels{"query_type": queryType2})

	return promhttp.InstrumentHandlerDuration(duration,
		promhttp.InstrumentHandlerCounter(requestTotal, next))
}

// 	return promhttp.InstrumentHandlerDuration(
// 		HttpDuration.MustCurryWith(prometheus.Labels{"status_code": "", "action": "serve_http_request"}),
// 		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

// 			route := mux.CurrentRoute(r)
// 			path, _ := route.GetPathTemplate()

// 			klog.V(4).Infof("URL Referrer: %s", r.Referer())
// 			klog.V(4).Infof("User Agent: %s", r.UserAgent())

// 			//with status code:
// 			// rr := NewResponseRecorder(w, r)
// 			// status := rr.statusCode
// 			// timer := prometheus.NewTimer(HttpDuration.WithLabelValues(strconv.Itoa(200), "serve_http_request"))
// 			// defer timer.ObserveDuration()

// 			HttpRequestTotal.WithLabelValues(path, r.Method).Inc()
// 			// This will run at the end of the middleware chain.

// 			rr := NewResponseRecorder(w)

// 			defer func() {
// 				klog.Infof("At prom middleware AFTER processing the request. response_code  %+v", rr.statusCode)

// 				HttpDurationByQuery := HttpDurationByLabels(prometheus.Labels{"action": "serve_http_request"})
// 				HttpDurationByQuery.WithLabelValues(strconv.Itoa(rr.statusCode))
// 			}()

// 			next.ServeHTTP(rr, r)

// 			// rr := NewResponseRecorder(w, r)
// 			// status := rr.statusCode

// 		}))
// }
