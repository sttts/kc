//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 object paths=.

package tableclient

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TableTargetAccessor lets callers describe which Kubernetes resource a row or list represents.
type TableTargetAccessor interface {
	TableTarget() schema.GroupVersionKind
	SetTableTarget(schema.GroupVersionKind)
}

// Row represents a single Kubernetes API object rendered in table form.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Row struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Columns reflects the column schema returned by the server for this row.
	Columns []metav1.TableColumnDefinition `json:"columns,omitempty"`

	metav1.TableRow `json:",inline"`

	target schema.GroupVersionKind `json:"-"`
}

// NewRow allocates a Row with metadata prepared for the table client API group.
func NewRow(target schema.GroupVersionKind) *Row {
	r := &Row{}
	r.TypeMeta = metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: RowKind}
	r.SetTableTarget(target)
	return r
}

// TableTarget returns the GroupVersionKind the row originated from.
func (r *Row) TableTarget() schema.GroupVersionKind {
	if r == nil {
		return schema.GroupVersionKind{}
	}
	return r.target
}

// SetTableTarget configures the GroupVersionKind the row represents.
func (r *Row) SetTableTarget(target schema.GroupVersionKind) {
	if r == nil {
		return
	}
	r.target = target
}

// RowList represents a list of table-rendered Kubernetes objects.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Columns reflects the table schema for the list.
	Columns []metav1.TableColumnDefinition `json:"columns,omitempty"`

	Items []Row `json:"items"`

	target schema.GroupVersionKind `json:"-"`
}

// NewRowList allocates a RowList with metadata prepared for the table client API group.
func NewRowList(target schema.GroupVersionKind) *RowList {
	list := &RowList{}
	list.TypeMeta = metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: RowListKind}
	list.SetTableTarget(target)
	return list
}

// TableTarget returns the GroupVersionKind the list represents.
func (r *RowList) TableTarget() schema.GroupVersionKind {
	if r == nil {
		return schema.GroupVersionKind{}
	}
	return r.target
}

// SetTableTarget configures the GroupVersionKind the list represents.
func (r *RowList) SetTableTarget(target schema.GroupVersionKind) {
	if r == nil {
		return
	}
	r.target = target
}

var (
	_ runtime.Object      = (*Row)(nil)
	_ runtime.Object      = (*RowList)(nil)
	_ TableTargetAccessor = (*Row)(nil)
	_ TableTargetAccessor = (*RowList)(nil)
)
