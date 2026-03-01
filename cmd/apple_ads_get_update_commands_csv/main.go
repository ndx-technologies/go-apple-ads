package appleadsgetupdatecommandscsv

import (
	"flag"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	goappleads "github.com/ndx-technologies/go-apple-ads"
)

func campaignKeywordsPageURL(appID string, campaignID goappleads.CampaignID) string {
	return "https://app-ads.apple.com/cm/app/" + appID + "/report/campaign/" + campaignID.String() + "?tab=1"
}

func campaignKeywordsNegativePageURL(appID string, campaignID goappleads.CampaignID) string {
	return "https://app-ads.apple.com/cm/app/" + appID + "/report/campaign/" + campaignID.String() + "?tab=1&subTab=2"
}

const DocShort string = "get update commands CSV to apply to Apple Ads"

func Run(args []string) {
	flag := flag.NewFlagSet("get update-commands-csv", flag.ExitOnError)
	var fromPath, toPath, outPath, appID string
	flag.StringVar(&appID, "app-id", "", "app id for apple-ads")
	flag.StringVar(&fromPath, "from", "", "dir for apple-ads")
	flag.StringVar(&toPath, "to", "", "dir for apple-ads")
	flag.StringVar(&outPath, "out", "", "dir for commands")
	flag.Parse(args)

	if fromPath == "" || toPath == "" || outPath == "" || appID == "" {
		flag.Usage()
		os.Exit(1)
	}

	fromConfig, fromKeywordsDB, err := goappleads.Load(fromPath)
	if err != nil {
		slog.Error("cannot load from config", "error", err)
	}
	toConfig, toKeywordsDB, err := goappleads.Load(toPath)
	if err != nil {
		slog.Error("cannot load to config", "error", err)
	}

	updater := goappleads.KeywordUpdater{From: *fromKeywordsDB, To: *toKeywordsDB}

	camapigns := make(map[goappleads.CampaignID]struct{})
	regular := make(map[goappleads.CampaignID][]goappleads.KeywordCommand)
	negative := make(map[goappleads.CampaignID][]goappleads.KeywordCommand)

	for _, cmd := range updater.UpdateCommands() {
		camapigns[cmd.CampaignID] = struct{}{}
		if cmd.IsNegative {
			negative[cmd.CampaignID] = append(negative[cmd.CampaignID], cmd)
		} else {
			regular[cmd.CampaignID] = append(regular[cmd.CampaignID], cmd)
		}
	}

	for campaignID := range camapigns {
		campaign := fromConfig.GetCampaign(campaignID)

		if campaign.IsZero() {
			campaign = toConfig.GetCampaign(campaignID)
		}

		if regular := regular[campaignID]; len(regular) > 0 {
			f, err := os.Create(filepath.Join(outPath, campaign.ID.String()+"_"+campaign.Name+"_commands.csv"))
			if err != nil {
				slog.Error("cannot open file", "error", err)
			}
			defer f.Close()
			if err := goappleads.PrintCommandsToCSV(f, regular); err != nil {
				slog.Error("cannot write commands", "file", f.Name(), "error", err)
			}
			log.Println(campaign.Name + " " + campaignKeywordsPageURL(appID, campaign.ID))
		}

		if negative := negative[campaignID]; len(negative) > 0 {
			f, err := os.Create(filepath.Join(outPath, campaign.ID.String()+"_"+campaign.Name+"_negative_commands.csv"))
			if err != nil {
				slog.Error("cannot open file", "error", err)
			}
			defer f.Close()
			if err := goappleads.PrintNegativeCommandsToCSV(f, negative); err != nil {
				slog.Error("cannot write commands", "file", f.Name(), "error", err)
			}
			log.Println(campaign.Name + " " + campaignKeywordsNegativePageURL(appID, campaign.ID))
		}
	}
}
