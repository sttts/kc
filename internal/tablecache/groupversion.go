package tablecache

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// GroupName is the API group used for table client objects.
	GroupName = "table.kc.dev"
	// GroupVersion is the API version for table client objects.
	GroupVersion = "v1alpha1"

	// RowKind identifies the Row kind.
	RowKind = "Row"
	// RowListKind identifies the RowList kind.
	RowListKind = "RowList"
)

// SchemeGroupVersion is the group and version used to register Row and RowList types.
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: GroupVersion}

// RowGroupVersionKind identifies the GroupVersionKind for Row.
var RowGroupVersionKind = SchemeGroupVersion.WithKind(RowKind)

// RowListGroupVersionKind identifies the GroupVersionKind for RowList.
var RowListGroupVersionKind = SchemeGroupVersion.WithKind(RowListKind)

// SchemeBuilder registers Row and RowList with a scheme.
var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

// AddToScheme adds Row and RowList to a scheme.
var AddToScheme = SchemeBuilder.AddToScheme

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&Row{},
		&RowList{},
	)

	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
