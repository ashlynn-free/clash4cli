package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"rocket4cli/internal/tui/styles"
)

// Column represents a table column
type Column struct {
	Title string
	Width int
}

// Table represents a simple table component
type Table struct {
	columns  []Column
	rows     [][]string
	selected int
	offset   int
	height   int
}

// NewTable creates a new table
func NewTable(columns []Column) *Table {
	return &Table{
		columns:  columns,
		rows:     make([][]string, 0),
		selected: 0,
		offset:   0,
		height:   10,
	}
}

// SetColumns replaces the table columns.
func (t *Table) SetColumns(columns []Column) {
	t.columns = columns
}

// SetRows sets the table rows
func (t *Table) SetRows(rows [][]string) {
	t.rows = rows
	if t.selected >= len(rows) {
		t.selected = len(rows) - 1
	}
	if t.selected < 0 {
		t.selected = 0
	}
}

// SetHeight sets the visible height
func (t *Table) SetHeight(height int) {
	t.height = height
}

// MoveUp moves selection up, wrapping to the bottom when at the top.
func (t *Table) MoveUp() {
	if len(t.rows) == 0 {
		return
	}
	if t.selected > 0 {
		t.selected--
		if t.selected < t.offset {
			t.offset = t.selected
		}
	} else {
		// Wrap to bottom
		t.selected = len(t.rows) - 1
		if len(t.rows) > t.height {
			t.offset = len(t.rows) - t.height
		} else {
			t.offset = 0
		}
	}
}

// MoveDown moves selection down, wrapping to the top when at the bottom.
func (t *Table) MoveDown() {
	if len(t.rows) == 0 {
		return
	}
	if t.selected < len(t.rows)-1 {
		t.selected++
		if t.selected >= t.offset+t.height {
			t.offset = t.selected - t.height + 1
		}
	} else {
		// Wrap to top
		t.selected = 0
		t.offset = 0
	}
}

// Selected returns the selected index
func (t *Table) Selected() int {
	return t.selected
}

// SetSelected sets the selected row index and adjusts the scroll offset so the
// selected row stays visible.
func (t *Table) SetSelected(index int) {
	if len(t.rows) == 0 {
		t.selected = 0
		t.offset = 0
		return
	}

	if index < 0 {
		index = 0
	}
	if index >= len(t.rows) {
		index = len(t.rows) - 1
	}
	t.selected = index

	if t.selected < t.offset {
		t.offset = t.selected
	}
	if t.height > 0 && t.selected >= t.offset+t.height {
		t.offset = t.selected - t.height + 1
		if t.offset < 0 {
			t.offset = 0
		}
	}
}

// Offset returns the current scroll offset (top visible row index).
func (t *Table) Offset() int {
	return t.offset
}

// SetOffset sets the scroll offset directly.
func (t *Table) SetOffset(offset int) {
	if offset < 0 {
		offset = 0
	}
	t.offset = offset
}

// SelectedRow returns the selected row data
func (t *Table) SelectedRow() []string {
	if t.selected >= 0 && t.selected < len(t.rows) {
		return t.rows[t.selected]
	}
	return nil
}

// RowCount returns the number of rows
func (t *Table) RowCount() int {
	return len(t.rows)
}

// View renders the table
func (t *Table) View() string {
	var b strings.Builder

	// Render header
	var headerCells []string
	for _, col := range t.columns {
		cell := lipgloss.NewStyle().
			Width(col.Width).
			Bold(true).
			Foreground(styles.PrimaryColor).
			Render(truncate(col.Title, col.Width))
		headerCells = append(headerCells, cell)
	}
	header := lipgloss.JoinHorizontal(lipgloss.Left, headerCells...)
	b.WriteString(header)
	b.WriteString("\n")

	// Separator
	totalWidth := 0
	for _, col := range t.columns {
		totalWidth += col.Width
	}
	b.WriteString(styles.Muted.Render(strings.Repeat("─", totalWidth)))
	b.WriteString("\n")

	// Render visible rows
	visibleEnd := t.offset + t.height
	if visibleEnd > len(t.rows) {
		visibleEnd = len(t.rows)
	}

	for i := t.offset; i < visibleEnd; i++ {
		row := t.rows[i]
		var rowCells []string

		for j, col := range t.columns {
			cellValue := ""
			if j < len(row) {
				cellValue = row[j]
			}

			cell := lipgloss.NewStyle().
				Width(col.Width).
				Render(truncate(cellValue, col.Width))
			rowCells = append(rowCells, cell)
		}

		rowStr := lipgloss.JoinHorizontal(lipgloss.Left, rowCells...)

		if i == t.selected {
			rowStr = styles.TableRowSelected.Render(rowStr)
		}

		b.WriteString(rowStr)
		if i < visibleEnd-1 {
			b.WriteString("\n")
		}
	}

	// Show scroll indicator if needed
	if len(t.rows) > t.height {
		scrollText := fmt.Sprintf("[%d-%d/%d]", t.offset+1, visibleEnd, len(t.rows))
		scrollInfo := lipgloss.NewStyle().
			Foreground(styles.MutedColor).
			Width(totalWidth).
			Align(lipgloss.Right).
			Render(scrollText)
		b.WriteString("\n")
		b.WriteString(scrollInfo)
	}

	return b.String()
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return s
	}
	if width <= 3 {
		return runewidth.Truncate(s, width, "")
	}
	return runewidth.Truncate(s, width, "...")
}
