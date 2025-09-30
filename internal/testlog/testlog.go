package testlog

import (
    "io"
    "os"

    klog "k8s.io/klog/v2"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Setup configures controller-runtime and klog to use a shared logr.
// Defaults to quiet (discard) unless DEBUG is set, in which case a dev zap
// logger is used and writes to stderr.
func Setup() {
    var logger = zap.New(zap.UseDevMode(true))
    if os.Getenv("DEBUG") == "" {
        logger = zap.New(zap.WriteTo(io.Discard))
    }
    ctrl.SetLogger(logger)
    // Point klog to controller-runtime's logr so both stacks share output
    klog.SetLogger(ctrl.Log)
}
