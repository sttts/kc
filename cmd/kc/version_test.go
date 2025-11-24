package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sttts/kc"
	apimachineryversion "k8s.io/apimachinery/pkg/version"
)

type fakeDiscovery struct {
	version *apimachineryversion.Info
	err     error
}

func (f fakeDiscovery) ServerVersion() (*apimachineryversion.Info, error) {
	return f.version, f.err
}

func TestCollectVersionInfoIncludesServer(t *testing.T) {
	ctx := t.Context()
	server := &apimachineryversion.Info{GitVersion: "v1.2.3"}
	expectedClient := kc.ClientGoVersion()

	info, err := collectVersionInfo(ctx, fakeDiscovery{version: server})
	if err != nil {
		t.Fatalf("collectVersionInfo returned error: %v", err)
	}
	if info.serverVersion != server {
		t.Fatalf("expected server version %v, got %v", server, info.serverVersion)
	}
	if info.clientGo != expectedClient {
		t.Fatalf("expected clientGo version %s, got %s", expectedClient, info.clientGo)
	}
}

func TestCollectVersionInfoPropagatesError(t *testing.T) {
	ctx := t.Context()
	_, err := collectVersionInfo(ctx, fakeDiscovery{err: errors.New("boom")})
	if err == nil {
		t.Fatalf("expected error from collectVersionInfo, got nil")
	}
}

func TestPrintVersionInfo(t *testing.T) {
	origVersion := version
	origCommit := commit
	origDate := date
	version = "v0.0.0-test"
	commit = "abcdef"
	date = "2024-01-01T00:00:00Z"
	t.Cleanup(func() {
		version = origVersion
		commit = origCommit
		date = origDate
	})

	expectedClientGo := kc.ClientGoVersion()
	info, err := collectVersionInfo(t.Context(), fakeDiscovery{
		version: &apimachineryversion.Info{GitVersion: "v1.2.3"},
	})
	if err != nil {
		t.Fatalf("collectVersionInfo returned error: %v", err)
	}

	var buf bytes.Buffer
	printVersionInfo(&buf, info)

	output := buf.String()
	expect := []string{
		"kc Version: v0.0.0-test",
		"Client Version (client-go): " + expectedClientGo,
		"Server Version: v1.2.3",
		"Commit: abcdef",
		"Date: 2024-01-01T00:00:00Z",
	}
	for _, want := range expect {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
