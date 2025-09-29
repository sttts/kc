package view

import (
	"encoding/base64"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metamapper "k8s.io/apimachinery/pkg/api/meta"
	yaml "sigs.k8s.io/yaml"
)

// Context abstracts what a viewer needs from the UI/app.
type Context interface {
    RESTMapper() metamapper.RESTMapper
    GetObject(gvk schema.GroupVersionKind, namespace, name string) (map[string]interface{}, error)
}

// ViewProvider renders a title and body for F3 viewing.
type ViewProvider interface {
	BuildView(ctx Context) (title string, body string, err error)
}

// KubeObjectView shows an entire object as YAML.
type KubeObjectView struct {
	Namespace string
	Resource  string // plural
	Name      string
	GVK       schema.GroupVersionKind
}

func (v *KubeObjectView) BuildView(a Context) (string, string, error) {
	gvk := v.GVK
	if gvk.Empty() {
		// Resolve resource → preferred GVK via RESTMapper
		k, err := a.RESTMapper().KindFor(schema.GroupVersionResource{Resource: v.Resource})
		if err != nil { return "", "", err }
		gvk = k
	}
	obj, err := a.GetObject(gvk, v.Namespace, v.Name)
	if err != nil {
		return "", "", err
	}
	unstructured.RemoveNestedField(obj, "metadata", "managedFields")
	yb, _ := yaml.Marshal(obj)
	return v.Name, string(yb), nil
}

// ConfigKeyView shows only a single key value; secrets decoded when textual.
type ConfigKeyView struct {
	Namespace string
	Name      string
	Key       string
	IsSecret  bool
}

func (v *ConfigKeyView) BuildView(a Context) (string, string, error) {
	res := "configmaps"
	if v.IsSecret {
		res = "secrets"
	}
	gvk, err := a.RESTMapper().KindFor(schema.GroupVersionResource{Resource: res})
	if err != nil {
		return "", "", err
	}
	obj, err := a.GetObject(gvk, v.Namespace, v.Name)
	if err != nil {
		return "", "", err
	}
	if data, found, _ := unstructured.NestedMap(obj, "data"); found {
		if val, ok := data[v.Key]; ok {
			if s, ok := val.(string); ok {
				if v.IsSecret {
					if b, err := base64.StdEncoding.DecodeString(s); err == nil {
						if utf8.Valid(b) {
							return v.Name + ":" + v.Key, string(b), nil
						}
						return v.Name + ":" + v.Key, s, nil
					}
					return v.Name + ":" + v.Key, s, nil
				}
				return v.Name + ":" + v.Key, s, nil
			}
			yb, _ := yaml.Marshal(val)
			return v.Name + ":" + v.Key, string(yb), nil
		}
	}
	return v.Name + ":" + v.Key, "", nil
}

// PodContainerView shows the YAML spec for a single container within a pod.
type PodContainerView struct {
	Namespace string
	Pod       string
	Container string
}

func (v *PodContainerView) BuildView(a Context) (string, string, error) {
	gvk, err := a.RESTMapper().KindFor(schema.GroupVersionResource{Resource: "pods"})
	if err != nil {
		return "", "", err
	}
	obj, err := a.GetObject(gvk, v.Namespace, v.Pod)
	if err != nil {
		return "", "", err
	}
	if m := findContainer(obj, v.Container); m != nil {
		yb, _ := yaml.Marshal(m)
		return v.Pod + "/" + v.Container, string(yb), nil
	}
	return v.Pod + "/" + v.Container, "", nil
}

func findContainer(pod map[string]interface{}, name string) map[string]interface{} {
	if arr, found, _ := unstructured.NestedSlice(pod, "spec", "containers"); found {
		for _, c := range arr {
			if m, ok := c.(map[string]interface{}); ok {
				if n, _ := m["name"].(string); n == name {
					return m
				}
			}
		}
	}
	if arr, found, _ := unstructured.NestedSlice(pod, "spec", "initContainers"); found {
		for _, c := range arr {
			if m, ok := c.(map[string]interface{}); ok {
				if n, _ := m["name"].(string); n == name {
					return m
				}
			}
		}
	}
	return nil
}
