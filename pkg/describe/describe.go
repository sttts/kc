package describe

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kubedescribe "k8s.io/kubectl/pkg/describe"
)

// Target identifies the object to describe.
type Target struct {
	GVR       schema.GroupVersionResource
	Namespace string
	Name      string
}

// Result contains describe output with optional metadata.
type Result struct {
	Title string
	Body  string
}

// Renderer executes kubectl-style describe operations.
type Renderer struct {
	getter genericclioptions.RESTClientGetter
}

// NewRenderer builds a Renderer backed by the provided Kubernetes plumbing.
func NewRenderer(cfg *rest.Config, mapper meta.RESTMapper, disco discovery.CachedDiscoveryInterface, loader clientcmd.ClientConfig) (*Renderer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rest config is required")
	}
	getter := newStaticRESTClientGetter(cfg, mapper, disco, loader)
	return &Renderer{getter: getter}, nil
}

// Describe renders kubectl describe output for the target resource.
func (r *Renderer) Describe(target Target) (Result, error) {
	if r == nil || r.getter == nil {
		return Result{}, fmt.Errorf("describe renderer unavailable")
	}
	if target.GVR.Resource == "" {
		return Result{}, fmt.Errorf("describe target missing resource")
	}
	mapper, err := r.getter.ToRESTMapper()
	if err != nil {
		return Result{}, fmt.Errorf("describe mapper: %w", err)
	}
	gvk, err := mapper.KindFor(target.GVR)
	if err != nil {
		return Result{}, fmt.Errorf("describe kind for %s: %w", target.GVR.String(), err)
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return Result{}, fmt.Errorf("describe mapping for %s: %w", gvk.String(), err)
	}
	describer, err := kubedescribe.Describer(r.getter, mapping)
	if err != nil {
		return Result{}, fmt.Errorf("describe constructor: %w", err)
	}
	out, err := describer.Describe(target.Namespace, target.Name, kubedescribe.DescriberSettings{ShowEvents: true})
	if err != nil {
		return Result{}, err
	}
	return Result{
		Title: renderTitle(gvk, target),
		Body:  out,
	}, nil
}

func renderTitle(gvk schema.GroupVersionKind, target Target) string {
	name := target.Name
	if name == "" {
		name = target.GVR.Resource
	}
	if target.Namespace != "" {
		name = fmt.Sprintf("%s/%s", target.Namespace, name)
	}
	if gvk.Kind != "" {
		return fmt.Sprintf("%s %s", gvk.Kind, name)
	}
	return name
}
