package models

import "k8s.io/apimachinery/pkg/runtime/schema"

// ChildConstructor returns a Folder for a virtual child under an object row.
type ChildConstructor func(Deps, string, string, []string) Folder

// ResolveChild is set by the navigation package to allow models to discover
// registered child folders for a given GVR.
var ResolveChild func(schema.GroupVersionResource) (ChildConstructor, bool)
