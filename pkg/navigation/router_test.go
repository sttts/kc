package navigation

import "testing"

func TestSimpleRouter_ParseClusterPaths(t *testing.T) {
	r := NewSimpleRouter()

	cases := []struct {
		in   string
		ns   string
		res  string
		name string
	}{
		{"/cluster", "", "", ""},
		{"/cluster/pods", "", "pods", ""},
		{"/cluster/pods/some-pod", "", "pods", "some-pod"},
		{"/cluster/namespaces/default/pods", "default", "pods", ""},
		{"/cluster/namespaces/kube-system/pods/coredns-xyz", "kube-system", "pods", "coredns-xyz"},
	}

	for _, tc := range cases {
		p, err := r.Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%s) unexpected error: %v", tc.in, err)
		}
		if p.Namespace != tc.ns || p.Resource != tc.res || p.Name != tc.name {
			t.Fatalf("Parse(%s) got ns=%q res=%q name=%q", tc.in, p.Namespace, p.Resource, p.Name)
		}
		// Build should round-trip to a canonical cluster path
		out := r.Build(p)
		// Normalize: Build may produce canonical cluster path; parse again and compare struct
		p2, err := r.Parse(out)
		if err != nil {
			t.Fatalf("Parse(Build(%s)) error: %v", tc.in, err)
		}
		if p2.Namespace != p.Namespace || p2.Resource != p.Resource || p2.Name != p.Name {
			t.Fatalf("Round-trip mismatch for %s -> %s", tc.in, out)
		}
	}
}

func TestSimpleRouter_ParseContexts(t *testing.T) {
	r := NewSimpleRouter()
	p, err := r.Parse("/contexts/dev/cluster/namespaces/ns1/configmaps/cfg")
	if err != nil {
		t.Fatal(err)
	}
	if p.Context != "dev" || p.Namespace != "ns1" || p.Resource != "configmaps" || p.Name != "cfg" {
		t.Fatalf("unexpected parse: %+v", p)
	}
}

func TestSimpleRouter_ParseGroups(t *testing.T) {
	r := NewSimpleRouter()
	p, err := r.Parse("/groups/apps/v1/namespaces/default/deployments/nginx")
	if err != nil {
		t.Fatal(err)
	}
	if p.Group != "apps" || p.Version != "v1" || p.Namespace != "default" || p.Resource != "deployments" || p.Name != "nginx" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	// Build should prefer group mode when Group/Version present
	out := r.Build(p)
	if out != "/groups/apps/v1/namespaces/default/deployments/nginx" {
		t.Fatalf("unexpected build: %s", out)
	}
}

func TestSimpleRouter_Parent(t *testing.T) {
	r := NewSimpleRouter()
	p, err := r.Parse("/cluster/namespaces/default/pods/nginx")
	if err != nil {
		t.Fatal(err)
	}
	p1 := r.Parent(p)
	if p1.Name != "" || p1.Resource != "pods" || p1.Namespace != "default" {
		t.Fatalf("parent step1 unexpected: %+v", p1)
	}
	p2 := r.Parent(p1)
	if p2.Resource != "" || p2.Namespace != "default" {
		t.Fatalf("parent step2 unexpected: %+v", p2)
	}
	p3 := r.Parent(p2)
	if p3.Namespace != "" {
		t.Fatalf("parent step3 unexpected: %+v", p3)
	}
}
