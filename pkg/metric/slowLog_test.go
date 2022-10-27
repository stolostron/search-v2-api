// Copyright Contributors to the Open Cluster Management project

package metric

import (
	"testing"
	"time"
	// klog "k8s.io/klog/v2"
)

// Should use default value when environment variable does not exist.
func Test_SlowLog_Default(t *testing.T) {
	// Mock console

	// Set env SLOW_LOG

	endFn := SlowLog("Test", 0)
	// nolint:staticcheck //lint:ignore SA1004
	time.Sleep(100)
	endFn()

	// Verify console log is called.
}

func Test_SlowLog_CustomDuration(t *testing.T) {
	// Mock console

	endFn := SlowLog("Test", 5*time.Millisecond)
	// nolint:staticcheck //lint:ignore SA1004
	time.Sleep(100)
	endFn()

	// Verify console is called.
}

func Test_SlowLog_ThreasholdNotMet(t *testing.T) {
	// Mock console

	endFn := SlowLog("Test", 5*time.Millisecond)
	// nolint:staticcheck //lint:ignore SA1004
	time.Sleep(1)
	endFn()

	// Verify console log not called.
}
