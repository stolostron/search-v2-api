// Copyright Contributors to the Open Cluster Management project

package metric

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/klog/v2"
)

// Redirect and capture the logger output.
func captureLogOutput() func() string {
	var buf bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)

	return func() string {
		klog.SetOutput(os.Stderr)
		return buf.String()
	}
}

// Should use default value when passed duration is 0.
func Test_SlowLog_Default(t *testing.T) {
	// Capture the logger output for verification.
	stop := captureLogOutput()

	// Set DEFAULT_SLOW_LOG to 2ms
	DEFAULT_SLOW_LOG = 2 * time.Millisecond

	endFn := SlowLog("MockFunction", 0)
	time.Sleep(5 * time.Millisecond) // nolint:staticcheck //lint:ignore SA1004
	endFn()

	// Verify log was called.
	logBuf := stop()
	if !strings.Contains(logBuf, "MockFunction") {
		t.Error("Expected SlowLog to write to log buffer. Received: ", logBuf)
	}
}

func Test_SlowLog_CustomDuration(t *testing.T) {
	// Capture the logger output for verification.
	stop := captureLogOutput()

	endFn := SlowLog("MockFunction2", 3*time.Millisecond)
	time.Sleep(5 * time.Millisecond) // nolint:staticcheck //lint:ignore SA1004
	endFn()

	// Verify log is written.
	logBuf := stop()
	if !strings.Contains(logBuf, "MockFunction2") {
		t.Error("Expected SlowLog to write to log buffer. Received: ", logBuf)
	}
}

func Test_SlowLog_ThresholdNotMet(t *testing.T) {
	// Capture the logger output for verification.
	stop := captureLogOutput()

	endFn := SlowLog("MockFunction_DontLog", 5*time.Millisecond)
	time.Sleep(1 * time.Millisecond) // nolint:staticcheck //lint:ignore SA1004
	endFn()

	// Verify log is not written.
	logBuf := stop()
	if logBuf != "" {
		t.Error("Expected log buffer to be empty. Received: ", logBuf)
	}
}
