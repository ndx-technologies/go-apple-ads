package appleadstimeline

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/fmtx"
	"github.com/ndx-technologies/geo"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/slicex"
	"github.com/ndx-technologies/timex"
)

const DocShort string = "daily performance"
const doc string = "Apple Ads Timeline of daily performance\n\n"

func Run(args []string) {
	var (
		applePath                                    string
		keywordStatsCSV                              string
		campaignStatsCSV                             string
		searchTermStatsCSV                           string
		searchTermInfoCSV                            string
		campaignIDsStr, adGroupIDsStr, keywordsIDStr string
		searchTerm                                   string
		countries                                    []geo.Country
		from, until                                  time.Time
	)

	flag := flag.NewFlagSet("build", flag.ExitOnError)

	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to apple ads db")
	flag.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.StringVar(&campaignStatsCSV, "campaign-stats-csv", "data/apple_ads_campaign_stats_by_day.csv", "path to campaign stats by day CSV")
	flag.StringVar(&searchTermStatsCSV, "search-term-stats-csv", "data/apple_ads_search_terms_by_day.csv", "path to search term stats by day CSV")
	flag.StringVar(&searchTermInfoCSV, "search-term-info-csv", "data/apple_ads_search_term_impression_share_by_day.csv", "path to search term impression share by day CSV")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of adgroup IDs to keep")
	flag.StringVar(&keywordsIDStr, "keyword-ids", "", "comma-separated list of keyword IDs to keep")
	flag.StringVar(&searchTerm, "search-term", "", "if set, show daily stats for selected search term and impression share columns")
	flag.Func("countries", "comma-separated list of countries (ISO 3166) to keep (default keep all)", slicex.Parser(&countries))
	flag.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	var (
		keepCampaignIDs map[goappleads.CampaignID]bool
		keepAdGroupIDs  map[goappleads.AdGroupID]bool
		keepKeywordIDs  map[goappleads.KeywordID]bool
		keepCountries   map[geo.Country]bool
	)

	if len(campaignIDsStr) > 0 {
		keepCampaignIDs = make(map[goappleads.CampaignID]bool)
		for id := range strings.SplitSeq(campaignIDsStr, ",") {
			keepCampaignIDs[goappleads.CampaignID(id)] = true
		}
	}
	if len(adGroupIDsStr) > 0 {
		keepAdGroupIDs = make(map[goappleads.AdGroupID]bool)
		for id := range strings.SplitSeq(adGroupIDsStr, ",") {
			keepAdGroupIDs[goappleads.AdGroupID(id)] = true
		}
	}

	if len(keywordsIDStr) > 0 {
		keepKeywordIDs = make(map[goappleads.KeywordID]bool)
		for id := range strings.SplitSeq(keywordsIDStr, ",") {
			keepKeywordIDs[goappleads.KeywordID(id)] = true
		}
	}

	if len(countries) > 0 {
		keepCountries = make(map[geo.Country]bool)
		for _, c := range countries {
			keepCountries[c] = true
		}
	}

	keepSearchTermInfoCountries := keepCountriesForSearchTermInfo(keepCountries, keepCampaignIDs, keepAdGroupIDs, keepKeywordIDs, config, keywordsDB)

	var keywordsStats []goappleads.KeywordRow
	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepKeywordIDs != nil && !keepKeywordIDs[e.KeywordID] {
			continue
		}
		if keepCampaignIDs != nil && !keepCampaignIDs[e.CampaignID] {
			continue
		}
		if keepAdGroupIDs != nil && !keepAdGroupIDs[e.AdGroupID] {
			continue
		}
		if keepCountries != nil {
			campaign := config.GetCampaign(e.CampaignID)
			hasCountry := false
			for _, c := range campaign.Countries {
				if keepCountries[c] {
					hasCountry = true
					break
				}
			}
			if !hasCountry {
				continue
			}
		}
		keywordsStats = append(keywordsStats, e)
	}

	var campaigns []goappleads.CampaignRow
	for e := range iterx.FromFile(campaignStatsCSV, goappleads.ParseCampaignsStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepCampaignIDs != nil && !keepCampaignIDs[e.CampaignID] {
			continue
		}
		if keepCountries != nil {
			campaign := config.GetCampaign(e.CampaignID)
			hasCountry := false
			for _, c := range campaign.Countries {
				if keepCountries[c] {
					hasCountry = true
					break
				}
			}
			if !hasCountry {
				continue
			}
		}
		campaigns = append(campaigns, e)
	}

	var searchTerms []goappleads.SearchTermRow
	if searchTerm != "" {
		for e := range iterx.FromFile(searchTermStatsCSV, goappleads.ParseSearchTermsStatsCSV) {
			if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
				continue
			}
			if e.SearchTerm != searchTerm {
				continue
			}
			if keepKeywordIDs != nil && !keepKeywordIDs[e.KeywordID] {
				continue
			}
			if keepCampaignIDs != nil && !keepCampaignIDs[e.CampaignID] {
				continue
			}
			if keepAdGroupIDs != nil && !keepAdGroupIDs[e.AdGroupID] {
				continue
			}
			if keepCountries != nil {
				campaign := config.GetCampaign(e.CampaignID)
				hasCountry := false
				for _, c := range campaign.Countries {
					if keepCountries[c] {
						hasCountry = true
						break
					}
				}
				if !hasCountry {
					continue
				}
			}
			searchTerms = append(searchTerms, e)
		}
	}

	searchTermInfoByDay := make(map[time.Time]searchTermTimelineInfo)
	if searchTerm != "" {
		for e := range iterx.FromFile(searchTermInfoCSV, goappleads.ParseSearchTermInfoFromCSV) {
			if e.SearchTerm != searchTerm {
				continue
			}
			day, err := time.Parse(time.DateOnly, e.Day)
			if err != nil {
				log.Fatal("failed to parse search term impression share day:", err)
			}
			if (!from.IsZero() && day.Before(from)) || (!until.IsZero() && day.After(until)) {
				continue
			}
			if keepSearchTermInfoCountries != nil && !keepSearchTermInfoCountries[e.Country] {
				continue
			}
			a := searchTermInfoByDay[day]
			a.Add(e)
			searchTermInfoByDay[day] = a
		}
	}

	w := os.Stdout

	_, overall := goappleads.ComputeBaselines(keywordsStats)

	byDay := make(map[time.Time]goappleads.Agg)

	if searchTerm != "" {
		for _, r := range searchTerms {
			a := byDay[r.Day]
			if a.Days == nil {
				a.Days = make(map[time.Time]struct{})
			}
			a.Spend += r.Spend
			a.Imp += r.Impressions
			a.Taps += r.Taps
			a.Inst += r.Installs
			a.Days[r.Day] = struct{}{}
			byDay[r.Day] = a
		}
	} else if keepAdGroupIDs != nil || keepKeywordIDs != nil {
		for _, r := range keywordsStats {
			a := byDay[r.Day]
			if a.Days == nil {
				a.Days = make(map[time.Time]struct{})
			}
			a.Spend += r.Spend
			a.Imp += r.Impressions
			a.Taps += r.Taps
			a.Inst += r.Installs
			a.Days[r.Day] = struct{}{}
			byDay[r.Day] = a
		}
	} else {
		for _, r := range campaigns {
			a := byDay[r.Day]
			if a.Days == nil {
				a.Days = make(map[time.Time]struct{})
			}
			a.Spend += r.Spend
			a.Imp += r.Impressions
			a.Taps += r.Taps
			a.Inst += r.Installs
			a.Days[r.Day] = struct{}{}
			byDay[r.Day] = a
		}
	}

	days := make([]time.Time, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}

	sort.Slice(days, func(i, j int) bool { return days[i].After(days[j]) })

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Date(UTC)", Width: 10},
			{Header: "Spend(USD)", Width: 10, Alignment: fmtx.AlignRight},
			{Header: "Inst", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Installs", Width: 10, Alignment: fmtx.AlignRight},
			{Header: "Taps", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Imp", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "CPI", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CVR", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CTR", Width: 7, Alignment: fmtx.AlignRight},
		},
	}
	if searchTerm != "" {
		tw.Cols = append(tw.Cols,
			fmtx.TablCol{Header: "Pop", Width: 4, Alignment: fmtx.AlignRight},
			fmtx.TablCol{Header: "Rank", Width: 4, Alignment: fmtx.AlignRight},
			fmtx.TablCol{Header: "ImpShare", Width: 9, Alignment: fmtx.AlignRight},
			fmtx.TablCol{Header: "Pot.Imp", Width: 8, Alignment: fmtx.AlignRight},
			fmtx.TablCol{Header: "LostImp", Width: 8, Alignment: fmtx.AlignRight},
		)
	}
	tw.WriteHeader()
	tw.WriteHeaderLine()

	maxInst := 0
	for _, d := range byDay {
		if d.Inst > maxInst {
			maxInst = d.Inst
		}
	}

	for _, day := range days {
		d := byDay[day]

		var ctr, cvr, cpi float64
		if d.Imp > 0 {
			ctr = float64(d.Taps) / float64(d.Imp)
		}
		if d.Taps > 0 {
			cvr = float64(d.Inst) / float64(d.Taps)
		}
		if d.Inst > 0 {
			cpi = d.Spend / float64(d.Inst)
		}

		instS := strconv.Itoa(d.Inst)
		instColor := instS
		if d.Inst >= int(float64(maxInst)*0.8) {
			instColor = fmtx.GreenS(instS)
		} else if d.Inst < int(float64(maxInst)*0.5) {
			instColor = fmtx.RedS(instS)
		}

		cpiS := "∞"
		cpiColor := fmtx.DimS(cpiS)
		if cpi > 0 {
			cpiS = strconv.FormatFloat(cpi, 'f', 2, 64)
			if overall.CPI > 0 {
				if cpi < overall.CPI {
					cpiColor = fmtx.GreenS(cpiS)
				} else if cpi > overall.CPI*1.5 {
					cpiColor = fmtx.RedS(cpiS)
				} else {
					cpiColor = fmtx.YellowS(cpiS)
				}
			}
		}

		row := []string{
			day.Format(time.DateOnly),
			strconv.FormatFloat(d.Spend, 'f', 2, 64),
			instColor,
			fmtx.VolumeBar(d.Inst, int(float64(maxInst)*1.1), 10),
			strconv.Itoa(d.Taps),
			strconv.Itoa(d.Imp),
			cpiColor,
			strconv.FormatFloat(cvr*100, 'f', 2, 64) + "%",
			strconv.FormatFloat(ctr*100, 'f', 2, 64) + "%",
		}

		if searchTerm != "" {
			info := searchTermInfoByDay[day]
			row = append(row,
				info.PopularityFormat(),
				info.RankFormat(),
				info.ImpressionShareFormat(),
				info.PotentialImpressionsFormat(d.Imp),
				info.LostImpressionsFormat(d.Imp),
			)
		}

		tw.WriteRow(row...)
	}

	fmt.Println()
}

