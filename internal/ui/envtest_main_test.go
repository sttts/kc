package ui

import (
	"context"
	kctesting "github.com/sttts/kc/internal/testing"
	"k8s.io/client-go/rest"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
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
	if testEnv != nil && testCfg != nil {
		_ = testEnv.Stop()
	}
	os.Exit(code)
}
