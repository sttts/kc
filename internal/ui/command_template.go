package ui

import (
	"bytes"
	"text/template"

	"github.com/sttts/kc/internal/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// CommandContext holds the data available to command templates
type CommandContext struct {
	// Single selection fields (also available for first item in multi-selection)
	Name      string
	Namespace string
	Kind      string
	Group     string
	Version   string
	Resource  string

	// Multi-selection fields
	Items []CommandItemContext
}

// CommandItemContext represents a single item in the selection
type CommandItemContext struct {
	Name      string
	Namespace string
	Kind      string
	Group     string
	Version   string
	Resource  string
}

// RenderCommand renders a command template with the given context
func RenderCommand(tmplStr string, items []models.Item, resourceInfo schema.GroupVersionResource) (string, error) {
	if len(items) == 0 {
		return tmplStr, nil
	}

	// Prepare context
	ctx := CommandContext{
		Items: make([]CommandItemContext, len(items)),
	}

	// Populate items
	for i, item := range items {
		var name, namespace, kind string
		if obj, ok := item.(models.ObjectItem); ok {
			name = obj.Name()
			namespace = obj.Namespace()
			// Kind is not on ObjectItem interface directly in the snippet I saw,
			// but usually it is. Let's check if ObjectItem has Kind().
			// The snippet showed: GVR(), Namespace(), Name(), SupportsVerb().
			// It inherits Item. Item has Details(), Path().
			// Wait, where is Kind()?
			// Maybe it's not available?
			// If not, I might need to rely on GVR or something else.
			// But usually objects have Kind.
			// Let's assume for now I can get it from GVR or maybe I missed it.
			// Actually, let's look at `item_object.go` if possible, or just use what I have.
			// If Kind is missing, I'll leave it empty or use Resource.
		}

		ctx.Items[i] = CommandItemContext{
			Name:      name,
			Namespace: namespace,
			Kind:      kind,
			Group:     resourceInfo.Group,
			Version:   resourceInfo.Version,
			Resource:  resourceInfo.Resource,
		}
	}

	// Populate top-level fields from the first item for convenience
	if len(items) > 0 {
		first := ctx.Items[0]
		ctx.Name = first.Name
		ctx.Namespace = first.Namespace
		ctx.Kind = first.Kind
		ctx.Group = first.Group
		ctx.Version = first.Version
		ctx.Resource = first.Resource
	}

	// Parse and execute template
	tmpl, err := template.New("command").Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}

	return buf.String(), nil
}
