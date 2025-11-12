package models

var namespaceResourceVerbs = []string{"get", "list", "watch", "create", "update", "patch", "delete"}

// NamespaceResourceVerbs returns the default verb set for core Namespace resources.
func NamespaceResourceVerbs() []string {
	return append([]string(nil), namespaceResourceVerbs...)
}
