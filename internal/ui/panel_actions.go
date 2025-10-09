package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	models "github.com/sttts/kc/internal/models"
)

// PanelAction identifies high-level actions exposed by a panel.
type PanelAction int

const (
	PanelActionHelp PanelAction = iota
	PanelActionOptions
	PanelActionView
	PanelActionEdit
	PanelActionCreateNamespace
	PanelActionDelete
	PanelActionMenu
)

// PanelActionHandler executes an action for a panel and may return a command.
type PanelActionHandler func(*Panel) tea.Cmd

// PanelActionHandlers enumerates handlers for supported actions.
type PanelActionHandlers map[PanelAction]PanelActionHandler

// PanelEnvironment captures app-level allowances that affect panel capabilities.
type PanelEnvironment struct {
	AllowEditObjects      bool
	AllowDeleteObjects    bool
	AllowCreateNamespaces bool
}

// PanelEnvironmentSupplier resolves the current environment prior to computing capabilities.
type PanelEnvironmentSupplier func() PanelEnvironment

// PanelCapabilities describes which actions are currently enabled.
type PanelCapabilities struct {
	CanView          bool
	CanEdit          bool
	CanDelete        bool
	CanCreateNS      bool
	HasOptions       bool
	HasContextMenu   bool
	HasHelp          bool
	SupportsDescribe bool
}

// SetActionHandlers installs the action handler map for the panel.
func (p *Panel) SetActionHandlers(h PanelActionHandlers) {
	if h == nil {
		p.actionHandlers = nil
		return
	}
	// Copy to avoid external mutation.
	p.actionHandlers = make(PanelActionHandlers, len(h))
	for k, v := range h {
		p.actionHandlers[k] = v
	}
}

// SetEnvironmentSupplier wires a supplier for environment-level capability hints.
func (p *Panel) SetEnvironmentSupplier(s PanelEnvironmentSupplier) { p.envSupplier = s }

// InvokeAction runs the handler for the requested action when available.
func (p *Panel) InvokeAction(action PanelAction) tea.Cmd {
	if p == nil {
		return nil
	}
	if handler, ok := p.actionHandlers[action]; ok && handler != nil {
		return handler(p)
	}
	return nil
}

// Capabilities reports the current action capabilities for the panel.
func (p *Panel) Capabilities(ctx context.Context) PanelCapabilities {
	var caps PanelCapabilities
	if p == nil {
		return caps
	}
	env := p.environment()
	caps.HasHelp = p.actionHandlers[PanelActionHelp] != nil
	caps.HasOptions = p.actionHandlers[PanelActionOptions] != nil
	caps.HasContextMenu = p.actionHandlers[PanelActionMenu] != nil

	item, ok := p.SelectedNavItem(ctx)
	if ok && item != nil {
		if _, isBack := item.(models.Back); !isBack {
			if isViewableItem(item) {
				caps.CanView = true
			}
			if _, ok := item.(models.ObjectItem); ok {
				if env.AllowEditObjects {
					caps.CanEdit = true
				}
				if env.AllowDeleteObjects {
					caps.CanDelete = true
				}
			}
			// Describe/manifest widgets will use this flag when introduced.
			if _, ok := item.(models.ObjectItem); ok {
				caps.SupportsDescribe = true
			}
		}
	}
	// Namespace creation depends on both environment and location.
	if env.AllowCreateNamespaces {
		if strings.EqualFold(strings.TrimSpace(p.GetCurrentPath()), "/namespaces") {
			caps.CanCreateNS = true
		}
	}
	return caps
}

func (p *Panel) environment() PanelEnvironment {
	if p == nil || p.envSupplier == nil {
		return PanelEnvironment{}
	}
	return p.envSupplier()
}

func isViewableItem(item models.Item) bool {
	if item == nil {
		return false
	}
	if _, ok := item.(models.Viewable); ok {
		return true
	}
	type viewContentProvider interface {
		ViewContent() (string, string, string, string, string, error)
	}
	_, ok := item.(viewContentProvider)
	return ok
}

// actionAllowed evaluates whether the action is currently available.
func (p *Panel) actionAllowed(ctx context.Context, action PanelAction) bool {
	caps := p.Capabilities(ctx)
	switch action {
	case PanelActionHelp:
		return caps.HasHelp
	case PanelActionOptions:
		return caps.HasOptions
	case PanelActionView:
		return caps.CanView
	case PanelActionEdit:
		return caps.CanEdit
	case PanelActionCreateNamespace:
		return caps.CanCreateNS
	case PanelActionDelete:
		return caps.CanDelete
	case PanelActionMenu:
		return caps.HasContextMenu
	default:
		return false
	}
}

func (p *Panel) invokeActionIfAllowed(ctx context.Context, action PanelAction) tea.Cmd {
	if !p.actionAllowed(ctx, action) {
		return nil
	}
	return p.InvokeAction(action)
}
