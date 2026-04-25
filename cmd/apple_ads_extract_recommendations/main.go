package appleadsextractrecommendations

import (
	"archive/zip"
	"encoding/csv"
	"encoding/xml"
	"flag"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const DocShort string = "extract Apple Ads keyword recommendations .xlsx into csv"

const doc string = "Extract Apple Ads Keyword Recommendations .xlsx workbook into CSV."

type xmlRow struct {
	Cells []xmlCell `xml:"c"`
}

type xmlCell struct {
	Ref       string         `xml:"r,attr"`
	Type      string         `xml:"t,attr"`
	Value     string         `xml:"v"`
	InlineStr *xmlInlineCell `xml:"is"`
}

func (s xmlCell) Text() string {
	if s.Type == "inlineStr" && s.InlineStr != nil {
		return s.InlineStr.Text
	}
	return s.Value
}

type xmlInlineCell struct {
	Text string `xml:"t"`
}

func Run(args []string) {
	flag := flag.NewFlagSet("extract recommendations", flag.ExitOnError)
	var workbookPath, outDir string
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&workbookPath, "path", "", "path to Apple Ads Keyword Recommendations .xlsx")
	flag.StringVar(&outDir, "out-dir", "", "directory for extracted csv outputs (default same dir as xlsx)")
	flag.Parse(args)

	if outDir == "" {
		outDir = filepath.Dir(workbookPath)
	}

	r, err := zip.OpenReader(workbookPath)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	for _, f := range r.File {
		if path.Dir(f.Name) == "xl/worksheets" && strings.HasSuffix(f.Name, ".xml") {
			log.Printf("extracting %s to %s\n", f.Name, outputFileName(f.Name))
			if err := writeSheetCSV(f, filepath.Join(outDir, outputFileName(f.Name))); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func outputFileName(worksheetPath string) string {
	base := path.Base(worksheetPath)
	suffix := strings.TrimSuffix(base, path.Ext(base))
	return "apple_keyword_recommendations_" + suffix + ".csv"
}

func writeSheetCSV(f *zip.File, outPath string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	w := csv.NewWriter(out)
	defer w.Flush()

	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "row" {
			continue
		}

		var row xmlRow
		if err := dec.DecodeElement(&row, &start); err != nil {
			return err
		}

		var values []string
		for _, cell := range row.Cells {
			idx := colToIndex(cell.Ref)
			for len(values) <= idx {
				values = append(values, "")
			}
			values[idx] = cell.Text()
		}

		if err := w.Write(values); err != nil {
			return err
		}
	}
	return w.Error()
}

func colToIndex(ref string) int {
	var v int
	for _, ch := range ref {
		if ch < 'A' || ch > 'Z' {
			break
		}
		v = v*26 + int(ch-'A') + 1
	}
	return v - 1
}
