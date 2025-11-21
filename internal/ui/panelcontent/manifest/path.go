package manifest

import "k8s.io/apimachinery/pkg/runtime/schema"

// apiPath renders the Kubernetes REST API path for a resource.
// Core group uses /api, otherwise /apis; namespace is optional.
func apiPath(gvr schema.GroupVersionResource, namespace, name string) string {
	base := "/apis/"
	if gvr.Group == "" {
		base = "/api/"
	}
	groupPrefix := gvr.Group
	if groupPrefix != "" {
		groupPrefix += "/"
	}
	path := base + groupPrefix + gvr.Version + "/"
	if namespace != "" {
		path += "namespaces/" + namespace + "/"
	}
	path += gvr.Resource
	if name != "" {
		path += "/" + name
	}
	return path
}
