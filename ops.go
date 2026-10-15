package main

import (
	"sort"
	"strings"
	"time"
)

// A gap longer than this between an operator's contacts counts as a break, not
// time spent running or searching. Short enough that a coffee break doesn't drag
// the rate down, long enough to cover a slow patch on a dead band.
const rateGap = 10 * time.Minute

// Contest rules usually call 30 minutes or more without a contact an off-time.
const offTimeGap = 30 * time.Minute

type OpStats struct {
	Name      string         `json:"name"`
	QSOs      int            `json:"qsos"`
	Points    int            `json:"points"`
	Mults     int            `json:"mults"`
	RunQSOs   int            `json:"run_qsos"`
	SPQSOs    int            `json:"sp_qsos"`
	RunHours  float64        `json:"run_hours"`
	SPHours   float64        `json:"sp_hours"`
	RunRate   float64        `json:"run_rate"` // QSOs per hour while running
	SPRate    float64        `json:"sp_rate"`  // QSOs per hour while searching and pouncing
	OnHours   float64        `json:"on_hours"` // time on the air, breaks of 30 minutes or more left out
	AvgRate   float64        `json:"avg_rate"` // QSOs per hour on the air
	Best60    int            `json:"best60"`   // most QSOs in any 60 minutes
	Best10    int            `json:"best10"`   // most QSOs in any 10 minutes
	Last60    int            `json:"last60"`
	Countries int            `json:"countries"`
	Share     float64        `json:"share"` // percent of the station's QSOs
	First     time.Time      `json:"first"`
	Last      time.Time      `json:"last"`
	Bands     map[string]int `json:"bands"`
	Modes     map[string]int `json:"modes"`
	Stations  []string       `json:"stations"`
}

type OpsData struct {
	Version   int64        `json:"version"`
	Active    *ContestInfo `json:"active"`
	Operators []OpStats    `json:"operators"`
	// Cumulative QSOs per operator at the end of each time slot, for the comparison chart.
	Slots   []time.Time      `json:"slots"`
	Cumul   map[string][]int `json:"cumulative"`
	SlotMin int              `json:"slot_min"`
}

type OpDetail struct {
	Version   int64                     `json:"version"`
	Active    *ContestInfo              `json:"active"`
	Stats     OpStats                   `json:"stats"`
	Units     string                    `json:"units"`
	BandMode  map[string]map[string]int `json:"band_mode"`
	BandList  []string                  `json:"bands"`
	Modes     []string                  `json:"modes"`
	Timeline  []RunSPBucket             `json:"timeline"`
	BucketMin int                       `json:"bucket_min"`
	Hourly    []Bucket                  `json:"hourly"`
	Recent    []RecentQSO               `json:"recent"`
	Countries []Count                   `json:"top_countries"`
}

type RunSPBucket struct {
	Start time.Time `json:"t"`
	Run   int       `json:"run"`
	SP    int       `json:"sp"`
}

func opName(q *QSO) string {
	if q.Operator == "" {
		return "?"
	}
	return q.Operator
}

// computeOpStats works out one operator's numbers from their contacts (sorted by time).
func computeOpStats(name string, qs []*QSO, total int, ref time.Time) OpStats {
	st := OpStats{Name: name, Bands: map[string]int{}, Modes: map[string]int{}}
	countries := map[string]bool{}
	stations := map[string]bool{}
	for i, q := range qs {
		st.QSOs++
		st.Points += q.Points
		if q.Mult {
			st.Mults++
		}
		if q.Run {
			st.RunQSOs++
		} else {
			st.SPQSOs++
		}
		st.Bands[q.Band]++
		st.Modes[q.Mode]++
		if q.Station != "" {
			stations[q.Station] = true
		}
		if e, ok := cty.Lookup(q.Call); ok {
			countries[e.Name] = true
		}
		if ref.Sub(q.Time) <= time.Hour && !q.Time.After(ref.Add(time.Minute)) {
			st.Last60++
		}
		if i == 0 {
			continue
		}
		// Time between two contacts belongs to whatever the operator was doing for the second one.
		gap := q.Time.Sub(qs[i-1].Time)
		if gap <= rateGap {
			if q.Run {
				st.RunHours += gap.Hours()
			} else {
				st.SPHours += gap.Hours()
			}
		}
		if gap < offTimeGap {
			st.OnHours += gap.Hours()
		}
	}
	if len(qs) > 0 {
		st.First, st.Last = qs[0].Time, qs[len(qs)-1].Time
	}
	st.RunRate = rate(st.RunQSOs, st.RunHours)
	st.SPRate = rate(st.SPQSOs, st.SPHours)
	st.AvgRate = rate(st.QSOs, st.OnHours)
	st.Best60 = bestWindow(qs, time.Hour)
	st.Best10 = bestWindow(qs, 10*time.Minute)
	st.Countries = len(countries)
	if total > 0 {
		st.Share = 100 * float64(st.QSOs) / float64(total)
	}
	for s := range stations {
		st.Stations = append(st.Stations, s)
	}
	sort.Strings(st.Stations)
	return st
}

// rate is QSOs per hour, left at zero when there isn't enough time to say.
func rate(n int, hours float64) float64 {
	if hours < 1.0/60 {
		return 0
	}
	return float64(n) / hours
}

func bestWindow(qs []*QSO, w time.Duration) int {
	best, j := 0, 0
	for i := range qs {
		for qs[i].Time.Sub(qs[j].Time) >= w {
			j++
		}
		if n := i - j + 1; n > best {
			best = n
		}
	}
	return best
}

