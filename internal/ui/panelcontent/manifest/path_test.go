package manifest

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestAPIPath(t *testing.T) {
	tests := []struct {
		name      string
		gvr       schema.GroupVersionResource
		namespace string
		obj       string
		want      string
	}{
		{
			name:      "core namespaced",
			gvr:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			namespace: "default",
			obj:       "nginx",
			want:      "/api/v1/namespaces/default/pods/nginx",
		},
		{
			name: "core cluster scoped",
			gvr:  schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"},
			want: "/api/v1/namespaces",
		},
		{
			name:      "grouped namespaced",
			gvr:       schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			namespace: "prod",
			obj:       "web",
			want:      "/apis/apps/v1/namespaces/prod/deployments/web",
		},
		{
			name: "grouped cluster scoped",
			gvr:  schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"},
			want: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions",
		},
	}

	for _, tt := range tests {
		if got := apiPath(tt.gvr, tt.namespace, tt.obj); got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
