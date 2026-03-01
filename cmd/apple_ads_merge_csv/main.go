package appleadsmergecsv

import (
	"bufio"
	"encoding/csv"
	"flag"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func rowKey(groupByColumn map[string]bool, header, row []string) string {
	var b strings.Builder
	for i, v := range row {
		if groupByColumn[header[i]] {
			b.WriteString(v)
		}
		b.WriteByte('\x00')
	}
	return b.String()
}

type metadata struct {
	ReportName      string
	TimeGranularity string
	OrgID           string
	From, Until     time.Time
	TimeZone        string
	Currency        string
	AdPlacement     string
	Comment         string
}

// ReadFrom reads metadata key-value lines from r until an empty line or CSV header.
// Returns the unconsumed CSV header line (if any) so the caller can prepend it back.
func (m *metadata) ReadFrom(r *bufio.Reader) (string, error) {
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		rawLine := line
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "\xef\xbb\xbf")
		if line == "" {
			return "", nil
		}
		k, v, ok := strings.Cut(line, ", ")
		if !ok {
			// line has no ", " — either a comment or the CSV header row.
			// If it has many commas it's the CSV header; return it unconsumed.
			if strings.Count(line, ",") >= 2 {
				return rawLine, nil
			}
			if m.Comment != "" && m.Comment != line {
				m.Comment += "\n"
			}
			m.Comment += line
			continue
		}
		switch k {
		case "Report Name":
			m.ReportName = v
		case "Date Range":
			if from, until, ok := strings.Cut(v, " - "); ok {
				if t, err := time.Parse(time.DateOnly, strings.TrimSpace(from)); err == nil {
					m.From = t
				}
				if t, err := time.Parse(time.DateOnly, strings.TrimSpace(until)); err == nil {
					m.Until = t
				}
			}
		case "Time Granularity":
			m.TimeGranularity = v
		case "Org Id":
			m.OrgID = v
		case "Time Zone":
			m.TimeZone = v
		case "Currency":
			m.Currency = v
		case "Ad Placement":
			m.AdPlacement = v
		default:
			slog.Error("header: unknown key, skipping", "k", k, "v", v)
		}
	}
}

func (m metadata) Write(w io.StringWriter) {
	if m.ReportName != "" {
		w.WriteString("Report Name, ")
		w.WriteString(m.ReportName)
		w.WriteString("\n")
	}
	if !m.From.IsZero() && !m.Until.IsZero() {
		w.WriteString("Date Range, ")
		w.WriteString(m.From.Format(time.DateOnly))
		w.WriteString(" - ")
		w.WriteString(m.Until.Format(time.DateOnly))
		w.WriteString("\n")
	}
	if m.TimeGranularity != "" {
		w.WriteString("Time Granularity, ")
		w.WriteString(m.TimeGranularity)
		w.WriteString("\n")
	}
	if m.OrgID != "" {
		w.WriteString("Org Id, ")
		w.WriteString(m.OrgID)
		w.WriteString("\n")
	}
	if m.TimeZone != "" {
		w.WriteString("Time Zone, ")
		w.WriteString(m.TimeZone)
		w.WriteString("\n")
	}
	if m.Currency != "" {
		w.WriteString("Currency, ")
		w.WriteString(m.Currency)
		w.WriteString("\n")
	}
	if m.Comment != "" {
		w.WriteString(m.Comment)
		w.WriteString("\n")
	}
}

const DocShort string = "merge Apple Ads Insights CSV files "

