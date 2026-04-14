package appleadsmergecsv

import (
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

	goappleads "github.com/ndx-technologies/go-apple-ads"
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
		meta   goappleads.CSVMetadata
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

		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		r.LazyQuotes = true

		metaCurrent, headerCurrent, err := goappleads.ReadCSVMetadata(r)
		if err != nil {
			slog.Error("cannot read metadata/header", "file", e.Name(), "error", err)
			continue
		}
		if header != nil && !slices.Equal(header, headerCurrent) {
			slog.Error("header mismatch, skipping file", "file", e.Name())
			continue
		}
		header = headerCurrent
		if meta.ReportName == "" {
			meta.ReportName = metaCurrent.ReportName
		}
		if meta.TimeZone == "" {
			meta.TimeZone = metaCurrent.TimeZone
		}
		if meta.Currency == "" {
			meta.Currency = metaCurrent.Currency
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
			ts, err := time.Parse("01/02/2006", row[0])
			if err != nil {
				ts, err = time.Parse(time.DateOnly, row[0])
			}
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
				v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(v), "$"), ",", ""), 64)
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
