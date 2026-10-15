package main

import (
	"sort"
	"time"
)

// MaxRates are the "how fast at their best" numbers, like N1MM's rate reports.
// Every value is in QSOs per hour.
type MaxRates struct {
	Best10Q  float64 `json:"best_10q"`  // fastest 10 contacts in a row
	Best100Q float64 `json:"best_100q"` // fastest 100 contacts in a row
	Best15   float64 `json:"best_15m"`  // most contacts in any 15 minutes, as an hourly rate
	Best30   float64 `json:"best_30m"`
	Best60   float64 `json:"best_60m"`
	// Longest stretch of running without a break or band change, and the fastest
	// run stretch of at least minStretchQSOs contacts.
	LongestRun *Stretch `json:"longest_run"`
	FastestRun *Stretch `json:"fastest_run"`
}

// A Stretch is one unbroken run: the same operator running on the same band,
// with no gap longer than rateGap between contacts.
type Stretch struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	QSOs    int       `json:"qsos"`
	Band    string    `json:"band"`
	Minutes float64   `json:"minutes"`
	Rate    float64   `json:"rate"`
}

const minStretchQSOs = 20

// A "run" of fewer contacts than this is a stray CQ answer, not a run worth listing.
const minRunQSOs = 5

// RatePoint is one contact, reduced to what the rate charts need:
// unix time, 1 if running, and the seconds of operating time it is credited with.
type RatePoint [3]int64

// bestRun is the fastest rate over n contacts in a row: n-1 gaps between the first and last.
func bestRun(qs []*QSO, n int) float64 {
	if len(qs) < n || n < 2 {
		return 0
	}
	best := 0.0
	for i := n - 1; i < len(qs); i++ {
		d := qs[i].Time.Sub(qs[i-n+1].Time)
		if d < time.Minute {
			d = time.Minute // a burst of contacts in the same minute isn't a real rate
		}
		if r := float64(n-1) / d.Hours(); r > best {
			best = r
		}
	}
	return best
}

func computeMaxRates(qs []*QSO) MaxRates {
	m := MaxRates{
		Best10Q:  bestRun(qs, 10),
		Best100Q: bestRun(qs, 100),
		Best15:   float64(bestWindow(qs, 15*time.Minute)) * 4,
		Best30:   float64(bestWindow(qs, 30*time.Minute)) * 2,
		Best60:   float64(bestWindow(qs, time.Hour)),
	}
	for _, s := range runStretches(qs) {
		s := s
		if m.LongestRun == nil || s.Minutes > m.LongestRun.Minutes {
			m.LongestRun = &s
		}
		if s.QSOs >= minStretchQSOs && (m.FastestRun == nil || s.Rate > m.FastestRun.Rate) {
			m.FastestRun = &s
		}
	}
	return m
}

// runStretches splits one operator's contacts (sorted by time) into unbroken runs.
func runStretches(qs []*QSO) []Stretch {
	var out []Stretch
	var cur *Stretch
	var prev *QSO
	flush := func() {
		if cur != nil && cur.QSOs >= minRunQSOs {
			cur.Minutes = cur.End.Sub(cur.Start).Minutes()
			if cur.Minutes > 0 {
				cur.Rate = float64(cur.QSOs-1) / (cur.Minutes / 60)
			}
			out = append(out, *cur)
		}
		cur = nil
	}
	for _, q := range qs {
		// Search-and-pounce contacts are skipped rather than ending the run, so an
		// SO2R operator pouncing on the second radio keeps their run going.
		if !q.Run {
			continue
		}
		if cur != nil && (q.Band != cur.Band || prev == nil || q.Time.Sub(prev.Time) > rateGap) {
			flush()
		}
		if cur == nil {
			cur = &Stretch{Start: q.Time, Band: q.Band}
		}
		cur.End = q.Time
		cur.QSOs++
		prev = q
	}
	flush()
	return out
}

// topStretches returns the n biggest runs, listed in time order.
func topStretches(qs []*QSO, n int) []Stretch {
	all := runStretches(qs)
	sort.Slice(all, func(i, j int) bool { return all[i].QSOs > all[j].QSOs })
	if len(all) > n {
		all = all[:n]
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Start.Before(all[j].Start) })
	return all
}

// ratePoints turns one operator's contacts into chart input. The gap before each
// contact counts as time spent on it, unless it's long enough to be a break.
func ratePoints(qs []*QSO) []RatePoint {
	out := make([]RatePoint, len(qs))
	for i, q := range qs {
		var run, credit int64
		if q.Run {
			run = 1
		}
		if i > 0 {
			if gap := q.Time.Sub(qs[i-1].Time); gap <= rateGap {
				credit = int64(gap.Seconds())
			}
		}
		out[i] = RatePoint{q.Time.Unix(), run, credit}
	}
	return out
}
