package allpaused

import (
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"sort"
	"strconv"

	"github.com/ndx-technologies/fmtx"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/go-apple-ads/analysis"
)

const DocShort string = "detect AdGroups and Campaigns with all keywords paused"
const doc string = `
It is likely misconfiguration that AdGroup or Campaign is Active, yet all keywords are paused.
`

type Issue struct {
	CampaignID     goappleads.CampaignID
	AdGroupID      goappleads.AdGroupID
	ActiveKeywords int
	TotalKeywords  int
}

func Analyze(config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) []Issue {
	// count active and total (non-negative) keywords per adgroup and campaign
	activePerAdGroup := make(map[goappleads.AdGroupID]int)
	totalPerAdGroup := make(map[goappleads.AdGroupID]int)
	activePerCampaign := make(map[goappleads.CampaignID]int)
	totalPerCampaign := make(map[goappleads.CampaignID]int)

	for _, kw := range keywordsDB.Keywords {
		if kw.IsNegative {
			continue
		}
		totalPerAdGroup[kw.AdGroupID]++
		totalPerCampaign[kw.CampaignID]++
		if kw.Status != goappleads.Active {
			continue
		}
		activePerAdGroup[kw.AdGroupID]++
		activePerCampaign[kw.CampaignID]++
	}

	var issues []Issue

	for _, camp := range config.Campaigns {
		if camp.Status != goappleads.Enabled {
			continue
		}
		for _, ag := range camp.AdGroups {
			if ag.Status != goappleads.Enabled {
				continue
			}
			if activePerAdGroup[ag.ID] == 0 {
				issues = append(issues, Issue{
					CampaignID:     camp.ID,
					AdGroupID:      ag.ID,
					ActiveKeywords: activePerAdGroup[ag.ID],
					TotalKeywords:  totalPerAdGroup[ag.ID],
				})
			}
		}
		if activePerCampaign[camp.ID] == 0 {
			issues = append(issues, Issue{
				CampaignID:     camp.ID,
				ActiveKeywords: activePerCampaign[camp.ID],
				TotalKeywords:  totalPerCampaign[camp.ID],
			})
		}
	}

	return issues
}

func printAnalysis(w io.StringWriter, showID bool, config *goappleads.Config, issues []Issue) {
	fmtx.HeaderTo(w, "All Keywords Paused Analysis")

	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		campA := config.GetCampaign(a.CampaignID).Name
		campB := config.GetCampaign(b.CampaignID).Name
		if campA == campB {
			agA := config.GetAdGroup(a.AdGroupID).Name
			agB := config.GetAdGroup(b.AdGroupID).Name
			if agA == agB {
				return a.AdGroupID < b.AdGroupID
			}
			return agA < agB
		}
		return campA < campB
	})

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Campaign", Width: 36},
			{Header: "Ad Group", Width: 28},
			{Header: "Paused", Width: 6, Alignment: fmtx.AlignRight},
		},
	}

	if showID {
		tw.Cols = append([]fmtx.TablCol{
			{Header: "CampaignID", Width: 12},
			{Header: "AdGroupID", Width: 12},
		}, tw.Cols...)
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, e := range issues {
		if e.TotalKeywords == 0 {
			continue
		}

		ratio := 100 * (e.TotalKeywords - e.ActiveKeywords) / e.TotalKeywords

		campaign := config.GetCampaign(e.CampaignID)

		var adgroupName string
		if e.AdGroupID != "" {
			adgroupName = config.GetAdGroup(e.AdGroupID).Name
		}

		row := []string{
			campaign.Name,
			adgroupName,
			fmtx.RedS(strconv.Itoa(ratio) + "%"),
		}
		if showID {
			row = append([]string{fmtx.DimS(string(e.CampaignID)), fmtx.DimS(string(e.AdGroupID))}, row...)
		}
		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

func Run(args []string) (analysis.Info, error) {
	fs := flag.NewFlagSet("analyse keywords all-paused", flag.ExitOnError)
	var (
		applePath string
		showID    bool
		verbose   bool
	)
	fs.Usage = func() {
		fs.Output().Write([]byte(doc))
		fs.PrintDefaults()
	}
	fs.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	fs.BoolVar(&showID, "id", false, "show IDs")
	fs.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	fs.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	fs.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	issues := Analyze(config, keywordsDB)

	if len(issues) == 0 {
		var numCampaigns, numAdGroups int
		for _, camp := range config.Campaigns {
			if camp.Status != goappleads.Enabled {
				continue
			}
			numCampaigns++
			for _, ag := range camp.AdGroups {
				if ag.Status == goappleads.Enabled {
					numAdGroups++
				}
			}
		}
		return analysis.Join(
			InfoCampaignsKeywordsStatus{NumCampaigns: numCampaigns},
			InfoAdGroupsKeywordsStatus{NumAdGroups: numAdGroups},
		), nil
	}

	if verbose {
		printAnalysis(os.Stdout, showID, config, issues)
	}

	var numAdGroup, numCampaign int
	for _, iss := range issues {
		if iss.AdGroupID != "" {
			numAdGroup++
		} else {
			numCampaign++
		}
	}
	var errs []error
	if numAdGroup > 0 {
		errs = append(errs, ErrAllPausedAdGroup{N: numAdGroup})
	}
	if numCampaign > 0 {
		errs = append(errs, ErrAllPausedCampaign{N: numCampaign})
	}
	return nil, errors.Join(errs...)
}

type InfoCampaignsKeywordsStatus struct{ NumCampaigns int }

func (s InfoCampaignsKeywordsStatus) String() string {
	return "all active campaigns(" + strconv.Itoa(s.NumCampaigns) + ") have active keywords"
}

type InfoAdGroupsKeywordsStatus struct{ NumAdGroups int }

func (s InfoAdGroupsKeywordsStatus) String() string {
	return "all active adgroups(" + strconv.Itoa(s.NumAdGroups) + ") have active keywords"
}

type ErrAllPausedAdGroup struct{ N int }

func (e ErrAllPausedAdGroup) Error() string {
	return strconv.Itoa(e.N) + " active adgroups have all keywords paused (run with -v for details)"
}

type ErrAllPausedCampaign struct{ N int }

func (e ErrAllPausedCampaign) Error() string {
	return strconv.Itoa(e.N) + " active campaigns have all keywords paused (run with -v for details)"
}
