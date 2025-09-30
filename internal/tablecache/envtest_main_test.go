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
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testCfg    *rest.Config
	testScheme = runtime.NewScheme()
	testEnv    *envtest.Environment
	testCtx    context.Context
)

func TestMain(m *testing.M) {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr))
	ctrl.SetLogger(logger)
	klog.SetLogger(logger)
	log.SetLogger(logger)

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
