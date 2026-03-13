package appleadsanalysiskeyworddiscovery

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
	"github.com/ndx-technologies/go-apple-ads/analysis"
	"github.com/ndx-technologies/timex"
)

const DocShort string = "detect broad/exact/negative keyword mis-configuration in discovery/targetted campaigns"
const doc string = `
Apple recommends to split broad/exact keyword between discovery/targetted campaigns.
In addition, discovery campaigns should contain exact negatives from targetted campaigns to improve quality of discovery and reduce cannibalisation.

Discovery campaigns are detected by "- Discovery" in campaign name and matched by prefix. Example of matched group of campaign names:
- "Search Results UK - Discovery"
- "Search Results UK Banana"
- "Search Results UK"

Each Campaign can have only one corresponding Discovery campaign.
Each Discovery campaign can have multiple corresponding Targetted campaigns.

`

type IssueType string

const (
	ExactInDiscovery           IssueType = "exact_in_discovery"
	BroadInTargeted            IssueType = "broad_in_targeted"
	MissingNegativeInDiscovery IssueType = "missing_negative_in_discovery"
)

type DiscoveryIssue struct {
	Type                IssueType
	Keyword             string
	MatchType           goappleads.MatchType
	KeywordID           goappleads.KeywordID
	CampaignID          goappleads.CampaignID
	DiscoveryCampaignID goappleads.CampaignID // set for type=MissingNegativeInDiscovery
}

func (e DiscoveryIssue) Error() string {
	switch e.Type {
	case ExactInDiscovery:
		return "exact in discovery"
	case BroadInTargeted:
		return "broad in targeted"
	case MissingNegativeInDiscovery:
		return "missing negative in discovery campaign(" + e.DiscoveryCampaignID.String() + ")"
	}
	return ""
}

type AppleAdsKeywordsDiscoveryAnalyzer struct {
	Config     *goappleads.Config
	KeywordsDB *goappleads.KeywordCSVDB
}

func NewAppleAdsKeywordsDiscoveryAnalyzer(config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) *AppleAdsKeywordsDiscoveryAnalyzer {
	return &AppleAdsKeywordsDiscoveryAnalyzer{
		Config:     config,
		KeywordsDB: keywordsDB,
	}
}

func (a *AppleAdsKeywordsDiscoveryAnalyzer) Finalize() []DiscoveryIssue {
	var discoveryNames []string
	for _, camp := range a.Config.Campaigns {
		if isDiscoveryCampaign(camp.Name) {
			discoveryNames = append(discoveryNames, camp.Name)
		}
	}

	discoveryCampaigns := make(map[goappleads.CampaignID]bool)
	negativesByDiscoveryCampaign := make(map[goappleads.CampaignID]map[string]bool)
	discoveryByTargetedName := make(map[string]goappleads.CampaignID)
	for _, camp := range a.Config.Campaigns {
		if isDiscoveryCampaign(camp.Name) {
			discoveryCampaigns[camp.ID] = true
			negativesByDiscoveryCampaign[camp.ID] = make(map[string]bool)
			discoveryByTargetedName[targetedCampaignName(camp.Name, discoveryNames)] = camp.ID
		}
	}

	for _, kw := range a.KeywordsDB.Keywords {
		if kw.IsNegative && discoveryCampaigns[kw.CampaignID] {
			negativesByDiscoveryCampaign[kw.CampaignID][kw.Keyword] = true
		}
	}

	var issues []DiscoveryIssue

	for _, kw := range a.KeywordsDB.Keywords {
		if kw.IsNegative || kw.IsZero() || kw.MatchType == goappleads.Auto || kw.Status != goappleads.Active {
			continue
		}

		campaign := a.Config.GetCampaign(kw.CampaignID)
		isDiscovery := isDiscoveryCampaign(campaign.Name)

		if isDiscovery && kw.MatchType == goappleads.Exact {
			issues = append(issues, DiscoveryIssue{
				Type:       ExactInDiscovery,
				Keyword:    kw.Keyword,
				MatchType:  kw.MatchType,
				KeywordID:  kw.ID,
				CampaignID: kw.CampaignID,
			})
		}

		if !isDiscovery && kw.MatchType == goappleads.Broad {
			issues = append(issues, DiscoveryIssue{
				Type:       BroadInTargeted,
				Keyword:    kw.Keyword,
				MatchType:  kw.MatchType,
				KeywordID:  kw.ID,
				CampaignID: kw.CampaignID,
			})
		}

		if !isDiscovery && kw.MatchType == goappleads.Exact {
			discCampID, ok := discoveryByTargetedName[targetedCampaignName(campaign.Name, discoveryNames)]
			if !ok {
				continue
			}
			negatives := negativesByDiscoveryCampaign[discCampID]
			if !negatives[kw.Keyword] {
				issues = append(issues, DiscoveryIssue{
					Type:                MissingNegativeInDiscovery,
					Keyword:             kw.Keyword,
					MatchType:           kw.MatchType,
					KeywordID:           kw.ID,
					CampaignID:          kw.CampaignID,
					DiscoveryCampaignID: discCampID,
				})
			}
		}
	}

	return issues
}

