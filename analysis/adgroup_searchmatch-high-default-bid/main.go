package adgroupsearchmatchhighdefaultbid

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/fmtx"
	"github.com/ndx-technologies/geo"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/go-apple-ads/analysis"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/timex"
)

const DocShort string = "detect Search Match enabled with high default bid"
const doc string = `
Search Match generates traffic even without keywords or for searches not related to keywords at all.
Search Match generates unrelated low quality traffic.
This means that with high default bid AdGroup will win many unrelated queries and burn budget.

Apple Search Ads auctions are CPT-based (max CPT bid). The DefaultMaxBid in config is a max CPT bid.
CPT is estimated from keyword stats per campaign → country (Spend / Taps).
If a country has no stats data, fallback values from --max-bids are used.
`

type CPTSource string

const (
	CPTSourceStats    CPTSource = "stats"
	CPTSourceFallback CPTSource = "fallback"
)

type Issue struct {
	CampaignID    goappleads.CampaignID
	CampaignName  string
	AdGroupID     goappleads.AdGroupID
	AdGroupName   string
	DefaultMaxBid float64
	CPT           float64
	Country       geo.Country
	Source        CPTSource
}

func Analyze(config *goappleads.Config, cptByCountry map[geo.Country]float64, sourceByCountry map[geo.Country]CPTSource) []Issue {
	var issues []Issue
	for _, camp := range config.Campaigns {
		for _, ag := range camp.AdGroups {
			if !ag.SearchMatch || ag.DefaultMaxBid <= 0 {
				continue
			}
			for _, country := range camp.Countries {
				cpt, ok := cptByCountry[country]
				if !ok {
					continue
				}
				if ag.DefaultMaxBid > cpt {
					if delta := math.Abs(ag.DefaultMaxBid - cpt); delta < 0.01 {
						continue
					}
					issues = append(issues, Issue{
						CampaignID:    camp.ID,
						CampaignName:  camp.Name,
						AdGroupID:     ag.ID,
						AdGroupName:   ag.Name,
						DefaultMaxBid: ag.DefaultMaxBid,
						CPT:           cpt,
						Country:       country,
						Source:        sourceByCountry[country],
					})
				}
			}
		}
	}
	return issues
}

func computeCPTFromStats(config *goappleads.Config, rows []goappleads.KeywordRow) (map[geo.Country]float64, map[geo.Country]CPTSource) {
	spendByCamp := make(map[goappleads.CampaignID]float64)
	tapsByCamp := make(map[goappleads.CampaignID]int)
	for _, r := range rows {
		spendByCamp[r.CampaignID] += r.Spend
		tapsByCamp[r.CampaignID] += r.Taps
	}

	cptByCountry := make(map[geo.Country]float64)
	sourceByCountry := make(map[geo.Country]CPTSource)
	for _, camp := range config.Campaigns {
		taps := tapsByCamp[camp.ID]
		if taps <= 0 {
			continue
		}
		campCPT := spendByCamp[camp.ID] / float64(taps)
		for _, country := range camp.Countries {
			if _, exists := cptByCountry[country]; !exists {
				cptByCountry[country] = campCPT
				sourceByCountry[country] = CPTSourceStats
			}
		}
	}
	return cptByCountry, sourceByCountry
}

func parseMaxCPT(s string) (map[geo.Country]float64, error) {
	result := make(map[geo.Country]float64)
	if s == "" {
		return result, nil
	}
	for pair := range strings.SplitSeq(s, ",") {
		kv := strings.SplitN(pair, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid max-bids entry %q, expected CC:value", pair)
		}
		var country geo.Country
		if err := country.UnmarshalText([]byte(strings.TrimSpace(kv[0]))); err != nil {
			return nil, fmt.Errorf("unknown country code %q: %w", kv[0], err)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid bid value for %q: %w", kv[0], err)
		}
		result[country] = v
	}
	return result, nil
}

func adGroupURL(appID string, campaignID goappleads.CampaignID, adGroupID goappleads.AdGroupID) string {
	return "https://app.searchads.apple.com/cm/app/" + appID + "/campaign/" + campaignID.String() + "/adgroup/" + adGroupID.String()
}

