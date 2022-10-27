// Copyright Contributors to the Open Cluster Management project
package metric

import (
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

var DEFAULT_SLOW_LOG = time.Duration(config.Cfg.SlowLog) * time.Millisecond

// Record the time when a function starts and logs if the function takes more than the expected duration.
// This function should be invoked with defer.
func SlowLog(funcName string, after time.Duration) func() {
	klog.V(7).Infof("ENTERING: %s", funcName)
	start := time.Now()

	// This part gets executed when the function exits.
	return func() {
		if (after > 0 && time.Since(start) > after) || (time.Since(start) > DEFAULT_SLOW_LOG) {
			klog.Warningf("%s - %s", time.Since(start), funcName)
		}

		// We could emit metric here, but it could be too much data.

		klog.V(7).Infof("EXITING: %s", funcName)
	}
}
