package goappleads

import (
	"encoding/csv"
	"io"
	"strconv"
)

type Action string

const (
	Create Action = "CREATE"
	Update Action = "UPDATE"
	Delete Action = "DELETE"
)

func (s Action) String() string { return string(s) }

type KeywordCommand struct {
	Action     Action
	KeywordID  KeywordID
	Keyword    string
	MatchType  MatchType
	Bid        float64
	Status     Status
	IsNegative bool
	CampaignID CampaignID
	AdGroupID  AdGroupID
}

type KeywordUpdater struct{ From, To KeywordCSVDB }

type keywordKey struct {
	CampaignID CampaignID
	AdGroupID  AdGroupID
	Keyword    string
	MatchType  MatchType
	IsNegative bool
}

func (s KeywordUpdater) UpdateCommands() []KeywordCommand {
	toMap := make(map[keywordKey]KeywordInfo, len(s.To.Keywords))
	for _, kw := range s.To.Keywords {
		toMap[keywordKey{kw.CampaignID, kw.AdGroupID, kw.Keyword, kw.MatchType, kw.IsNegative}] = kw
	}

	fromMap := make(map[keywordKey]KeywordInfo, len(s.From.Keywords))
	for _, kw := range s.From.Keywords {
		fromMap[keywordKey{kw.CampaignID, kw.AdGroupID, kw.Keyword, kw.MatchType, kw.IsNegative}] = kw
	}

	var cmds []KeywordCommand

	for k, to := range toMap {
		if from, ok := fromMap[k]; ok {
			if from.Bid != to.Bid || from.Status != to.Status {
				cmds = append(cmds, KeywordCommand{
					Action:     Update,
					KeywordID:  from.ID,
					Keyword:    to.Keyword,
					MatchType:  to.MatchType,
					Status:     to.Status,
					IsNegative: to.IsNegative,
					Bid:        to.Bid,
					CampaignID: to.CampaignID,
					AdGroupID:  to.AdGroupID,
				})
			}
		} else {
			cmds = append(cmds, KeywordCommand{
				Action:     Create,
				Keyword:    to.Keyword,
				MatchType:  to.MatchType,
				Status:     to.Status,
				IsNegative: to.IsNegative,
				Bid:        to.Bid,
				CampaignID: to.CampaignID,
				AdGroupID:  to.AdGroupID,
			})
		}
	}

	for k, from := range fromMap {
		if _, ok := toMap[k]; !ok {
			cmds = append(cmds, KeywordCommand{
				Action:     Delete,
				KeywordID:  from.ID,
				Keyword:    from.Keyword,
				MatchType:  from.MatchType,
				Status:     from.Status,
				IsNegative: from.IsNegative,
				Bid:        from.Bid,
				CampaignID: from.CampaignID,
				AdGroupID:  from.AdGroupID,
			})
		}
	}

	return cmds
}

func PrintCommandsToCSV(w io.Writer, cmds []KeywordCommand) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"Action", "Keyword ID", "Keyword", "Match Type", "Status", "Bid", "Campaign ID", "Ad Group ID"}); err != nil {
		return err
	}
	for _, cmd := range cmds {
		if err := cw.Write([]string{
			cmd.Action.String(),
			cmd.KeywordID.String(),
			cmd.Keyword,
			cmd.MatchType.String(),
			cmd.Status.String(),
			strconv.FormatFloat(cmd.Bid, 'f', -1, 64),
			cmd.CampaignID.String(),
			cmd.AdGroupID.String(),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func PrintNegativeCommandsToCSV(w io.Writer, cmds []KeywordCommand) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"Action", "Keyword ID", "Negative Keyword", "Match Type", "Campaign ID", "Ad Group ID"}); err != nil {
		return err
	}
	for _, cmd := range cmds {
		if err := cw.Write([]string{
			cmd.Action.String(),
			cmd.KeywordID.String(),
			cmd.Keyword,
			cmd.MatchType.String(),
			cmd.CampaignID.String(),
			cmd.AdGroupID.String(),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
