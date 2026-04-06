package statuskeywords

import (
	"flag"
	"log"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ndx-technologies/fmtx"
	goappleads "github.com/ndx-technologies/go-apple-ads"
)

const DocShort string = "merged keyword status table"
const doc string = "Keyword status across campaign (enabled, bid, match type)\n\n"

type keywordPlacement struct {
	keyword     string
	matchType   goappleads.MatchType
	status      goappleads.Status
	bid         float64
	adGroupName string
	active      bool
}

func Run(args []string) {
	var (
		applePath     string
		keepCampaigns map[goappleads.CampaignID]bool
		keywords      []string
	)

	flag := flag.NewFlagSet("status keywords", flag.ExitOnError)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.Func("campaign-ids", "comma-separated list of campaign IDs to show (default: all)", func(s string) error {
		if len(s) == 0 {
			return nil
		}
		keepCampaigns = make(map[goappleads.CampaignID]bool)
		for id := range strings.SplitSeq(s, ",") {
			keepCampaigns[goappleads.CampaignID(id)] = true
		}
		return nil
	})
	flag.Func("keywords", "comma-separated list of keywords to show", func(s string) error {
		keywords = strings.Split(s, ",")
		return nil
	})
	flag.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	var campaigns []goappleads.CampaignConfig
	for _, campaign := range config.Campaigns {
		if keepCampaigns != nil && !keepCampaigns[campaign.ID] {
			continue
		}
		campaigns = append(campaigns, campaign)
	}
	sort.Slice(campaigns, func(i, j int) bool { return campaigns[i].Name < campaigns[j].Name })

	rows := make([][]string, 0, len(keywords))
	for _, keyword := range keywords {
		byCampaign := make(map[goappleads.CampaignID][]keywordPlacement, len(campaigns))

		for _, kw := range keywordsDB.GetKeywordsByText(keyword) {
			if kw.IsNegative {
				continue
			}

			campaign := config.GetCampaign(kw.CampaignID)
			if keepCampaigns != nil && !keepCampaigns[campaign.ID] {
				continue
			}

			placement := keywordPlacement{
				keyword:     keyword,
				matchType:   kw.MatchType,
				status:      kw.Status,
				bid:         kw.Bid,
				adGroupName: config.GetAdGroup(kw.AdGroupID).Name,
				active:      isKeywordActive(kw, *config),
			}
			byCampaign[campaign.ID] = append(byCampaign[campaign.ID], placement)
		}

		row := make([]string, 0, len(campaigns)+1)
		row = append(row, keyword)
		for _, campaign := range campaigns {
			placements := byCampaign[campaign.ID]
			slices.SortFunc(placements, comparePlacements)
			row = append(row, formatPlacements(placements))
		}
		rows = append(rows, row)
	}

	w := os.Stdout

	fmtx.HeaderTo(w, "KEYWORD STATUS")

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Keyword", Width: 28},
		},
	}

	for _, campaign := range campaigns {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: campaign.Name, Width: 50})
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, row := range rows {
		tw.WriteRow(row...)
	}
}

func isKeywordActive(kw goappleads.KeywordInfo, config goappleads.Config) bool {
	if kw.Status != goappleads.Active && kw.Status != goappleads.Enabled {
		return false
	}
	return !config.IsAdGroupPaused(kw.AdGroupID)
}

func comparePlacements(a, b keywordPlacement) int {
	if a.active != b.active {
		if a.active {
			return -1
		}
		return 1
	}
	if a.adGroupName != b.adGroupName {
		return strings.Compare(a.adGroupName, b.adGroupName)
	}
	if a.matchType != b.matchType {
		return strings.Compare(a.matchType.String(), b.matchType.String())
	}
	if a.bid != b.bid {
		if a.bid < b.bid {
			return -1
		}
		return 1
	}
	return strings.Compare(a.status.String(), b.status.String())
}

func formatPlacements(items []keywordPlacement) string {
	if len(items) == 0 {
		return fmtx.DimS("-")
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		placement := compactMatchType(item.matchType) + " " + strconv.FormatFloat(item.bid, 'f', 2, 64) + " " + item.adGroupName
		if !item.active {
			placement = fmtx.DimS(placement)
		}
		parts = append(parts, placement)
	}
	return strings.Join(parts, " | ")
}

func compactMatchType(matchType goappleads.MatchType) string {
	switch matchType {
	case goappleads.Broad:
		return "B"
	case goappleads.Exact:
		return "E"
	case goappleads.Auto:
		return "A"
	default:
		return matchType.String()
	}
}
