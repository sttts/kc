package ui

import (
    "os"
    "testing"
    "context"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
    kctesting "github.com/sttts/kc/internal/testing"
)

var (
    testCfg *rest.Config
    testCtx context.Context
    testEnv *envtest.Environment
)

func TestMain(m *testing.M) {
    kctesting.SetupLogging()
    testEnv = &envtest.Environment{}
    cfg, err := testEnv.Start()
    if err == nil && cfg != nil {
        testCfg = cfg
    }
    code := m.Run()
    if testEnv != nil && testCfg != nil { _ = testEnv.Stop() }
    os.Exit(code)
}
