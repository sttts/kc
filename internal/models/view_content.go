package models

import (
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

func objectViewContent(deps Deps, gvr schema.GroupVersionResource, namespace, name string) ViewContentFunc {
	return func() (string, string, string, string, string, error) {
		obj, err := deps.Cl.GetByGVR(deps.Ctx, gvr, namespace, name)
		if err != nil {
			return "", "", "", "", "", err
		}
		if obj == nil {
			return "", "", "", "", "", ErrNoViewContent
		}
		unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
		data, err := yaml.Marshal(obj.Object)
		if err != nil {
			return "", "", "", "", "", err
		}
		title := name
		if title == "" {
			title = obj.GetName()
		}
		if title == "" {
			title = gvr.Resource
		}
		return title, string(data), "yaml", "application/yaml", title + ".yaml", nil
	}
}

func keyViewContent(deps Deps, gvr schema.GroupVersionResource, namespace, name, key string, secret bool) ViewContentFunc {
	return func() (string, string, string, string, string, error) {
		obj, err := deps.Cl.GetByGVR(deps.Ctx, gvr, namespace, name)
		if err != nil {
			return "", "", "", "", "", err
		}
		if obj == nil {
			return "", "", "", "", "", ErrNoViewContent
		}
		data, found, _ := unstructured.NestedMap(obj.Object, "data")
		title := fmt.Sprintf("%s:%s", name, key)
		filename := fmt.Sprintf("%s_%s", name, key)
		if !found {
			return title, "", "", "", filename, nil
		}
		val, ok := data[key]
		if !ok {
			return title, "", "", "", filename, nil
		}
		switch v := val.(type) {
		case string:
			if secret {
				decoded, err := base64.StdEncoding.DecodeString(v)
				if err != nil {
					return title, v, "", "", filename, nil
				}
				if isProbablyText(decoded) {
					return title, string(decoded), "", "", filename, nil
				}
				return title, string(decoded), "", "application/octet-stream", filename, nil
			}
			return title, v, "", "", filename, nil
		default:
			out, err := yaml.Marshal(v)
			if err != nil {
				return "", "", "", "", "", err
			}
			return title, string(out), "yaml", "application/yaml", filename + ".yaml", nil
		}
	}
}

func containerViewContent(deps Deps, namespace, pod, container string) ViewContentFunc {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	return func() (string, string, string, string, string, error) {
		obj, err := deps.Cl.GetByGVR(deps.Ctx, gvr, namespace, pod)
		if err != nil {
			return "", "", "", "", "", err
		}
		if obj == nil {
			return "", "", "", "", "", ErrNoViewContent
		}
		m := findContainer(obj.Object, container)
		if m == nil {
			return "", "", "", "", "", fmt.Errorf("container %q not found", container)
		}
		out, err := yaml.Marshal(m)
		if err != nil {
			return "", "", "", "", "", err
		}
		title := fmt.Sprintf("%s/%s", pod, container)
		return title, string(out), "yaml", "application/yaml", container + ".yaml", nil
	}
}

func containerLogsViewContent(deps Deps, namespace, pod, container string, tailLines int64) ViewContentFunc {
	return func() (string, string, string, string, string, error) {
		cfg := rest.CopyConfig(deps.Cl.GetConfig())
		clientset, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kc-log-view"))
		if err != nil {
			return "", "", "", "", "", err
		}
		opts := &corev1.PodLogOptions{Container: container}
		if tailLines > 0 {
			opts.TailLines = &tailLines
		}
		req := clientset.CoreV1().Pods(namespace).GetLogs(pod, opts)
		data, err := req.Do(deps.Ctx).Raw()
		if err != nil {
			return "", "", "", "", "", err
		}
		title := fmt.Sprintf("logs:%s/%s", pod, container)
		return title, string(data), "", "text/plain", title + ".log", nil
	}
}

func findContainer(obj map[string]interface{}, name string) map[string]interface{} {
	if arr, found, _ := unstructured.NestedSlice(obj, "spec", "containers"); found {
		for _, c := range arr {
			if m, ok := c.(map[string]interface{}); ok {
				if n, _ := m["name"].(string); n == name {
					return m
				}
			}
		}
	}
	if arr, found, _ := unstructured.NestedSlice(obj, "spec", "initContainers"); found {
		for _, c := range arr {
			if m, ok := c.(map[string]interface{}); ok {
				if n, _ := m["name"].(string); n == name {
					return m
				}
			}
		}
	}
	if arr, found, _ := unstructured.NestedSlice(obj, "spec", "ephemeralContainers"); found {
		for _, c := range arr {
			if m, ok := c.(map[string]interface{}); ok {
				if metadata, ok := m["metadata"].(map[string]interface{}); ok {
					if n, _ := metadata["name"].(string); n == name {
						return m
					}
				}
			}
		}
	}
	return nil
}

func isProbablyText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	for _, r := range string(b) {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			continue
		case r < 0x20:
			return false
		}
	}
	return true
}

func decodeConfigMap(obj map[string]interface{}) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &cm); err != nil {
		return nil, err
	}
	return &cm, nil
}

func decodeSecret(obj map[string]interface{}) (*corev1.Secret, error) {
	var sec corev1.Secret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &sec); err != nil {
		return nil, err
	}
	return &sec, nil
}
