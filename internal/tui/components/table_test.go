package components

import "testing"

func TestTable_SetSelected_AdjustsOffset(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 10}})
	rows := make([][]string, 100)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)
	tbl.SetHeight(10)

	tbl.SetSelected(15)
	if tbl.selected != 15 {
		t.Fatalf("expected selected=15, got %d", tbl.selected)
	}
	// height=10 => visible indices [offset, offset+9], so offset should shift to keep 15 visible.
	if tbl.offset != 6 {
		t.Fatalf("expected offset=6, got %d", tbl.offset)
	}

	tbl.SetSelected(0)
	if tbl.selected != 0 {
		t.Fatalf("expected selected=0, got %d", tbl.selected)
	}
	if tbl.offset != 0 {
		t.Fatalf("expected offset=0, got %d", tbl.offset)
	}
}

func TestTable_MoveUp_WrapsToBottom(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 10}})
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)
	tbl.SetHeight(5)

	// Start at top, move up should wrap to bottom
	tbl.SetSelected(0)
	tbl.MoveUp()
	if tbl.selected != 19 {
		t.Fatalf("expected selected=19 (wrap to bottom), got %d", tbl.selected)
	}
	// Offset should show the last page
	if tbl.offset != 15 {
		t.Fatalf("expected offset=15, got %d", tbl.offset)
	}
}

func TestTable_MoveDown_WrapsToTop(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 10}})
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)
	tbl.SetHeight(5)

	// Move to bottom, then move down should wrap to top
	tbl.SetSelected(19)
	tbl.MoveDown()
	if tbl.selected != 0 {
		t.Fatalf("expected selected=0 (wrap to top), got %d", tbl.selected)
	}
	if tbl.offset != 0 {
		t.Fatalf("expected offset=0, got %d", tbl.offset)
	}
}

func TestTable_MoveUp_NoWrapWhenNotAtTop(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 10}})
	rows := make([][]string, 10)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)
	tbl.SetHeight(5)

	tbl.SetSelected(5)
	tbl.MoveUp()
	if tbl.selected != 4 {
		t.Fatalf("expected selected=4, got %d", tbl.selected)
	}
}

func TestTable_WrapOnEmptyRows(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 10}})
	tbl.SetRows(nil)
	tbl.SetHeight(5)

	// Should not panic on empty rows
	tbl.MoveUp()
	tbl.MoveDown()
	if tbl.selected != 0 {
		t.Fatalf("expected selected=0 on empty table, got %d", tbl.selected)
	}
}