func Run(args []string) {
	flag := flag.NewFlagSet("merge-csv", flag.ExitOnError)
	var (
		folder            string
		groupByColumnsStr string
	)
	flag.StringVar(&folder, "path", "", "path to folder with Apple Ads CSV files")
	flag.StringVar(&groupByColumnsStr, "group-by", "", "comma-separated list of columns to group by")
	flag.Parse(args)

	groupByColumn := make(map[string]bool)
	for col := range strings.SplitSeq(groupByColumnsStr, ",") {
		if col = strings.TrimSpace(col); len(col) > 0 {
			groupByColumn[col] = true
		}
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		log.Fatal(err)
	}
	if len(entries) == 0 {
		log.Fatalf("no files found in %s", folder)
	}

	var (
		meta   metadata
		header []string
	)

	seen := make(map[string]struct{})
	sum := make(map[string][]float64)

	var numExact, numAgg int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".csv") {
			continue
		}

		f, err := os.Open(filepath.Join(folder, e.Name()))
		if err != nil {
			slog.Error("cannot open file", "file", e.Name(), "error", err)
			continue
		}
		defer f.Close()

		br := bufio.NewReader(f)

		peek, err := br.Peek(50)
		if err != nil {
			slog.Error("cannot peek file", "file", e.Name(), "error", err)
			continue
		}
		peekStr := strings.TrimPrefix(string(peek), "\xef\xbb\xbf")
		hasMetadata := strings.HasPrefix(peekStr, "Report Name")

		var csvReader io.Reader = br
		if hasMetadata {
			leftover, err := meta.ReadFrom(br)
			if err != nil {
				slog.Error("cannot read metadata", "file", e.Name(), "error", err)
				continue
			}
			if leftover != "" {
				csvReader = io.MultiReader(strings.NewReader(leftover), br)
			}
		}

		r := csv.NewReader(csvReader)
		r.FieldsPerRecord = -1
		r.LazyQuotes = true

		if headerCurrent, err := r.Read(); err != nil {
			slog.Error("cannot read header", "file", e.Name(), "error", err)
			continue
		} else {
			if header != nil && !slices.Equal(header, headerCurrent) {
				slog.Error("header mismatch, skipping file", "file", e.Name())
				continue
			}
			for i := range headerCurrent {
				headerCurrent[i] = strings.TrimSpace(headerCurrent[i])
			}
			header = headerCurrent
		}

		// trim header
		for i := range header {
			header[i] = strings.TrimSpace(header[i])
		}

		r.FieldsPerRecord = len(header)

		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				slog.Error("cannot read row", "file", e.Name(), "error", err)
				continue
			}

			// skip header if repeated
			trimmedRow := make([]string, len(row))
			for i, v := range row {
				trimmedRow[i] = strings.TrimSpace(v)
			}
			if slices.Equal(trimmedRow, header) {
				continue
			}

			// exact dedup
			var parts []string
			for _, v := range row {
				parts = append(parts, strings.TrimSpace(v))
			}
			ck := strings.Join(parts, "\x00")
			if _, ok := seen[ck]; ok {
				numExact++
				continue
			}
			seen[ck] = struct{}{}

			// date range
			ts, err := time.Parse(time.DateOnly, row[0])
			if err != nil {
				slog.Error("cannot parse date", "value", row[0], "error", err, "file", e.Name())
				continue
			}
			if meta.From.IsZero() || ts.Before(meta.From) {
				meta.From = ts
			}
			if meta.Until.IsZero() || ts.After(meta.Until) {
				meta.Until = ts
			}

			k := rowKey(groupByColumn, header, row)
			if _, ok := sum[k]; !ok {
				sum[k] = make([]float64, len(row))
			}
			for i, v := range row {
				if v == "" || v == "--" || v == "null" || groupByColumn[header[i]] {
					continue
				}
				v, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
				if err != nil {
					slog.Error("cannot parse float", "value", v, "error", err, "file", e.Name())
				}
				sum[k][i] += v
			}
			numAgg++
		}
	}

	w := os.Stdout

	meta.ReportName = filepath.Base(folder) + "_merged"
	meta.Write(w)
	w.Write([]byte("\n"))

	cw := csv.NewWriter(w)
	if len(header) > 0 {
		cw.Write(header)
	}
	for k, a := range sum {
		keys := strings.Split(strings.TrimSuffix(k, "\x00"), "\x00")
		row := make([]string, len(header))
		keyIndex := 0
		for i, col := range header {
			if groupByColumn[col] {
				row[i] = keys[keyIndex]
				keyIndex++
			} else {
				row[i] = strconv.FormatFloat(a[i], 'f', -1, 64)
			}
		}
		cw.Write(row)
	}
	cw.Flush()

	slog.Info("done", "num_files", len(entries), "num_rows", numExact+numAgg, "exact", float64(numExact)/float64(numExact+numAgg), "agg", float64(numAgg)/float64(numExact+numAgg))
}
