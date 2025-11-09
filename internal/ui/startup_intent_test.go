package ui

import "testing"

func TestGroupGetTargets(t *testing.T) {
	intent := &GetIntent{
		Tokens: []GetToken{
			{Value: "pods", ExplicitResource: true},
			{Value: "foo"},
			{Value: "bar"},
			{Value: "svc/api", Resource: "svc", Name: "api", ExplicitResource: true, ExplicitName: true, FromSlash: true},
		},
	}
	order, groups, err := groupGetTargets(intent)
	if err != nil {
		t.Fatalf("groupGetTargets unexpected error: %v", err)
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 resources, got %v", order)
	}
	pods := groups["pods"]
	if pods == nil || len(pods.Names) != 2 {
		t.Fatalf("expected pods group with two names, got %#v", pods)
	}
	if pods.Names[0] != "foo" || pods.Names[1] != "bar" {
		t.Fatalf("unexpected pod names %v", pods.Names)
	}
	svc := groups["svc"]
	if svc == nil || len(svc.Names) != 1 || svc.Names[0] != "api" {
		t.Fatalf("unexpected svc group %#v", svc)
	}
}

func TestGroupGetTargetsNoResource(t *testing.T) {
	if _, _, err := groupGetTargets(&GetIntent{}); err == nil {
		t.Fatalf("expected error when no resources specified")
	}
}