func printDiscoveryAnalysis(
	w io.StringWriter,
	config goappleads.Config,
	showID bool,
	issues []DiscoveryIssue,
) {
	fmtx.HeaderTo(w, "Keyword Discovery Configuration Analysis")

	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Keyword == b.Keyword {
			if a.Type == b.Type {
				return a.CampaignID < b.CampaignID
			}
			return a.Type < b.Type
		}
		return a.Keyword < b.Keyword
	})

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Keyword", Width: 28},
			{Header: "Campaign", Width: 32},
			{Header: "Issue", Width: 52},
		},
	}

	if showID {
		tw.Cols = append([]fmtx.TablCol{{Header: "ID", Width: 10}}, tw.Cols...)
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, e := range issues {
		keyword := e.Keyword
		if e.MatchType == goappleads.Exact {
			keyword = "[" + keyword + "]"
		}

		campaign := config.GetCampaign(e.CampaignID)

		row := []string{
			keyword,
			campaign.Name,
			e.Error(),
		}

		if showID {
			row = append([]string{fmtx.DimS(string(e.KeywordID))}, row...)
		}

		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

func Run(args []string) (analysis.Info, error) {
	flag := flag.NewFlagSet("analyse keywords discovery", flag.ExitOnError)
	var (
		applePath                     string
		showID                        bool
		verbose                       bool
		campaignIDsStr, adGroupIDsStr string
		from, until                   time.Time
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of ad group IDs to keep")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2025-12-31) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
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

	analyzer := NewAppleAdsKeywordsDiscoveryAnalyzer(config, keywordsDB)

	groups := analyzer.Finalize()

	if len(groups) == 0 {
		var numCampaigns, numDiscoveryCampaigns int
		for _, c := range config.Campaigns {
			numCampaigns++
			if isDiscoveryCampaign(c.Name) {
				numDiscoveryCampaigns++
			}
		}

		var numBroad, numExact, numBroadInDiscovery int
		for _, kw := range keywordsDB.Keywords {
			if kw.Status != goappleads.Active {
				continue
			}
			if kw.IsNegative {
				continue
			}
			camp := config.GetCampaign(kw.CampaignID)
			switch kw.MatchType {
			case goappleads.Broad:
				numBroad++
				if isDiscoveryCampaign(camp.Name) {
					numBroadInDiscovery++
				}
			case goappleads.Exact:
				numExact++
			}
		}
		numKeywords := numBroad + numExact

		return analysis.Join(
			InfoBroadKeywordInDiscoveryCampaign{NumBroad: numBroad, NumKeywords: numKeywords, NumDiscoveryCampaigns: numDiscoveryCampaigns, NumCampaigns: numCampaigns},
			InfoExactKeywordNegativeInDiscoveryCampaign{NumExact: numExact, NumKeywords: numKeywords, NumDiscoveryCampaigns: numDiscoveryCampaigns, NumCampaigns: numCampaigns},
			InfoDiscoveryCampaignBroadKeywordsOnly{NumDiscoveryCampaigns: numDiscoveryCampaigns, NumCampaigns: numCampaigns, NumBroadInDiscovery: numBroadInDiscovery, NumBroad: numBroad},
		), nil
	}

	if verbose {
		printDiscoveryAnalysis(os.Stdout, *config, showID, groups)
	}

	return nil, ErrKeywordDiscovery{NumConflicts: len(groups)}
}

type InfoBroadKeywordInDiscoveryCampaign struct{ NumBroad, NumKeywords, NumDiscoveryCampaigns, NumCampaigns int }

func (s InfoBroadKeywordInDiscoveryCampaign) String() string {
	return "each broad keyword(" + strconv.Itoa(s.NumBroad) + "/" + strconv.Itoa(s.NumKeywords) + ") is only in discovery campaign(" + strconv.Itoa(s.NumDiscoveryCampaigns) + "/" + strconv.Itoa(s.NumCampaigns) + ")"
}

type InfoExactKeywordNegativeInDiscoveryCampaign struct{ NumExact, NumKeywords, NumDiscoveryCampaigns, NumCampaigns int }

func (s InfoExactKeywordNegativeInDiscoveryCampaign) String() string {
	return "each exact keyword(" + strconv.Itoa(s.NumExact) + "/" + strconv.Itoa(s.NumKeywords) + ") has corresponding exact negative in discovery campaign(" + strconv.Itoa(s.NumDiscoveryCampaigns) + "/" + strconv.Itoa(s.NumCampaigns) + ")"
}

type InfoDiscoveryCampaignBroadKeywordsOnly struct{ NumDiscoveryCampaigns, NumCampaigns, NumBroadInDiscovery, NumBroad int }

func (s InfoDiscoveryCampaignBroadKeywordsOnly) String() string {
	return "each discovery campaign(" + strconv.Itoa(s.NumDiscoveryCampaigns) + "/" + strconv.Itoa(s.NumCampaigns) + ") contains only broad keywords(" + strconv.Itoa(s.NumBroadInDiscovery) + "/" + strconv.Itoa(s.NumBroad) + ")"
}

type ErrKeywordDiscovery struct{ NumConflicts int }

func (e ErrKeywordDiscovery) Error() string {
	return strconv.Itoa(e.NumConflicts) + " keyword discovery conflicts found (run with -v for details)"
}
