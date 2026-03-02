package goappleads

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"sync/atomic"
)

type KeywordInfo struct {
	ID         KeywordID
	CampaignID CampaignID // cannot be changed
	AdGroupID  AdGroupID  // cannot be changed
	Keyword    string     // cannot be cahnged
	MatchType  MatchType  // cannot be changed
	Bid        float64
	Status     Status
	IsNegative bool // cannot be changed
}

func (s KeywordInfo) IsZero() bool { return s.ID.IsZero() }

func (s KeywordInfo) Label() string {
	switch s.MatchType {
	case Exact:
		return "[" + s.Keyword + "]"
	case Auto:
		return "⌕" + s.Keyword
	}
	return s.Keyword
}

type Status string

const (
	Active  Status = "ACTIVE"
	Paused  Status = "PAUSED"
	Deleted Status = "DELETED"
)

func (s Status) String() string { return string(s) }

type KeywordID string

func (s KeywordID) IsZero() bool { return s == "" }

func (s KeywordID) String() string { return string(s) }

var newKeywordIDCounter atomic.Int64

// NewKeywordID generates local version of keyword ID.
// Refer to Apple API to get real keywordID.
func NewKeywordID() KeywordID {
	return KeywordID("tmp-" + strconv.FormatInt(newKeywordIDCounter.Add(1), 10))
}

type MatchType string

const (
	Broad MatchType = "BROAD"
	Exact MatchType = "EXACT"
	Auto  MatchType = "AUTO"
)

func (s MatchType) String() string { return string(s) }

type KeywordCSVDB struct {
	Keywords      map[KeywordID]KeywordInfo
	keywordByText map[string][]KeywordID
}

func (s *KeywordCSVDB) GetKeywordInfo(id KeywordID) KeywordInfo { return s.Keywords[id] }

func (s *KeywordCSVDB) GetKeywordsByText(text string) []KeywordInfo {
	if s.keywordByText == nil {
		s.keywordByText = make(map[string][]KeywordID)
		for id, v := range s.Keywords {
			s.keywordByText[v.Keyword] = append(s.keywordByText[v.Keyword], id)
		}
	}
	if ids, ok := s.keywordByText[text]; ok {
		var result []KeywordInfo
		for _, id := range ids {
			result = append(result, s.Keywords[id])
		}
		return result
	}
	return nil
}

func (s *KeywordCSVDB) NumKeywordsInCampaign(campaignID CampaignID) int {
	count := 0
	for _, kw := range s.Keywords {
		if kw.CampaignID == campaignID {
			count++
		}
	}
	return count
}

func (s *KeywordCSVDB) NumKeywordsInAdGroup(adGroupID AdGroupID) int {
	count := 0
	for _, kw := range s.Keywords {
		if kw.AdGroupID == adGroupID {
			count++
		}
	}
	return count
}

func (s *KeywordCSVDB) LoadFromCSV(r io.Reader) error {
	reader := csv.NewReader(r)

	header, err := reader.Read()
	if err != nil {
		return err
	}

	idx := make(map[string]int)
	for i, col := range header {
		idx[col] = i
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		v := KeywordInfo{
			ID:         KeywordID(row[idx["Keyword ID"]]),
			MatchType:  MatchType(row[idx["Match Type"]]),
			CampaignID: CampaignID(row[idx["Campaign ID"]]),
			AdGroupID:  AdGroupID(row[idx["Ad Group ID"]]),
		}

		if _, ok := idx["Negative Keyword"]; ok {
			v.Keyword = row[idx["Negative Keyword"]]
			v.IsNegative = true
		} else {
			v.Keyword = row[idx["Keyword"]]
			v.Status = Status(row[idx["Status"]])
			v.Bid, err = strconv.ParseFloat(row[idx["Bid"]], 64)

			if err != nil {
				return err
			}
		}

		if s.Keywords == nil {
			s.Keywords = make(map[KeywordID]KeywordInfo)
		}

		if v.ID.IsZero() {
			v.ID = NewKeywordID()
		}

		if existing, ok := s.Keywords[v.ID]; ok {
			return &ErrDuplicateKeywordID{ID: v.ID, Existing: existing, New: v}
		}

		s.Keywords[v.ID] = v
	}

	return nil
}

type ErrDuplicateKeywordID struct {
	ID       KeywordID
	Existing KeywordInfo
	New      KeywordInfo
}

func (e *ErrDuplicateKeywordID) Error() string {
	return fmt.Sprintf("duplicate keyword ID: %s, existing: %+v, new: %+v", e.ID, e.Existing, e.New)
}