func groupByOp(qsos []*QSO) map[string][]*QSO {
	m := map[string][]*QSO{}
	for _, q := range qsos {
		m[opName(q)] = append(m[opName(q)], q)
	}
	return m
}

func activeQSOs(snap Snapshot, cfg Config, now time.Time) (*ContestInfo, []*QSO, time.Time) {
	active := pickContest(listContests(snap.QSOs), cfg.Contest)
	if active == nil {
		return nil, nil, now
	}
	ref := now
	if now.Sub(active.Last) > 6*time.Hour {
		ref = active.Last
	}
	return active, filterContest(snap.QSOs, active.Key), ref
}

func BuildOps(snap Snapshot, cfg Config, now time.Time) *OpsData {
	d := &OpsData{Version: snap.Version, Operators: []OpStats{}, Cumul: map[string][]int{}}
	active, qsos, ref := activeQSOs(snap, cfg, now)
	d.Active = active
	if len(qsos) == 0 {
		return d
	}
	for name, qs := range groupByOp(qsos) {
		d.Operators = append(d.Operators, computeOpStats(name, qs, len(qsos), ref))
	}
	sort.Slice(d.Operators, func(i, j int) bool {
		if d.Operators[i].QSOs != d.Operators[j].QSOs {
			return d.Operators[i].QSOs > d.Operators[j].QSOs
		}
		return d.Operators[i].Name < d.Operators[j].Name
	})

	d.SlotMin = slotMinutes(qsos)
	size := time.Duration(d.SlotMin) * time.Minute
	start := qsos[0].Time.Truncate(size)
	n := min(int(qsos[len(qsos)-1].Time.Sub(start)/size)+1, 2000)
	for i := 0; i < n; i++ {
		d.Slots = append(d.Slots, start.Add(time.Duration(i)*size))
	}
	for _, o := range d.Operators {
		d.Cumul[o.Name] = make([]int, n)
	}
	for _, q := range qsos {
		if i := int(q.Time.Sub(start) / size); i >= 0 && i < n {
			d.Cumul[opName(q)][i]++
		}
	}
	for _, c := range d.Cumul {
		for i := 1; i < len(c); i++ {
			c[i] += c[i-1]
		}
	}
	return d
}

func BuildOpDetail(snap Snapshot, cfg Config, op string, now time.Time) *OpDetail {
	d := &OpDetail{Version: snap.Version, Units: cfg.Units, BandMode: map[string]map[string]int{}}
	active, qsos, ref := activeQSOs(snap, cfg, now)
	d.Active = active
	var mine []*QSO
	for _, q := range qsos {
		if strings.EqualFold(opName(q), op) {
			mine = append(mine, q)
		}
	}
	d.Stats = computeOpStats(strings.ToUpper(op), mine, len(qsos), ref)
	if len(mine) == 0 {
		return d
	}
	countries := map[string]*Count{}
	modeSeen := map[string]bool{}
	for _, q := range mine {
		if d.BandMode[q.Band] == nil {
			d.BandMode[q.Band] = map[string]int{}
			d.BandList = append(d.BandList, q.Band)
		}
		d.BandMode[q.Band][q.Mode]++
		modeSeen[q.Mode] = true
		if e, ok := cty.Lookup(q.Call); ok {
			c := countries[e.Name]
			if c == nil {
				c = &Count{Name: e.Name}
				countries[e.Name] = c
			}
			c.QSOs++
		}
	}
	sort.Slice(d.BandList, func(i, j int) bool { return bandIndex(d.BandList[i]) < bandIndex(d.BandList[j]) })
	for _, m := range []string{"CW", "PH", "DG", ""} {
		if modeSeen[m] {
			d.Modes = append(d.Modes, m)
		}
	}
	d.Countries = sortedCounts(countries, 12)

	// Run and S&P contacts over the whole contest, so the chart lines up with the station's.
	d.BucketMin = slotMinutes(qsos)
	size := time.Duration(d.BucketMin) * time.Minute
	start := qsos[0].Time.Truncate(size)
	n := min(int(qsos[len(qsos)-1].Time.Sub(start)/size)+1, 2000)
	d.Timeline = make([]RunSPBucket, n)
	for i := range d.Timeline {
		d.Timeline[i].Start = start.Add(time.Duration(i) * size)
	}
	for _, q := range mine {
		if i := int(q.Time.Sub(start) / size); i >= 0 && i < n {
			if q.Run {
				d.Timeline[i].Run++
			} else {
				d.Timeline[i].SP++
			}
		}
	}
	d.Hourly = buckets(mine, time.Hour)

	homeLat, homeLon, haveHome := homeLocation(cfg.MyGrid, firstNonEmpty(cfg.MyCall, mine[len(mine)-1].MyCall))
	for i := len(mine) - 1; i >= 0 && len(d.Recent) < 30; i-- {
		q := mine[i]
		l := locate(q)
		r := RecentQSO{Time: q.Time, Call: q.Call, Band: q.Band, Mode: q.Mode, Operator: q.Operator,
			Station: q.Station, Exchange: q.Exchange, Country: l.country, Run: q.Run, Mult: q.Mult}
		if haveHome && l.ok {
			r.Dist = distanceKm(homeLat, homeLon, l.lat, l.lon)
		}
		d.Recent = append(d.Recent, r)
	}
	return d
}
