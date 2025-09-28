package table

// LinkedList is a doubly-linked list List implementation. It offers efficient
// inserts/removals by pointer and linear-time indexing.

type dnode struct {
    row  Row
    prev *dnode
    next *dnode
}

type LinkedList struct {
    head  *dnode
    tail  *dnode
    size  int
    byID  map[string]*dnode
}

// NewLinkedList builds a LinkedList from rows in order.
func NewLinkedList(rows []Row) *LinkedList {
    ll := &LinkedList{byID: make(map[string]*dnode, len(rows))}
    for _, r := range rows { ll.appendOne(r) }
    return ll
}

func (l *LinkedList) appendOne(r Row) {
    n := &dnode{row: r}
    if l.tail == nil {
        l.head, l.tail = n, n
    } else {
        n.prev = l.tail
        l.tail.next = n
        l.tail = n
    }
    id, _, _, _ := r.Columns()
    l.byID[id] = n
    l.size++
}

// Mutations (not part of List interface)

// Append adds rows to the end of the list.
func (l *LinkedList) Append(rows ...Row) { for _, r := range rows { l.appendOne(r) } }

// Prepend inserts rows at the beginning of the list.
func (l *LinkedList) Prepend(rows ...Row) {
    for i := len(rows) - 1; i >= 0; i-- { // keep order
        r := rows[i]
        n := &dnode{row: r}
        if l.head == nil {
            l.head, l.tail = n, n
        } else {
            n.next = l.head
            l.head.prev = n
            l.head = n
        }
        id, _, _, _ := r.Columns()
        l.byID[id] = n
        l.size++
    }
}

// InsertBeforeID inserts rows before the node with anchorID. Returns false if
// the anchor is not found.
func (l *LinkedList) InsertBeforeID(anchorID string, rows ...Row) bool {
    at, ok := l.byID[anchorID]
    if !ok { return false }
    for i := len(rows) - 1; i >= 0; i-- { // insert preserving order
        r := rows[i]
        n := &dnode{row: r}
        n.prev = at.prev
        n.next = at
        if at.prev != nil { at.prev.next = n } else { l.head = n }
        at.prev = n
        id, _, _, _ := r.Columns()
        l.byID[id] = n
        l.size++
    }
    return true
}

// InsertAfterID inserts rows after the node with anchorID. Returns false if
// the anchor is not found.
func (l *LinkedList) InsertAfterID(anchorID string, rows ...Row) bool {
    at, ok := l.byID[anchorID]
    if !ok { return false }
    for _, r := range rows {
        n := &dnode{row: r}
        n.next = at.next
        n.prev = at
        if at.next != nil { at.next.prev = n } else { l.tail = n }
        at.next = n
        at = n
        id, _, _, _ := r.Columns()
        l.byID[id] = n
        l.size++
    }
    return true
}

// RemoveIDs removes all rows with the provided IDs. Returns count removed.
func (l *LinkedList) RemoveIDs(ids ...string) int {
    removed := 0
    for _, id := range ids {
        n, ok := l.byID[id]
        if !ok { continue }
        if n.prev != nil { n.prev.next = n.next } else { l.head = n.next }
        if n.next != nil { n.next.prev = n.prev } else { l.tail = n.prev }
        delete(l.byID, id)
        removed++
        l.size--
    }
    return removed
}

// Helpers
// nodeAt returns the node at index, or nil if out of range.
func (l *LinkedList) nodeAt(idx int) *dnode {
    if idx < 0 || idx >= l.size { return nil }
    cur := l.head
    for i := 0; i < idx && cur != nil; i++ { cur = cur.next }
    return cur
}

// List interface implementation

// Len returns the number of rows in the list.
func (l *LinkedList) Len() int { return l.size }

// Lines returns a slice of rows starting at top with length up to num.
func (l *LinkedList) Lines(top, num int) []Row {
    if num <= 0 || top >= l.size { return nil }
    if top < 0 { top = 0 }
    cur := l.nodeAt(top)
    out := make([]Row, 0, num)
    for cur != nil && len(out) < num { out = append(out, cur.row); cur = cur.next }
    return out
}

// Above returns up to num rows strictly above the row with rowID.
func (l *LinkedList) Above(rowID string, num int) []Row {
    n, ok := l.byID[rowID]
    if !ok || num <= 0 { return nil }
    buf := make([]Row, 0, num)
    cur := n.prev
    for cur != nil && len(buf) < num { buf = append(buf, cur.row); cur = cur.prev }
    // reverse to keep ascending order like slice impl
    out := make([]Row, len(buf))
    for i := range buf { out[i] = buf[len(buf)-1-i] }
    return out
}

// Below returns up to num rows strictly below the row with rowID.
func (l *LinkedList) Below(rowID string, num int) []Row {
    n, ok := l.byID[rowID]
    if !ok || num <= 0 { return nil }
    out := make([]Row, 0, num)
    cur := n.next
    for cur != nil && len(out) < num { out = append(out, cur.row); cur = cur.next }
    return out
}

// Find returns the index and row with the given ID, if present.
func (l *LinkedList) Find(rowID string) (int, Row, bool) {
    n, ok := l.byID[rowID]
    if !ok { return -1, nil, false }
    // derive index by walking from head
    idx := 0
    cur := l.head
    for cur != nil && cur != n { cur = cur.next; idx++ }
    if cur == nil { return -1, nil, false }
    return idx, n.row, true
}

var _ List = (*LinkedList)(nil)

// Ensure SimpleRow still satisfies Row when referenced from this file.
var _ Row = (*SimpleRow)(nil)
