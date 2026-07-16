// Copyright Contributors to the Open Cluster Management project

package metrics

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/klog/v2"
)

// captureLogOutput redirects klog to an in-memory buffer and returns a stop
// function that restores the full klog routing state (both the output writer
// and the toStderr flag) and returns everything that was written.
func captureLogOutput() func() string {
	var buf bytes.Buffer
	klog.LogToStderr(false) // stop writing to stderr
	klog.SetOutput(&buf)    // redirect all output to buffer

	return func() string {
		klog.SetOutput(os.Stderr) // restore writer to stderr
		klog.LogToStderr(true)    // re-enable stderr routing
		return buf.String()
	}
}

// Test_SlowLog_Default verifies that DEFAULT_SLOW_LOG is used when no custom
// duration is supplied (logAfter == 0).
func Test_SlowLog_Default(t *testing.T) {
	// Restore the package-level default after this test so it does not leak
	// into other tests running in the same binary.
	orig := DEFAULT_SLOW_LOG
	t.Cleanup(func() { DEFAULT_SLOW_LOG = orig })

	DEFAULT_SLOW_LOG = 10 * time.Millisecond

	stop := captureLogOutput()

	endFn := SlowLog("MockFunction", 0)
	time.Sleep(100 * time.Millisecond) // 10× the threshold — always exceeds it
	endFn()

	logBuf := stop()
	if !strings.Contains(logBuf, "MockFunction") {
		t.Error("Expected SlowLog to write to log buffer. Received: ", logBuf)
	}
}

// Test_SlowLog_CustomDuration verifies that a caller-supplied duration
// overrides DEFAULT_SLOW_LOG.
func Test_SlowLog_CustomDuration(t *testing.T) {
	// Restore the package-level default after this test so it does not leak
	// into other tests running in the same binary.
	orig := DEFAULT_SLOW_LOG
	t.Cleanup(func() { DEFAULT_SLOW_LOG = orig })

	// Set DEFAULT_SLOW_LOG to a very long value so the test cannot pass via
	// the fallback path — only the custom threshold should trigger logging.
	DEFAULT_SLOW_LOG = 10 * time.Second

	stop := captureLogOutput()

	endFn := SlowLog("MockFunction2", 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // 10× the custom threshold — always exceeds it
	endFn()

	logBuf := stop()
	if !strings.Contains(logBuf, "MockFunction2") {
		t.Error("Expected SlowLog to write to log buffer. Received: ", logBuf)
	}
}

// Test_SlowLog_ThresholdNotMet verifies that no log is written when elapsed
// time is below the custom threshold, regardless of DEFAULT_SLOW_LOG.
func Test_SlowLog_ThresholdNotMet(t *testing.T) {
	// Restore the package-level default after this test so it does not leak
	// into other tests running in the same binary.
	orig := DEFAULT_SLOW_LOG
	t.Cleanup(func() { DEFAULT_SLOW_LOG = orig })

	// Set DEFAULT_SLOW_LOG to a tiny value to ensure the test would catch a
	// regression where the custom threshold is incorrectly OR-ed with the
	// default (i.e., the bug we fixed in slowLog.go).
	DEFAULT_SLOW_LOG = 1 * time.Microsecond

	stop := captureLogOutput()

	// Custom threshold: 1 hour.  Even extreme CI descheduling cannot push the
	// elapsed time above this, so the log must always remain empty.
	// (500 ms was theoretically reachable on a severely loaded runner.)
	endFn := SlowLog("MockFunction_DontLog", time.Hour)
	endFn()

	logBuf := stop()
	if logBuf != "" {
		t.Error("Expected log buffer to be empty. Received: ", logBuf)
	}
}
