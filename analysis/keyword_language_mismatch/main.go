package appleadsanalysiskeywordlanguagemismatch

import (
	"flag"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/fmtx"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/timex"
)

// Minimum samples required before trusting a ratio signal.
// CTR: binomial proportion — need ~30 imp for CLT to hold at typical mobile CTR.
// CPI: Poisson count — relative error ≈ 1/√inst; ≥5 gives ~45% precision, ≥10 gives ~32%.
const minImpForCTR = 30
const minInstForCPI = 5

type MismatchEntry struct {
	CampaignName string
	Keyword      goappleads.KeywordInfo
	Scripts      []string
	Imp          int
	Taps         int
	Inst         int
	Spend        float64
	BaseCTR      float64
	BaseCPI      float64
	ConfidentCTR bool
	ConfidentCPI bool
	Rec          Recommendation
}

type Recommendation string

const (
	Pause   Recommendation = "pause"
	Monitor Recommendation = "monitor"
	Keep    Recommendation = "keep"
)

func (s Recommendation) RequiresAction() bool { return s == Pause }

type AppleAdsKeywordsLanguageMismatchAnalyzer struct {
	Config     *goappleads.Config
	KeywordsDB *goappleads.KeywordCSVDB
	kwStats    map[goappleads.KeywordID]struct {
		imp, taps, inst int
		spend           float64
	}
	campStats map[goappleads.CampaignID]struct {
		imp, taps, inst int
		spend           float64
	}
}

func NewAppleAdsKeywordsLanguageMismatchAnalyzer(config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) *AppleAdsKeywordsLanguageMismatchAnalyzer {
	return &AppleAdsKeywordsLanguageMismatchAnalyzer{
		Config:     config,
		KeywordsDB: keywordsDB,
		kwStats: make(map[goappleads.KeywordID]struct {
			imp, taps, inst int
			spend           float64
		}),
		campStats: make(map[goappleads.CampaignID]struct {
			imp, taps, inst int
			spend           float64
		}),
	}
}

func (a *AppleAdsKeywordsLanguageMismatchAnalyzer) Add(r goappleads.KeywordRow) {
	kw := a.kwStats[r.KeywordID]
	kw.imp += r.Impressions
	kw.taps += r.Taps
	kw.inst += r.Installs
	kw.spend += r.Spend
	a.kwStats[r.KeywordID] = kw

	camp := a.campStats[r.CampaignID]
	camp.imp += r.Impressions
	camp.taps += r.Taps
	camp.inst += r.Installs
	camp.spend += r.Spend
	a.campStats[r.CampaignID] = camp
}

func (a *AppleAdsKeywordsLanguageMismatchAnalyzer) Finalize() []MismatchEntry {
	var entries []MismatchEntry
	for _, ki := range a.KeywordsDB.Keywords {
		if ki.IsNegative || ki.Status != goappleads.Active {
			continue
		}
		campaign := a.Config.GetCampaign(ki.CampaignID)
		if campaign.IsZero() {
			continue
		}
		unexpected := KeywordUnexpectedScripts(ki.Keyword, CountryAllowedScripts(campaign.Countries))
		if len(unexpected) == 0 {
			continue
		}
		kw := a.kwStats[ki.ID]
		camp := a.campStats[ki.CampaignID]

		var baseCTR, baseCPI float64
		if camp.imp > 0 {
			baseCTR = float64(camp.taps) / float64(camp.imp)
		}
		if camp.inst > 0 {
			baseCPI = camp.spend / float64(camp.inst)
		}

		confidentCTR := kw.imp >= minImpForCTR
		confidentCPI := kw.inst >= minInstForCPI

		var rec Recommendation
		if kw.inst > 0 && baseCPI > 0 {
			cpi := kw.spend / float64(kw.inst)
			ratio := cpi / baseCPI
			if !confidentCPI {
				rec = Monitor
			} else {
				switch {
				case ratio < 0.5:
					rec = Keep
				case ratio < 1.0:
					rec = Monitor
				default:
					rec = Pause
				}
			}
		} else if kw.imp >= minImpForCTR {
			rec = Pause
		} else if kw.imp > 0 {
			rec = Monitor
		}

		entries = append(entries, MismatchEntry{
			CampaignName: campaign.Name,
			Keyword:      ki,
			Scripts:      unexpected,
			Imp:          kw.imp,
			Taps:         kw.taps,
			Inst:         kw.inst,
			Spend:        kw.spend,
			BaseCTR:      baseCTR,
			BaseCPI:      baseCPI,
			ConfidentCTR: confidentCTR,
			ConfidentCPI: confidentCPI,
			Rec:          rec,
		})
	}

	return entries
}

