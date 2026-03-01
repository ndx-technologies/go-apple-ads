package main

import (
	"flag"
	"log"
	"maps"
	"slices"
	"strings"

	appleadsanalysisadgroups "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_analysis_adgroups"
	appleadsanalysiscampaigns "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_analysis_campaigns"
	appleadsanalysiskeywordcannibalisation "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_analysis_keyword_cannibalisation"
	appleadsanalysiskeywordlanguagemismatch "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_analysis_keyword_language_mismatch"
	appleadsanalysiskeywords "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_analysis_keywords"
	appleadsanalysissearchterms "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_analysis_search_terms"
	appleadsgetupdatecommandscsv "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_get_update_commands_csv"
	appleadsmergecsv "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_merge_csv"
	appleadstimeline "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_timeline"
)

type CommandInfo struct {
	DocShort string
	Run      func(args []string)
}

const dos string = `Apple Ads Toolkit

Tools for structured access to Apple Ads, export/import config, apply changes, analyze data.
Use this toolkit to setup your AI-driven Apple Ads GitOps.
`

var commands = map[string]CommandInfo{
	"timeline":                           {DocShort: appleadstimeline.DocShort, Run: appleadstimeline.Run},
	"merge-csv":                          {DocShort: appleadsmergecsv.DocShort, Run: appleadsmergecsv.Run},
	"get update-commands-csv":            {DocShort: appleadsgetupdatecommandscsv.DocShort, Run: appleadsgetupdatecommandscsv.Run},
	"analyse campaigns":                  {DocShort: appleadsanalysiscampaigns.DocShort, Run: appleadsanalysiscampaigns.Run},
	"analyse adgroups":                   {DocShort: appleadsanalysisadgroups.DocShort, Run: appleadsanalysisadgroups.Run},
	"analyse keywords":                   {DocShort: appleadsanalysiskeywords.DocShort, Run: appleadsanalysiskeywords.Run},
	"analyse searchterms":                {DocShort: appleadsanalysissearchterms.DocShort, Run: appleadsanalysissearchterms.Run},
	"analyse keywords cannibalisation":   {DocShort: appleadsanalysiskeywordcannibalisation.DocShort, Run: appleadsanalysiskeywordcannibalisation.Run},
	"analyse keywords language-mismatch": {DocShort: appleadsanalysiskeywordlanguagemismatch.DocShort, Run: appleadsanalysiskeywordlanguagemismatch.Run},
}

func main() {
	cmdNames := slices.Collect(maps.Keys(commands))

	flag.Usage = func() {
		w := flag.CommandLine.Output()
		w.Write([]byte(dos))
		w.Write([]byte("\n"))

		w.Write([]byte("Usage:\n\n"))

		slices.Sort(cmdNames)
		for _, name := range cmdNames {
			w.Write([]byte(" " + name + " - " + commands[name].DocShort + "\n"))
		}
	}
	flag.Parse()

	cmd, rest, err := route(flag.Args(), cmdNames)
	if err != nil {
		log.Fatal("unknown command, use -h for help")
	}

	commands[cmd].Run(rest)
}

type ErrUnknownCommand struct{}

func (e ErrUnknownCommand) Error() string { return "unknown command" }

func route(args []string, commands []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, ErrUnknownCommand{}
	}
	bestIdx := -1
	bestLen := 0
	bestRest := []string(nil)
	for i, cmd := range commands {
		parts := strings.Fields(cmd)
		if len(args) >= len(parts) && slices.Equal(args[:len(parts)], parts) {
			if len(parts) > bestLen {
				bestIdx = i
				bestLen = len(parts)
				bestRest = args[len(parts):]
			}
		}
	}
	if bestIdx < 0 {
		return "", nil, ErrUnknownCommand{}
	}
	if len(bestRest) > 0 && !strings.HasPrefix(bestRest[0], "-") {
		return "", nil, ErrUnknownCommand{}
	}
	return commands[bestIdx], bestRest, nil
}
