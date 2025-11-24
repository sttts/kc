package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	klog "k8s.io/klog/v2"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestSetupControllerRuntimeLoggerAlignsVerbosity(t *testing.T) {
	klogState := klog.CaptureState()
	defer klogState.Restore()

	origCtrlLogger := ctrllog.Log
	defer ctrllog.SetLogger(origCtrlLogger)

	origLogWriter := log.Writer()
	origLogFlags := log.Flags()
	defer func() {
		log.SetOutput(origLogWriter)
		log.SetFlags(origLogFlags)
	}()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	switchLogger, logPath := setupControllerRuntimeLogger(3)
	if switchLogger == nil {
		t.Fatalf("expected switch logger callback for verbosity >=1")
	}
	expectedLogPath := filepath.Join(tempHome, ".kc", "debug.log")
	if logPath != expectedLogPath {
		t.Fatalf("expected log path %q, got %q", expectedLogPath, logPath)
	}

	if !ctrllog.Log.V(2).Enabled() {
		t.Fatalf("expected controller-runtime logger to enable V(2)")
	}
	if ctrllog.Log.V(4).Enabled() {
		t.Fatalf("expected controller-runtime logger to disable V(4)")
	}
	if !klog.V(klog.Level(2)).Enabled() {
		t.Fatalf("expected klog V(2) to be enabled")
	}
	if klog.V(klog.Level(4)).Enabled() {
		t.Fatalf("expected klog V(4) to be disabled")
	}

	switchLogger()

	klog.V(klog.Level(2)).InfoS("klog verbosity check", "source", "klog")
	ctrllog.Log.V(2).Info("controller-runtime verbosity check")
	klog.Flush()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "klog verbosity check") {
		t.Fatalf("expected klog output in log file, got: %q", content)
	}
	if !strings.Contains(content, "controller-runtime verbosity check") {
		t.Fatalf("expected controller-runtime output in log file, got: %q", content)
	}
}
