package tablecache

import (
	"context"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	kctesting "github.com/sttts/kc/internal/testing"
)

var (
	testCfg    *rest.Config
	testScheme = runtime.NewScheme()
	testEnv    *envtest.Environment
	testCtx    context.Context
)

func TestMain(m *testing.M) {
	kctesting.SetupLogging()

	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(metav1.AddMetaToScheme(testScheme))
	utilruntime.Must(AddToScheme(testScheme))

	testEnv = &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err == nil {
		testCfg = cfg
		testCtx = context.Background()
	}

	code := m.Run()

	if testEnv != nil && testCfg != nil {
		_ = testEnv.Stop()
	}

	os.Exit(code)
}
