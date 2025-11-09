package main

import (
	"testing"

	"github.com/sttts/kc/internal/ui"
)

func TestDeriveStartupIntentRoot(t *testing.T) {
	intent, ns, err := deriveStartupIntent("kc root", &cliFlags{Namespace: "ns"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Verb != ui.KubectlVerbNone {
		t.Fatalf("expected no verb, got %v", intent.Verb)
	}
	if ns != "ns" {
		t.Fatalf("expected namespace ns, got %q", ns)
	}
}

func TestDeriveStartupIntentGetVariants(t *testing.T) {
	cli := &cliFlags{
		Namespace: "ns",
		Get:       getCommand{Targets: []string{"pods"}},
	}
	_, _, err := deriveStartupIntent("kc get <target>", cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, _, err = deriveStartupIntent("get <target>", cli)
	if err != nil {
		t.Fatalf("unexpected error for shorthand: %v", err)
	}
}
