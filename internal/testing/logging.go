package kctesting

import (
    "io"
    "os"

    klog "k8s.io/klog/v2"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// SetupLogging configures controller-runtime and klog to share a logr.
// When DEBUG is unset, logs are discarded to keep CI output clean.
func SetupLogging() {
    logger := zap.New(zap.UseDevMode(true))
    if os.Getenv("DEBUG") == "" {
        logger = zap.New(zap.WriteTo(io.Discard))
    }
    ctrl.SetLogger(logger)
    klog.SetLogger(logger)
}
