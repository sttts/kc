package ui

import (
	"fmt"
	"strings"
)

type getTokenTarget struct {
	Resource string
	Name     string
	HasName  bool
}

type getResourceGroup struct {
	Resource string
	Names    []string
	namesSet map[string]struct{}
}

// groupGetTargets normalizes kubectl get tokens into ordered resource groups.
func groupGetTargets(intent *GetIntent) ([]string, map[string]*getResourceGroup, error) {
	tokens := buildGetTargets(intent)
	order := make([]string, 0, len(tokens))
	groups := make(map[string]*getResourceGroup)
	seen := make(map[string]struct{})
	for _, tok := range tokens {
		if tok.Resource == "" {
			continue
		}
		if _, ok := seen[tok.Resource]; !ok {
			seen[tok.Resource] = struct{}{}
			order = append(order, tok.Resource)
		}
		group := groups[tok.Resource]
		if group == nil {
			group = &getResourceGroup{
				Resource: tok.Resource,
				namesSet: make(map[string]struct{}),
			}
			groups[tok.Resource] = group
		}
		if tok.HasName {
			name := strings.TrimSpace(tok.Name)
			if name == "" {
				name = strings.TrimSpace(tok.Name)
			}
			if name == "" {
				continue
			}
			if _, ok := group.namesSet[name]; ok {
				continue
			}
			group.namesSet[name] = struct{}{}
			group.Names = append(group.Names, name)
		}
	}
	if len(order) == 0 {
		return nil, nil, fmt.Errorf("no resources specified")
	}
	return order, groups, nil
}

func buildGetTargets(intent *GetIntent) []getTokenTarget {
	if intent == nil {
		return nil
	}
	currentResource := ""
	targets := make([]getTokenTarget, 0, len(intent.Tokens))
	for _, tok := range intent.Tokens {
		if tok.Resource != "" {
			currentResource = tok.Resource
			targets = append(targets, tokenToTarget(tok.Resource, tok.Name, tok.Name != "", tok))
			continue
		}
		if tok.ExplicitResource {
			currentResource = tok.Value
			targets = append(targets, tokenToTarget(currentResource, tok.Name, tok.Name != "", tok))
			continue
		}
		if currentResource == "" {
			continue
		}
		name := tok.Name
		hasName := tok.HasName()
		if name == "" {
			name = tok.Value
		}
		targets = append(targets, tokenToTarget(currentResource, name, hasName || name != "", tok))
	}
	return targets
}

func tokenToTarget(resource, name string, hasName bool, tok GetToken) getTokenTarget {
	name = strings.TrimSpace(name)
	if name == "" && tok.Value != "" && hasName {
		name = tok.Value
	}
	return getTokenTarget{
		Resource: resource,
		Name:     name,
		HasName:  hasName && name != "",
	}
}

func (t GetToken) HasName() bool {
	return t.ExplicitName || t.FromSlash
}
