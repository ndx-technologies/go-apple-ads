package goappleads

import (
	"math"
	"sort"
	"time"

	"github.com/ndx-technologies/tdigest"
)

// LearningThreshold is the minimum cumulative installs per ad group for Apple Ads learning phase to complete.
// Based on empirical observation: BR Search Results adgroup saw 4× daily-install spike exactly when cumulative
// installs crossed 50 (day 34). Taps crossing 50 on day 8 had no effect, ruling them out.
// Aligns with Apple's published guidance of ~50 tap-through conversions.
const LearningThreshold = 50

// learningPhaseResetCPTPct is the change in median CPT that triggers a new learning phase.
const learningPhaseResetCPTPct = 0.10

// learningPhaseResetKWRatio is the change in keyword count that triggers a new learning phase.
const learningPhaseResetKWRatio = 0.20

// LearningStatus describes whether an ad group has finished the Apple Ads learning phase.
type LearningStatus struct {
	EverLearned bool // EverLearned is true if the adgroup has completed at least one learning phase.

	// Set when the current phase has reached LearningThreshold.
	LearnedOn        time.Time
	DaysSinceLearned int

	// Set when still learning in the current phase.
	CumInst          int // cumulative installs in current phase
	EstDaysRemaining int // -1 if rate is zero or unknown
}

func (s LearningStatus) IsLearned() bool    { return !s.LearnedOn.IsZero() }
func (s LearningStatus) IsRelearning() bool { return s.EverLearned && !s.IsLearned() }

// ComputeLearningByAdGroup computes LearningStatus per ad group from keyword stats rows.
// It detects learning phase resets when CPT shifts >10% or keyword count changes by ≥20%.
// today is used to compute DaysSinceLearned.
func ComputeLearningByAdGroup(rows []KeywordRow, threshold int, today time.Time) map[AdGroupID]LearningStatus {
	type rawDay struct {
		inst    int
		cpt     tdigest.TDigest
		kwCount int
	}
	byAG := make(map[AdGroupID]map[time.Time]*rawDay)
	for _, r := range rows {
		if byAG[r.AdGroupID] == nil {
			byAG[r.AdGroupID] = make(map[time.Time]*rawDay)
		}
		d := byAG[r.AdGroupID][r.Day]
		if d == nil {
			d = &rawDay{}
			byAG[r.AdGroupID][r.Day] = d
		}
		d.inst += r.Installs
		if r.MaxCPTBid > 0 {
			d.cpt.Insert(float32(r.MaxCPTBid), 1)
		}
		d.kwCount++
	}

	result := make(map[AdGroupID]LearningStatus)
	for ag, dayMap := range byAG {
		days := make([]time.Time, 0, len(dayMap))
		for day := range dayMap {
			days = append(days, day)
		}
		sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })

		cum := 0
		var learnedOn time.Time
		everLearned := false
		prevCPT := float32(0)
		prevKW := 0

		for _, day := range days {
			d := dayMap[day]
			medCPT := d.cpt.Quantile(0.5)

			reset := false
			if prevCPT > 0 && medCPT > 0 {
				delta := (medCPT - prevCPT) / prevCPT
				if delta < 0 {
					delta = -delta
				}
				if delta >= learningPhaseResetCPTPct {
					reset = true
				}
			}
			if prevKW > 0 {
				kwDelta := d.kwCount - prevKW
				if kwDelta < 0 {
					kwDelta = -kwDelta
				}
				if float64(kwDelta)/float64(prevKW) >= learningPhaseResetKWRatio {
					reset = true
				}
			}

			if reset && !learnedOn.IsZero() {
				everLearned = true
				learnedOn = time.Time{}
				cum = 0
			} else if reset {
				cum = 0
			}

			if medCPT > 0 {
				prevCPT = medCPT
			}
			prevKW = d.kwCount

			cum += d.inst
			if learnedOn.IsZero() && cum >= threshold {
				learnedOn = day
			}
		}

		if !learnedOn.IsZero() {
			result[ag] = LearningStatus{
				EverLearned:      true,
				LearnedOn:        learnedOn,
				DaysSinceLearned: int(today.Sub(learnedOn).Hours() / 24),
			}
			continue
		}

		const recentWindow = 7
		recentDays := days[max(0, len(days)-recentWindow):]
		recentInst := 0
		for _, day := range recentDays {
			recentInst += dayMap[day].inst
		}
		recentRate := float64(recentInst) / float64(len(recentDays))

		remaining := threshold - cum
		estDays := -1
		if recentRate > 0 {
			estDays = int(math.Ceil(float64(remaining) / recentRate))
		}

		result[ag] = LearningStatus{
			EverLearned:      everLearned,
			CumInst:          cum,
			EstDaysRemaining: estDays,
		}
	}

	return result
}
