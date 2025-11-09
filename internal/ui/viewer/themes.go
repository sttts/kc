package viewer

import (
	"sort"

	"github.com/alecthomas/chroma/v2/styles"
)

// AvailableThemes returns a curated, sorted list of supported viewer themes.
func AvailableThemes() []string {
	EnsureStyles()
	curated := []string{
		"turbo-pascal",
		"dracula", "monokai", "github-dark", "nord", "solarized-dark",
		"solarized-light", "gruvbox-dark", "friendly", "borland", "native",
	}
	avail := styles.Names()
	set := make(map[string]struct{}, len(avail))
	for _, name := range avail {
		set[name] = struct{}{}
	}
	var names []string
	for _, n := range curated {
		if _, ok := set[n]; ok {
			names = append(names, n)
		}
	}
	if len(names) < 5 {
		names = append([]string(nil), avail...)
	}
	sort.Strings(names)
	return names
}