type searchTermTimelineInfo struct {
	Rank             int
	SearchPopularity int
	impShareFrom     float64
	impShareTo       float64
	impShareWeight   int
}

func (s *searchTermTimelineInfo) Add(v goappleads.SearchTermInfo) {
	if s.Rank == 0 || (v.Rank > 0 && v.Rank < s.Rank) {
		s.Rank = v.Rank
	}
	if v.SearchPopularity > s.SearchPopularity {
		s.SearchPopularity = v.SearchPopularity
	}
	weight := v.Impressions
	if weight <= 0 {
		weight = 1
	}
	s.impShareFrom += v.ImpressionShare.From * float64(weight)
	s.impShareTo += v.ImpressionShare.To * float64(weight)
	s.impShareWeight += weight
}

func (s searchTermTimelineInfo) avgShare() (goappleads.RatioRange, bool) {
	if s.impShareWeight <= 0 {
		return goappleads.RatioRange{}, false
	}
	return goappleads.RatioRange{
		From: s.impShareFrom / float64(s.impShareWeight),
		To:   s.impShareTo / float64(s.impShareWeight),
	}, true
}

func (s searchTermTimelineInfo) RankFormat() string {
	if s.Rank <= 0 {
		return fmtx.DimS("-")
	}

	rank := strconv.Itoa(s.Rank)
	if s.Rank == 6 {
		rank = ">5"
	}

	switch {
	case s.Rank == 1:
		return fmtx.GreenS(rank)
	case s.Rank <= 3:
		return fmtx.YellowS(rank)
	default:
		return fmtx.RedS(rank)
	}
}