func printAnalysis(w io.StringWriter, showID, showURL bool, appID string, issues []Issue) {
	fmtx.HeaderTo(w, "AdGroup Search Match - High Default Bid Analysis")

	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.CampaignName == b.CampaignName {
			return a.AdGroupName < b.AdGroupName
		}
		return a.CampaignName < b.CampaignName
	})

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Campaign", Width: 40},
			{Header: "Ad Group", Width: 24},
			{Header: "Country", Width: 7},
			{Header: "DefaultBid", Width: 10, Alignment: fmtx.AlignRight},
			{Header: "CPT", Width: 8, Alignment: fmtx.AlignRight},
			{Header: "Source", Width: 8},
		},
	}

	if showID {
		tw.Cols = append([]fmtx.TablCol{
			{Header: "CampaignID", Width: 12},
			{Header: "AdGroupID", Width: 12},
		}, tw.Cols...)
	}

	if showURL {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: "URL", Width: 0})
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, e := range issues {
		bidStr := fmtx.RedS(strconv.FormatFloat(e.DefaultMaxBid, 'f', 2, 64))
		cptStr := strconv.FormatFloat(e.CPT, 'f', 2, 64)
		row := []string{
			e.CampaignName,
			e.AdGroupName,
			e.Country.String(),
			bidStr,
			cptStr,
			string(e.Source),
		}
		if showID {
			row = append([]string{fmtx.DimS(string(e.CampaignID)), fmtx.DimS(string(e.AdGroupID))}, row...)
		}
		if showURL {
			row = append(row, adGroupURL(appID, e.CampaignID, e.AdGroupID))
		}
		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

func Run(args []string) (analysis.Info, error) {
	fs := flag.NewFlagSet("analyse adgroups searchmatch-high-default-bid", flag.ExitOnError)
	var (
		applePath       string
		keywordStatsCSV string
		maxBidsStr      string
		appID           string
		showID, showURL bool
		verbose         bool
		from, until     time.Time
	)
	fs.Usage = func() {
		fs.Output().Write([]byte(doc))
		fs.PrintDefaults()
	}
	fs.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	fs.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	fs.StringVar(&maxBidsStr, "max-bids", "US:1.50,CA:1.20,GB:1.40,AU:1.30,AT:1.60,DE:1.50,FR:1.30,IT:1.20,JP:1.10,KR:0.80,HK:0.15,SG:1.00,BR:0.20,MX:0.25,AR:0.30", "fallback max CPT bid in USD per country, based on industry CPT benchmarks (used only when no stats data for that country)")
	fs.StringVar(&appID, "app-id", "", "Apple App ID")
	fs.BoolVar(&showID, "id", false, "show IDs")
	fs.BoolVar(&showURL, "url", false, "show URL to adgroup settings")
	fs.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	fs.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	fs.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	fs.Func("until", "until UTC day start (e.g. 2026-01-01) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
	fs.Parse(args)

	config, _, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	fallbackCPT, err := parseMaxCPT(maxBidsStr)
	if err != nil {
		log.Fatal("failed to parse --max-bids:", err)
	}

	var keywordsStats []goappleads.KeywordRow
	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		keywordsStats = append(keywordsStats, e)
	}

	cptByCountry, sourceByCountry := computeCPTFromStats(config, keywordsStats)

	for country, cpt := range fallbackCPT {
		if _, exists := cptByCountry[country]; !exists {
			cptByCountry[country] = cpt
			sourceByCountry[country] = CPTSourceFallback
		}
	}

	issues := Analyze(config, cptByCountry, sourceByCountry)

	if len(issues) == 0 {
		var numChecked int
		for _, camp := range config.Campaigns {
			for _, ag := range camp.AdGroups {
				if ag.SearchMatch && ag.DefaultMaxBid > 0 {
					numChecked++
				}
			}
		}
		return InfoOK{NumChecked: numChecked, NumCountries: len(cptByCountry)}, nil
	}

	if verbose {
		printAnalysis(os.Stdout, showID, showURL, appID, issues)
	}

	return nil, ErrHighDefaultBid{NumIssues: len(issues)}
}

type InfoOK struct{ NumChecked, NumCountries int }

func (s InfoOK) String() string {
	return "no search match adgroups with high default bid (checked " +
		strconv.Itoa(s.NumChecked) + " adgroups across " +
		strconv.Itoa(s.NumCountries) + " countries)"
}

type ErrHighDefaultBid struct{ NumIssues int }

func (e ErrHighDefaultBid) Error() string {
	return strconv.Itoa(e.NumIssues) + " adgroups with search match have default bid exceeding CPT (run with -v for details)"
}
