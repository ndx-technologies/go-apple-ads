package main

import (
	"flag"
	"log"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/ndx-technologies/fmtx"
	"github.com/ndx-technologies/go-apple-ads/analysis"
	analysiskeywordcannibalisation "github.com/ndx-technologies/go-apple-ads/analysis/keyword_cannibalisation"
	analysiskeyworddiscovery "github.com/ndx-technologies/go-apple-ads/analysis/keyword_discovery"
	analysiskeywordlanguagemismatch "github.com/ndx-technologies/go-apple-ads/analysis/keyword_language_mismatch"
	appleadsgetupdatecommandscsv "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_get_update_commands_csv"
	appleadsmergecsv "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_merge_csv"
	appleadstimeline "github.com/ndx-technologies/go-apple-ads/cmd/apple_ads_timeline"
	statsadgroups "github.com/ndx-technologies/go-apple-ads/cmd/stats_adgroups"
	statscampaigns "github.com/ndx-technologies/go-apple-ads/cmd/stats_campaigns"
	statskeywords "github.com/ndx-technologies/go-apple-ads/cmd/stats_keywords"
	statssearchterms "github.com/ndx-technologies/go-apple-ads/cmd/stats_searchterms"
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
	"stats campaigns":                    {DocShort: statscampaigns.DocShort, Run: statscampaigns.Run},
	"stats adgroups":                     {DocShort: statsadgroups.DocShort, Run: statsadgroups.Run},
	"stats keywords":                     {DocShort: statskeywords.DocShort, Run: statskeywords.Run},
	"stats searchterms":                  {DocShort: statssearchterms.DocShort, Run: statssearchterms.Run},
	"analyse keywords discovery":         {DocShort: analysiskeyworddiscovery.DocShort, Run: analyse(analysiskeyworddiscovery.Run)},
	"analyse keywords cannibalisation":   {DocShort: analysiskeywordcannibalisation.DocShort, Run: analyse(analysiskeywordcannibalisation.Run)},
	"analyse keywords language-mismatch": {DocShort: analysiskeywordlanguagemismatch.DocShort, Run: analyse(analysiskeywordlanguagemismatch.Run)},
	"analyse": {DocShort: "run all analysers", Run: analyse(
		analysiskeyworddiscovery.Run,
		analysiskeywordcannibalisation.Run,
		analysiskeywordlanguagemismatch.Run,
	)},
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

	cmd, rest := route(flag.Args(), cmdNames)
	if cmd == "" {
		log.Fatal("unknown command, use -h for help")
	}

	commands[cmd].Run(rest)
}

func route(args []string, commands []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
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
		return "", nil
	}
	if len(bestRest) > 0 && !strings.HasPrefix(bestRest[0], "-") {
		return "", nil
	}
	return commands[bestIdx], bestRest
}

func analyse(fs ...func(args []string) (analysis.Info, error)) func(args []string) {
	return func(args []string) {
		w := os.Stderr
		hasError := false
		for _, f := range fs {
			info, err := f(args)
			if err != nil {
				w.WriteString(fmtx.RedS("error") + " " + err.Error() + "\n")
				hasError = true
			}
			if info != nil {
				if ci, ok := info.(analysis.CompositeInfo); ok {
					for _, i := range ci.Infos {
						w.WriteString(fmtx.GreenS("ok") + " " + i.String() + "\n")
					}
				} else {
					w.WriteString(fmtx.GreenS("ok") + " " + info.String() + "\n")
				}
			}
		}
		if hasError {
			os.Exit(1)
		}
	}
}
