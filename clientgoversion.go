package kc

import (
	"bufio"
	_ "embed"
	"strings"
)

//go:embed go.mod
var goModData []byte

var clientGoVersion = resolveClientGoVersion()

// ClientGoVersion returns the version of k8s.io/client-go as pinned in go.mod.
func ClientGoVersion() string {
	return clientGoVersion
}

func resolveClientGoVersion() string {
	if v := parseClientGoVersion(string(goModData)); v != "" {
		return normalizeClientGoVersion(v)
	}
	return "(unknown)"
}

func parseClientGoVersion(goMod string) string {
	const module = "k8s.io/client-go"
	scanner := bufio.NewScanner(strings.NewReader(goMod))
	inRequire := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "require ("):
			inRequire = true
			continue
		case inRequire && line == ")":
			inRequire = false
			continue
		case strings.HasPrefix(line, "require "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		case !inRequire:
			continue
		}

		if version, ok := parseModuleVersionLine(module, line); ok {
			return version
		}
	}
	return ""
}

func parseModuleVersionLine(module, line string) (string, bool) {
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", false
	}
	if fields[0] != module {
		return "", false
	}
	return fields[1], true
}

// client-go modules use v0.x.y while Kubernetes versions are v1.x.y; translate for display.
func normalizeClientGoVersion(modVersion string) string {
	if strings.HasPrefix(modVersion, "v0.") {
		return "v1." + strings.TrimPrefix(modVersion, "v0.")
	}
	return modVersion
}
