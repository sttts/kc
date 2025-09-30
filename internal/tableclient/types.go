//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 object:paths=.

package tableclient

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// Row represents a single Kubernetes API object rendered in table form.
type Row struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Columns reflects the column schema returned by the server for this row.
	Columns []metav1.TableColumnDefinition `json:"columns,omitempty"`

	metav1.TableRow `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// RowList represents a list of table-rendered Kubernetes objects.
type RowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Columns reflects the table schema for the list.
	Columns []metav1.TableColumnDefinition `json:"columns,omitempty"`

	Items []Row `json:"items"`
}

var (
	_ runtime.Object = (*Row)(nil)
	_ runtime.Object = (*RowList)(nil)
)
