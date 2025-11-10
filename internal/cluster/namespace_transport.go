package cluster

import (
	"net/http"
	"net/url"
	"strings"
)

type namespaceRoundTripper struct {
	namespace string
	base      http.RoundTripper
}

func newNamespaceRoundTripper(namespace string, base http.RoundTripper) http.RoundTripper {
	if namespace == "" || base == nil {
		return base
	}
	return &namespaceRoundTripper{namespace: namespace, base: base}
}

func (rt *namespaceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || rt.namespace == "" {
		return rt.base.RoundTrip(req)
	}
	if req.Method != http.MethodGet {
		return rt.base.RoundTrip(req)
	}
	q := req.URL.Query()
	if _, ok := q["watch"]; !ok {
		if _, ok := q["resourceVersion"]; !ok {
			return rt.base.RoundTrip(req)
		}
	}
	if strings.Contains(req.URL.Path, "/namespaces/") {
		return rt.base.RoundTrip(req)
	}
	newPath, changed := injectNamespace(req.URL.Path, rt.namespace)
	if !changed {
		return rt.base.RoundTrip(req)
	}
	cloned := req.Clone(req.Context())
	cloned.URL = cloneURL(req.URL)
	cloned.URL.Path = newPath
	return rt.base.RoundTrip(cloned)
}

func injectNamespace(path, namespace string) (string, bool) {
	if strings.HasPrefix(path, "/api/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/"), "/")
		if len(parts) < 2 {
			return path, false
		}
		version := parts[0]
		rest := strings.Join(parts[1:], "/")
		return "/api/" + version + "/namespaces/" + namespace + "/" + rest, true
	}
	if strings.HasPrefix(path, "/apis/") {
		parts := strings.Split(strings.TrimPrefix(path, "/apis/"), "/")
		if len(parts) < 3 {
			return path, false
		}
		group := parts[0]
		version := parts[1]
		rest := strings.Join(parts[2:], "/")
		return "/apis/" + group + "/" + version + "/namespaces/" + namespace + "/" + rest, true
	}
	return path, false
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	clone := *u
	if u.User != nil {
		user := *u.User
		clone.User = &user
	}
	return &clone
}
