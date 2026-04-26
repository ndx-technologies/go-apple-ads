package goappleads

import "github.com/ndx-technologies/fmtx"

func SetColDefaults(defaults map[string]fmtx.TablCol, cols []fmtx.TablCol) {
	for i, col := range cols {
		if d, ok := defaults[col.Header]; ok {
			if col.Width == 0 {
				col.Width = d.Width
			}
			if col.Alignment == fmtx.AlignUndefined {
				col.Alignment = d.Alignment
			}
		}
		cols[i] = col
	}
}