func (s searchTermTimelineInfo) PopularityFormat() string {
	if s.SearchPopularity <= 0 {
		return fmtx.DimS("-")
	}

	pop := strconv.Itoa(s.SearchPopularity)
	switch {
	case s.SearchPopularity >= 4:
		return fmtx.GreenS(pop)
	case s.SearchPopularity >= 3:
		return fmtx.YellowS(pop)
	default:
		return fmtx.RedS(pop)
	}
}

func (s searchTermTimelineInfo) ImpressionShareFormat() string {
	share, ok := s.avgShare()
	if !ok {
		return fmtx.DimS("-")
	}

	value := fmt.Sprintf("%2.0f-%2.0f%%", share.From*100, share.To*100)
	mid := (share.From + share.To) / 2 * 100
	switch {
	case mid >= 50:
		return fmtx.GreenS(value)
	case mid >= 25:
		return fmtx.YellowS(value)
	default:
		return fmtx.RedS(value)
	}
}

func (s searchTermTimelineInfo) PotentialImpressionsFormat(impressions int) string {
	potential, ok := s.potentialImpressions(impressions)
	if !ok {
		return fmtx.DimS("-")
	}
	return strconv.Itoa(potential)
}

func (s searchTermTimelineInfo) LostImpressionsFormat(impressions int) string {
	lost, ok := s.lostImpressions(impressions)
	if !ok {
		return fmtx.DimS("-")
	}
	return strconv.Itoa(lost)
}

