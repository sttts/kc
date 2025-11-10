package models

import (
	"os"
	"testing"

	kctesting "github.com/sttts/kc/internal/testing"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	testEnv *envtest.Environment
	testCfg *rest.Config
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
