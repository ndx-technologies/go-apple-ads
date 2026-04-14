package goappleads

import (
	"encoding/csv"
	"io"
	"strings"
	"time"
)

type CSVMetadata struct {
	ReportName  string
	From, Until time.Time
	TimeZone    string
	Currency    string
}

func (m CSVMetadata) Write(w io.StringWriter) {
	if m.ReportName != "" {
		w.WriteString("Report Name:,")
		w.WriteString(m.ReportName)
		w.WriteString("\n")
	}
	if !m.From.IsZero() && !m.Until.IsZero() {
		w.WriteString("Date Range:,")
		w.WriteString(m.From.Format(time.DateOnly))
		w.WriteString(" - ")
		w.WriteString(m.Until.Format(time.DateOnly))
		w.WriteString("\n")
	}
	var params []string
	if m.TimeZone != "" {
		params = append(params, "Timezone: "+m.TimeZone)
	}
	if m.Currency != "" {
		params = append(params, "Currency: "+m.Currency)
	}
	if len(params) > 0 {
		w.WriteString("Parameters applied:,")
		w.WriteString(strings.Join(params, "; "))
		w.WriteString(";\n")
	}
}

// ReadCSVMetadata consumes leading metadata rows whose first column ends with
// ':' and returns the first subsequent non-empty row as the CSV header.
func ReadCSVMetadata(r *csv.Reader) (m *CSVMetadata, header []string, err error) {
	m = &CSVMetadata{}
	for {
		rec, readErr := r.Read()
		if readErr != nil {
			if readErr == io.EOF {
				return nil, nil, readErr
			}
			err = readErr
			return
		}
		if len(rec) == 0 {
			continue
		}
		key, hasSuffix := strings.CutSuffix(strings.TrimSpace(rec[0]), ":")
		if key == "" {
			continue
		}
		if !hasSuffix {
			header = rec
			for i := range header {
				header[i] = strings.TrimSpace(header[i])
			}
			return
		}
		if len(rec) >= 2 {
			switch key {
			case "Report Name":
				m.ReportName = strings.TrimSpace(rec[1])
			case "Date Range":
				if from, until, cutOK := strings.Cut(strings.TrimSpace(rec[1]), " - "); cutOK {
					if t, err := time.Parse(time.DateOnly, strings.TrimSpace(from)); err == nil {
						m.From = t
					}
					if t, err := time.Parse(time.DateOnly, strings.TrimSpace(until)); err == nil {
						m.Until = t
					}
				}
			case "Time Zone":
				m.TimeZone = strings.TrimSpace(rec[1])
			case "Currency":
				m.Currency = strings.TrimSpace(rec[1])
			case "Parameters applied":
				for part := range strings.SplitSeq(rec[1], "; ") {
					pk, pv, cutOK := strings.Cut(part, ": ")
					if !cutOK {
						continue
					}
					switch pk {
					case "Timezone":
						m.TimeZone = strings.TrimRight(pv, ";")
					case "Currency":
						m.Currency = strings.TrimRight(pv, ";")
					}
				}
			}
		}
	}
}
