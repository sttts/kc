package main

import (
	"fmt"
	"strings"

	"github.com/sttts/kc/internal/ui"
)

func parseGetIntent(args []string, output string) (*ui.GetIntent, error) {
	tokens, err := tokenizeGetArgs(args)
	if err != nil {
		return nil, err
	}
	format := strings.TrimSpace(strings.ToLower(output))
	if format != "" && format != "yaml" {
		return nil, fmt.Errorf("unsupported output format %q; supported formats: yaml", output)
	}
	return &ui.GetIntent{
		OutputFormat: format,
		Tokens:       tokens,
	}, nil
}

func tokenizeGetArgs(args []string) ([]ui.GetToken, error) {
	var tokens []ui.GetToken
	firstToken := true
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		parts := strings.Split(arg, ",")
		for _, part := range parts {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			token := ui.GetToken{
				Original: value,
				Value:    value,
			}
			if strings.Contains(value, "/") {
				res, name, err := splitResourceName(value)
				if err != nil {
					return nil, err
				}
				token.Resource = res
				token.Name = name
				token.ExplicitResource = true
				token.ExplicitName = true
				token.FromSlash = true
			}
			if len(parts) > 1 {
				token.FromComma = true
				token.ExplicitResource = true
			}
			if firstToken && !token.ExplicitResource {
				token.ExplicitResource = true
			}
			tokens = append(tokens, token)
			firstToken = false
		}
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("kc get requires at least one resource or object")
	}
	return tokens, nil
}

func splitResourceName(value string) (string, string, error) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid resource/name value %q", value)
	}
	res := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])
	if res == "" || name == "" {
		return "", "", fmt.Errorf("invalid resource/name value %q", value)
	}
	return res, name, nil
}
