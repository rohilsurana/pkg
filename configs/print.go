package configs

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Print writes a formatted table of all config fields to w,
// showing the resolved value, source, and validation rules.
// Fields tagged sensitive:"true" have their values masked.
func (l *Loader) Print(w io.Writer) {
	if l.cfg == nil {
		return
	}

	rv := reflect.ValueOf(l.cfg)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	hasRules := false
	for _, f := range l.fields {
		if len(f.rules) > 0 {
			hasRules = true
			break
		}
	}

	headers := []string{"KEY", "VALUE", "SOURCE"}
	if hasRules {
		headers = append(headers, "RULES")
	}

	var rows [][]string
	for _, f := range l.fields {
		val := rv.FieldByIndex(f.index)
		value := formatValue(val, f.sensitive)
		source := l.resolveSource(f)
		row := []string{f.viperKey, value, source}
		if hasRules {
			row = append(row, describeRules(f.rules))
		}
		rows = append(rows, row)
	}

	printTable(w, headers, rows)
}

func formatValue(val reflect.Value, sensitive bool) string {
	if sensitive {
		return "********"
	}
	return fmt.Sprintf("%v", val.Interface())
}

func printTable(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	hline := func(left, mid, right string) {
		fmt.Fprint(w, left)
		for i, width := range widths {
			fmt.Fprint(w, strings.Repeat("─", width+2))
			if i < len(widths)-1 {
				fmt.Fprint(w, mid)
			}
		}
		fmt.Fprintln(w, right)
	}
	prow := func(cells []string) {
		fmt.Fprint(w, "│")
		for i, width := range widths {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			fmt.Fprintf(w, " %-*s ", width, cell)
			fmt.Fprint(w, "│")
		}
		fmt.Fprintln(w)
	}

	hline("┌", "┬", "┐")
	prow(headers)
	hline("├", "┼", "┤")
	for _, row := range rows {
		prow(row)
	}
	hline("└", "┴", "┘")
}