func (s searchTermTimelineInfo) potentialImpressions(impressions int) (int, bool) {
	share, ok := s.avgShare()
	if !ok {
		return 0, false
	}
	mid := (share.From + share.To) / 2
	if mid <= 0 {
		return 0, false
	}
	return int(float64(impressions) / mid), true
}

func (s searchTermTimelineInfo) lostImpressions(impressions int) (int, bool) {
	potential, ok := s.potentialImpressions(impressions)
	if !ok {
		return 0, false
	}
	if potential <= impressions {
		return 0, true
	}
	return potential - impressions, true
}

func keepCountriesForSearchTermInfo(keepCountries map[geo.Country]bool, keepCampaignIDs map[goappleads.CampaignID]bool, keepAdGroupIDs map[goappleads.AdGroupID]bool, keepKeywordIDs map[goappleads.KeywordID]bool, config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) map[geo.Country]bool {
	if keepCountries != nil {
		return keepCountries
	}

	if keepCampaignIDs == nil && keepAdGroupIDs == nil && keepKeywordIDs == nil {
		return nil
	}

	result := make(map[geo.Country]bool)
	for campaignID := range keepCampaignIDs {
		for _, country := range config.GetCampaign(campaignID).Countries {
			result[country] = true
		}
	}
	for adGroupID := range keepAdGroupIDs {
		for _, country := range config.GetCampaignForAdGroup(adGroupID).Countries {
			result[country] = true
		}
	}
	for keywordID := range keepKeywordIDs {
		keyword := keywordsDB.GetKeywordInfo(keywordID)
		for _, country := range config.GetCampaign(keyword.CampaignID).Countries {
			result[country] = true
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}
