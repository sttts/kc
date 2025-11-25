package models

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/appconfig"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestResourceGroupItemNotifyIfChanged(t *testing.T) {
	r := &ResourceGroupItem{
		RowItem:   NewRowItem("group", nil, nil, nil),
		watchable: true,
	}

	var fired int
	r.SetOnChange(func() { fired++ })

	// No known values yet, nothing should fire.
	r.notifyIfChanged(nil)
	if fired != 0 {
		t.Fatalf("expected no change, got %d", fired)
	}

	// Publish first count value – should trigger.
	r.mu.Lock()
	r.count = 5
	r.countKnown = true
	r.mu.Unlock()

	r.notifyIfChanged(nil)
	if fired != 1 {
		t.Fatalf("expected count change to trigger once, got %d", fired)
	}

	// Same count again should not trigger.
	r.notifyIfChanged(nil)
	if fired != 1 {
		t.Fatalf("expected no additional trigger, got %d", fired)
	}

	// Publish empty flag change – should trigger again.
	r.mu.Lock()
	r.empty = true
	r.emptyKnown = true
	r.mu.Unlock()

	r.notifyIfChanged(nil)
	if fired != 2 {
		t.Fatalf("expected empty change to trigger, got %d", fired)
	}
}

func TestResourceGroupItemNotifyOnUpdateCallback(t *testing.T) {
	r := &ResourceGroupItem{RowItem: NewRowItem("group", nil, nil, nil), watchable: true}

	var changeCount, updateCount int
	r.SetOnChange(func() { changeCount++ })

	r.mu.Lock()
	r.count = 1
	r.countKnown = true
	r.mu.Unlock()

	r.notifyIfChanged(func() { updateCount++ })
	if changeCount != 1 || updateCount != 1 {
		t.Fatalf("expected both change and update callbacks, got change=%d update=%d", changeCount, updateCount)
	}

	// No further updates when values unchanged.
	r.notifyIfChanged(func() { updateCount++ })
	if changeCount != 1 || updateCount != 1 {
		t.Fatalf("expected no additional callbacks, got change=%d update=%d", changeCount, updateCount)
	}
}

func TestResourceGroupItemPeeksStopAfterForbidden(t *testing.T) {
	ctx := t.Context()
	cfg := appconfig.Default()
	cfg.Resources.PeekInterval.Duration = time.Second

	env := &envtest.Environment{}
	testCfg, err := env.Start()
	if err != nil || testCfg == nil {
		if err != nil && strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("envtest unavailable: %v", err)
		}
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })

	cl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}

	deps := Deps{
		Cl:        cl,
		AppConfig: cfg,
		Ctx:       ctx,
	}
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	item := NewResourceGroupItem(deps, gvr, schema.GroupVersionKind{}, "default", "id", nil, nil, "", nil, true, nil)

	var calls int
	denied := apierrors.NewForbidden(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, "", errors.New("denied"))
	item.hasAny = func(context.Context, schema.GroupVersionResource, string) (bool, error) {
		calls++
		return false, denied
	}

	if empty := item.Empty(); empty {
		t.Fatalf("expected resource not to be marked empty on forbidden")
	}
	if calls != 1 {
		t.Fatalf("expected a single peek attempt, got %d", calls)
	}
	if !item.peekBlocked {
		t.Fatalf("expected peeks to be blocked after forbidden")
	}

	item.hasAny = func(context.Context, schema.GroupVersionResource, string) (bool, error) {
		calls++
		return true, nil
	}
	if empty := item.Empty(); empty {
		t.Fatalf("expected empty status to remain false after blocking")
	}
	if calls != 1 {
		t.Fatalf("expected forbidden to suppress subsequent peeks, got %d calls", calls)
	}
}
