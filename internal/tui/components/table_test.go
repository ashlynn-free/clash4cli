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
