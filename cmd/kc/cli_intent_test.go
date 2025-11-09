package main

import (
	"reflect"
	"testing"

	"github.com/sttts/kc/internal/ui"
)

func TestTokenizeGetArgs_SimpleList(t *testing.T) {
	tokens, err := tokenizeGetArgs([]string{"pods", "foo", "bar"})
	if err != nil {
		t.Fatalf("tokenizeGetArgs unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if !tokens[0].ExplicitResource || tokens[0].Resource != "" || tokens[0].Value != "pods" {
		t.Fatalf("first token expected resource, got %#v", tokens[0])
	}
	if tokens[1].ExplicitResource {
		t.Fatalf("second token should not be explicit resource: %#v", tokens[1])
	}
	if tokens[2].Value != "bar" {
		t.Fatalf("unexpected token value: %#v", tokens[2])
	}
}

func TestTokenizeGetArgs_CommaSeparated(t *testing.T) {
	tokens, err := tokenizeGetArgs([]string{"pods,svc"})
	if err != nil {
		t.Fatalf("tokenizeGetArgs unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	for _, tok := range tokens {
		if !tok.FromComma || !tok.ExplicitResource {
			t.Fatalf("token not marked as comma resource: %#v", tok)
		}
	}
}

func TestTokenizeGetArgs_ResourceNamePair(t *testing.T) {
	tokens, err := tokenizeGetArgs([]string{"pods/frontend", "svc/api"})
	if err != nil {
		t.Fatalf("tokenizeGetArgs unexpected error: %v", err)
	}
	want := []ui.GetToken{
		{Value: "pods/frontend", Resource: "pods", Name: "frontend", ExplicitResource: true, ExplicitName: true, FromSlash: true, Original: "pods/frontend"},
		{Value: "svc/api", Resource: "svc", Name: "api", ExplicitResource: true, ExplicitName: true, FromSlash: true, Original: "svc/api"},
	}
	if !reflect.DeepEqual(tokens, want) {
		t.Fatalf("unexpected tokens: %#v", tokens)
	}
}

func TestParseGetIntent_UnsupportedOutput(t *testing.T) {
	if _, err := parseGetIntent([]string{"pods"}, "json"); err == nil {
		t.Fatalf("expected error for unsupported output format")
	}
}

func TestParseGetIntent_EmptyArgs(t *testing.T) {
	if _, err := parseGetIntent([]string{}, ""); err == nil {
		t.Fatalf("expected error for empty args")
	}
}
