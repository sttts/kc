# WIP: Kubernetes Commander (kc)

A TUI (Terminal User Interface) for Kubernetes inspired by Midnight Commander, built with Go and BubbleTea.

## Features

### ✅ Completed
- **Modular Resource Handler System**: Generic base operations (Delete, Edit, Describe, View) with resource-specific extensions
- **Kubeconfig Management**: Automatic discovery of kubeconfigs, contexts, and clusters
- **Controller-Runtime Integration**: Uses `client.Object` directly without unnecessary wrapping
- **BubbleTea TUI Framework**: Two-panel layout with terminal integration
- **Comprehensive Testing**: All components are thoroughly tested

### 🚧 In Progress
- Resource informers for live updates
- Hierarchical navigation (contexts → namespaces → resources)
- F2 resource selection with presets
- F3/F4 view/edit functionality
- F7/F8 create/delete operations
- F9 context menus
- Terminal integration with kubectl
- Configuration system

## Architecture

### Core Components

1. **Handler System** (`pkg/handlers/`)
   - `BaseHandler`: Generic operations for all resources
   - `PodHandler`: Pod-specific functionality (logs, exec, status)
   - `Registry`: Maps GVKs to handlers
   - Extensible for any Kubernetes resource type

2. **Kubeconfig Management** (`pkg/kubeconfig/`)
   - Discovers all kubeconfigs in `~/.kube`
   - Manages contexts and clusters
   - Creates controller-runtime clients
   - Supports multiple kubeconfig files

3. **TUI Framework** (`internal/ui/`)
   - `App`: Main application with two-panel layout
   - `Panel`: File/resource browser with navigation
   - `Terminal`: Integrated terminal with kubectl support
   - Function key bindings (F1-F10)

### Key Design Principles

- **Use Existing Kubernetes Concepts**: No unnecessary wrapping of `client.Object`, `schema.GroupVersionKind`, etc.
- **Generic Base Operations**: All resources get standard operations automatically
- **Resource-Specific Extensions**: Only add functionality that's unique to specific resource types
- **Modular and Extensible**: Easy to add new resource types and handlers

## Usage

### Building
```bash
go build -o kc ./cmd/kc
```

### Running
```bash
./kc
# or without building first
go run ./cmd/kc
```


### Configuration
- Path: `~/.kc/config.yaml`
- Keys use lower-case only.

Example:
```yaml
viewer:
  theme: dracula
```

Themes (lower-case)
- turbo-pascal
- dracula
- monokai
- github-dark
- nord
- solarized-dark
- solarized-light
- gruvbox-dark
- friendly
- borland
- native

Change theme at runtime: open a YAML (F3), press F9 to open the theme dialog. Moving the cursor previews the theme live; Enter applies and saves it; Esc Esc or F10 cancels and restores the previous theme.


### Key Bindings
- `F1`: Help
- `F2`: Resource selector
- `F3`: View resource
- `F4`: Edit resource
- `F5`: Copy
- `F6`: Rename/Move
- `F7`: Create namespace
- `F8`: Delete resource
- `F9`: Context menu
- `F10`: Quit
- `Ctrl+O`: Toggle terminal
- `Tab`: Switch panels
- `Ctrl+C`: Quit

## Examples

### Handler Usage
```bash
go run examples/handler/main.go
```

### Kubeconfig Discovery
```bash
go run examples/kubeconfig/main.go
```

## Testing

Run all tests:
```bash
go test ./... -v
```

With coverage summary:
```bash
go test ./... -cover
```

Run specific component tests:
```bash
go test ./pkg/handlers/... -v
go test ./pkg/kubeconfig/... -v
go test ./internal/ui/... -v
```

## Project Structure

```
kc/
├── cmd/kc/                 # Main application entry point
├── internal/ui/            # TUI components (App, Panel, Terminal)
├── pkg/handlers/           # Resource handlers and registry
├── pkg/kubeconfig/         # Kubeconfig management
├── examples/               # Usage examples
│   ├── handler/           # Handler system examples
│   └── kubeconfig/        # Kubeconfig examples
└── README.md              # This file
```

## Next Steps

1. **Resource Informers**: Implement live updates using Kubernetes informers
2. **Navigation Hierarchy**: Build the context → namespace → resource navigation
3. **Resource Selection**: Create F2 resource selector with presets
4. **View/Edit Commands**: Implement F3/F4 functionality
5. **Create/Delete Operations**: Add F7/F8 operations
6. **Context Menus**: Build F9 popup menus
7. **Terminal Integration**: Complete kubectl integration
8. **Configuration System**: Add `~/.kc/config.yaml` configuration
9. **Custom Resources**: Support for CRDs
10. **Documentation**: Complete user documentation

## Contributing

This project follows Go best practices:
- Non-trivial logic MUST be covered by unit tests
- Write tests first (TDD approach)
- Use existing Kubernetes concepts directly
- Keep components modular and extensible
- Comprehensive testing for all functionality

See AGENTS.md for detailed contributor guidelines.

## License

[License information to be added]