func printLanguageMismatchAnalysis(w io.StringWriter, entries []MismatchEntry, showID bool) {
	fmtx.HeaderTo(w, "Language Mismatch Keywords Analysis")

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Imp != entries[j].Imp {
			return entries[i].Imp > entries[j].Imp
		}
		return entries[i].CampaignName < entries[j].CampaignName
	})

	tw := fmtx.TableWriter{
		Indent: " ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Campaign", Width: 20},
			{Header: "Keyword", Width: 28},
			{Header: "Script", Width: 12},
			{Header: "Bid", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Imp", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Taps", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "CTR", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CTR/Base", Width: 9, Alignment: fmtx.AlignRight},
			{Header: "Inst", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "CPI", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CPI/Base", Width: 9, Alignment: fmtx.AlignRight},
			{Header: "Rec", Width: 10},
		},
	}

	if showID {
		tw.Cols = append([]fmtx.TablCol{{Header: "ID", Width: 10}}, tw.Cols...)
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, e := range entries {
		var cpiStr, ratioStr, ctrStr, ctrRatioStr string

		// CTR
		if e.Imp > 0 {
			kwCTR := float64(e.Taps) / float64(e.Imp)
			ctrStr = strconv.FormatFloat(kwCTR*100, 'f', 2, 64) + "%"
			if e.BaseCTR > 0 {
				ctrRatio := kwCTR / e.BaseCTR
				ctrRatioStr = strconv.FormatFloat(ctrRatio, 'f', 1, 64) + "x"
				if !e.ConfidentCTR {
					ctrRatioStr = fmtx.DimS(ctrRatioStr)
				} else {
					switch {
					case ctrRatio >= 1.0:
						ctrRatioStr = fmtx.GreenS(ctrRatioStr)
					case ctrRatio >= 0.5:
						ctrRatioStr = fmtx.YellowS(ctrRatioStr)
					default:
						ctrRatioStr = fmtx.RedS(ctrRatioStr)
					}
				}
			}
		} else {
			ctrStr = fmtx.DimS("—")
		}

		// CPI
		if e.Inst > 0 {
			cpi := e.Spend / float64(e.Inst)
			cpiStr = strconv.FormatFloat(cpi, 'f', 2, 64)
			if e.BaseCPI > 0 {
				ratio := cpi / e.BaseCPI
				ratioStr = strconv.FormatFloat(ratio, 'f', 1, 64) + "x"
				if !e.ConfidentCPI {
					ratioStr = fmtx.DimS(ratioStr)
				} else {
					switch {
					case ratio < 0.5:
						ratioStr = fmtx.GreenS(ratioStr)
					case ratio < 1.0:
						ratioStr = fmtx.YellowS(ratioStr)
					default:
						ratioStr = fmtx.RedS(ratioStr)
					}
				}
			}
		} else {
			cpiStr = fmtx.DimS("—")
		}

		var recStr string
		switch e.Rec {
		case Pause:
			recStr = fmtx.RedS("pause")
		case Keep:
			recStr = fmtx.GreenS("keep")
		case Monitor:
			if e.ConfidentCPI {
				recStr = fmtx.YellowS("monitor")
			} else {
				recStr = fmtx.DimS("monitor")
			}
		}

		row := []string{
			e.CampaignName,
			e.Keyword.Keyword,
			strings.Join(e.Scripts, ","),
			"$" + strconv.FormatFloat(e.Keyword.Bid, 'f', 2, 64),
			strconv.Itoa(e.Imp),
			strconv.Itoa(e.Taps),
			ctrStr,
			ctrRatioStr,
			strconv.Itoa(e.Inst),
			cpiStr,
			ratioStr,
			recStr,
		}

		if showID {
			row = append([]string{string(e.Keyword.ID)}, row...)
		}

		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

const DocShort string = "detect foreign-script keywords in non-native markets"
const doc string = `
Language Mismatch — foreign-script keywords in non-native markets.

Currency: USD

`

func Run(args []string) {
	flag := flag.NewFlagSet("analyse keywords language-mismatch", flag.ExitOnError)
	var (
		applePath                     string
		keywordStatsCSV               string
		verbose                       bool
		campaignIDsStr, adGroupIDsStr string
		showID                        bool
		from, until                   time.Time
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	flag.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of ad group IDs to keep")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	var keepAdGroup map[goappleads.AdGroupID]bool
	if len(adGroupIDsStr) > 0 {
		keepAdGroup = make(map[goappleads.AdGroupID]bool)
		for id := range strings.SplitSeq(adGroupIDsStr, ",") {
			keepAdGroup[goappleads.AdGroupID(id)] = true
		}
	}

	var keepCampaign map[goappleads.CampaignID]bool
	if len(campaignIDsStr) > 0 {
		keepCampaign = make(map[goappleads.CampaignID]bool)
		for id := range strings.SplitSeq(campaignIDsStr, ",") {
			keepCampaign[goappleads.CampaignID(id)] = true
		}
	}

	analyzer := NewAppleAdsKeywordsLanguageMismatchAnalyzer(config, keywordsDB)

	for r := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && r.Day.Before(from)) || (!until.IsZero() && r.Day.After(until)) {
			continue
		}
		if keepAdGroup != nil && !keepAdGroup[r.AdGroupID] {
			continue
		}
		if keepCampaign != nil && !keepCampaign[r.CampaignID] {
			continue
		}
		analyzer.Add(r)
	}

	entries := analyzer.Finalize()

	w := os.Stdout

	if len(entries) == 0 {
		w.WriteString(fmtx.GreenS("ok") + " no language mismatch keywords found\n")
		return
	}

	var numRequiresAction int
	for _, e := range entries {
		if e.Rec.RequiresAction() {
			numRequiresAction++
		}
	}
	if numRequiresAction == 0 {
		w.WriteString(fmtx.GreenS("ok") + " keywords(" + strconv.Itoa(len(entries)) + "/" + strconv.Itoa(len(keywordsDB.Keywords)) + ") with language mismatch found, but none require action\n")
		return
	}

	if verbose {
		printLanguageMismatchAnalysis(w, entries, showID)
	} else {
		numRequiresActionStr := strconv.Itoa(numRequiresAction)
		if numRequiresAction > 0 {
			numRequiresActionStr = fmtx.RedS(numRequiresActionStr)
		}
		w.WriteString(fmtx.RedS("error") + " " + strconv.Itoa(len(entries)) + " keywords with foreign-script mismatch found, " + numRequiresActionStr + " require action (run with -v for details)\n")
	}
	os.Exit(1)
}
