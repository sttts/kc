package models

// ChildResourceItem represents a child resource type (e.g., /pods under a ReplicaSet).
type ChildResourceItem struct {
	*RowItem
	enter func() (Folder, error)
}

func NewChildResourceItem(id string, cells []string, path []string, enter func() (Folder, error)) *ChildResourceItem {
	return &ChildResourceItem{
		RowItem: NewRowItem(id, cells, path, WhiteStyle()),
		enter:   enter,
	}
}

func (c *ChildResourceItem) Enter() (Folder, error) {
	if c.enter == nil {
		return nil, nil
	}
	return c.enter()
}
