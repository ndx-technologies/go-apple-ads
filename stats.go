package goappleads

import (
	"time"
)

type BaselineMetrics struct {
	CTR, CVR, CPI   float64
	Imp, Taps, Inst int
	Spend           float64
}

type Agg struct {
	Spend float64
	Imp   int
	Taps  int
	Inst  int
	Days  map[time.Time]struct{}
}

func divSafe[T int | float32 | float64](num, denom T) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) / float64(denom)
}

func ComputeBaselines(keywords []KeywordRow) (map[CampaignID]BaselineMetrics, BaselineMetrics) {
	byCamp := make(map[CampaignID]Agg)

	total := Agg{Days: make(map[time.Time]struct{})}

	for _, r := range keywords {
		a := byCamp[r.CampaignID]
		if a.Days == nil {
			a.Days = make(map[time.Time]struct{})
		}
		a.Spend += r.Spend
		a.Imp += r.Impressions
		a.Taps += r.Taps
		a.Inst += r.Installs
		a.Days[r.Day] = struct{}{}
		byCamp[r.CampaignID] = a

		total.Spend += r.Spend
		total.Imp += r.Impressions
		total.Taps += r.Taps
		total.Inst += r.Installs
		total.Days[r.Day] = struct{}{}
	}

	baselines := make(map[CampaignID]BaselineMetrics)
	for camp, a := range byCamp {
		baselines[camp] = BaselineMetrics{
			CTR:   divSafe(a.Taps, a.Imp),
			CVR:   divSafe(a.Inst, a.Taps),
			CPI:   divSafe(a.Spend, float64(a.Inst)),
			Spend: a.Spend,
			Imp:   a.Imp,
			Taps:  a.Taps,
			Inst:  a.Inst,
		}
	}

	overall := BaselineMetrics{
		CTR:   divSafe(total.Taps, total.Imp),
		CVR:   divSafe(total.Inst, total.Taps),
		CPI:   divSafe(total.Spend, float64(total.Inst)),
		Spend: total.Spend,
		Imp:   total.Imp,
		Taps:  total.Taps,
		Inst:  total.Inst,
	}

	return baselines, overall
}
