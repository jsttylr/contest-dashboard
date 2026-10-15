package main

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

func itoa(n int) string { return strconv.Itoa(n) }

// contestKey groups contacts into one contest log. N1MM's contest number tells
// apart two years of the same contest.
func contestKey(q *QSO) string {
	name := q.Contest
	if name == "" {
		name = "No contest"
	}
	if q.ContestNr != "" {
		return name + " #" + q.ContestNr
	}
	return name
}

var bandOrder = []string{"160m", "80m", "60m", "40m", "30m", "20m", "17m", "15m", "12m", "10m", "6m", "4m", "2m", "1.25m", "70cm", "33cm", "23cm", ""}

func bandIndex(b string) int {
	for i, x := range bandOrder {
		if x == b {
			return i
		}
	}
	return len(bandOrder)
}

type ContestInfo struct {
	Key   string    `json:"key"`
	Name  string    `json:"name"`
	QSOs  int       `json:"qsos"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

type Count struct {
	Name  string `json:"name"`
	QSOs  int    `json:"qsos"`
	Last  int    `json:"last60"`
	Run   int    `json:"run"`
	Mults int    `json:"mults"`
	Pts   int    `json:"points"`
}

type Bucket struct {
	Start  time.Time      `json:"t"`
	ByBand map[string]int `json:"by_band"`
	Total  int            `json:"total"`
}

type RecentQSO struct {
	Time     time.Time `json:"time"`
	Call     string    `json:"call"`
	Band     string    `json:"band"`
	Mode     string    `json:"mode"`
	Operator string    `json:"op"`
	Station  string    `json:"station"`
	Exchange string    `json:"exch"`
	Country  string    `json:"country"`
	Run      bool      `json:"run"`
	Mult     bool      `json:"mult"`
	Dist     float64   `json:"dist"`
}

type Summary struct {
	Version  int64         `json:"version"`
	Contests []ContestInfo `json:"contests"`
	Active   *ContestInfo  `json:"active"`
	RefTime  time.Time     `json:"ref_time"`
	Live     bool          `json:"live"`
	MyCall   string        `json:"mycall"`
	Units    string        `json:"units"`
	Totals   struct {
		QSOs        int            `json:"qsos"`
		Points      int64          `json:"points"`
		Mults       int            `json:"mults"`
		Score       int64          `json:"score"`
		ScoreSource string         `json:"score_source"` // "logger" or "estimate"
		ScoreAge    int            `json:"score_age_s"`
		Countries   int            `json:"countries"`
		Continents  map[string]int `json:"continents"`
		RunQSOs     int            `json:"run_qsos"`
	} `json:"totals"`
	Rates struct {
		Last10 int     `json:"last10"` // QSOs in the last 10 minutes
		Last60 int     `json:"last60"` // QSOs in the last hour
		Best60 int     `json:"best60"` // best clock-free 60 minutes
		Hours  float64 `json:"hours"`  // hours from first to last contact
	} `json:"rates"`
	Bands        []string                  `json:"bands"`
	Modes        []string                  `json:"modes"`
	BandMode     map[string]map[string]int `json:"band_mode"`
	BandMults    map[string]int            `json:"band_mults"`
	Timeline     []Bucket                  `json:"timeline"`
	BucketMin    int                       `json:"bucket_min"`
	Hourly       []Bucket                  `json:"hourly"`
	Operators    []Count                   `json:"operators"`
	Stations     []Count                   `json:"stations"`
	TopCountries []Count                   `json:"top_countries"`
	Radios       []RadioInfo               `json:"radios"`
	Recent       []RecentQSO               `json:"recent"`
}

type MapPoint struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Call  string  `json:"call"`
	Band  string  `json:"band"`
	Mode  string  `json:"mode"`
	Time  int64   `json:"t"`
	Where string  `json:"where"`
}

type MapData struct {
	Version int64       `json:"version"`
	Home    *[2]float64 `json:"home"`
	Points  []MapPoint  `json:"points"`
}

func listContests(qsos []*QSO) []ContestInfo {
	byKey := map[string]*ContestInfo{}
	for _, q := range qsos {
		k := contestKey(q)
		c := byKey[k]
		if c == nil {
			name := q.Contest
			if name == "" {
				name = "No contest"
			}
			c = &ContestInfo{Key: k, Name: name, First: q.Time}
			byKey[k] = c
		}
		c.QSOs++
		if q.Time.After(c.Last) {
			c.Last = q.Time
		}
		if q.Time.Before(c.First) {
			c.First = q.Time
		}
	}
	out := []ContestInfo{}
	for _, c := range byKey {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	return out
}

// pickContest returns the chosen contest, or the one with the latest contact.
func pickContest(contests []ContestInfo, want string) *ContestInfo {
	for i := range contests {
		if contests[i].Key == want {
			return &contests[i]
		}
	}
	if len(contests) > 0 {
		return &contests[0]
	}
	return nil
}

type location struct {
	lat, lon float64
	where    string
	country  string
	cont     string
	ok       bool
}

// locate places a contact: grid square first, then state/section, then country.
func locate(q *QSO) location {
	ent, found := cty.Lookup(q.Call)
	l := location{country: ent.Name, cont: ent.Continent}
	if lat, lon, ok := GridToLatLon(q.Grid); ok {
		l.lat, l.lon, l.where, l.ok = lat, lon, q.Grid, true
		return l
	}
	if found && (ent.Prefix == "K" || ent.Prefix == "VE" || ent.Prefix == "KL" || ent.Prefix == "KH6") {
		for _, s := range []string{q.Section, q.Exchange} {
			if lat, lon, ok := RegionLatLon(s); ok {
				dl, dn := jitter(q.Call, 0.8)
				l.lat, l.lon, l.where, l.ok = lat+dl, lon+dn, strings.ToUpper(s), true
				return l
			}
		}
	}
	if found {
		spread := 1.5
		if ent.Prefix == "K" || ent.Prefix == "VE" || ent.Prefix == "UA9" || ent.Prefix == "VK" || ent.Prefix == "PY" {
			spread = 4
		}
		dl, dn := jitter(q.Call, spread)
		l.lat, l.lon, l.where, l.ok = ent.Lat+dl, ent.Lon+dn, ent.Name, true
	}
	return l
}

func filterContest(qsos []*QSO, key string) []*QSO {
	var out []*QSO
	for _, q := range qsos {
		if contestKey(q) == key {
			out = append(out, q)
		}
	}
	return out
}

// BuildSummary works out everything the dashboard shows for one contest.
func BuildSummary(snap Snapshot, cfg Config, now time.Time) *Summary {
	s := &Summary{Version: snap.Version, Units: cfg.Units, MyCall: cfg.MyCall,
		BandMode: map[string]map[string]int{}, BandMults: map[string]int{}}
	s.Totals.Continents = map[string]int{}
	s.Contests = listContests(snap.QSOs)
	s.Active = pickContest(s.Contests, cfg.Contest)
	// Radios that have gone quiet for two minutes are switched off or disconnected.
	for _, r := range snap.Radios {
		if now.Sub(r.Seen) < 2*time.Minute {
			s.Radios = append(s.Radios, r)
		}
	}
	s.RefTime = now
	s.Live = true
	if s.Active == nil {
		return s
	}
	qsos := filterContest(snap.QSOs, s.Active.Key)
	// Looking back at a finished contest: measure rates from its last contact.
	if now.Sub(s.Active.Last) > 6*time.Hour {
		s.RefTime = s.Active.Last
		s.Live = false
	}
	ref := s.RefTime
	if s.MyCall == "" && len(qsos) > 0 {
		s.MyCall = qsos[len(qsos)-1].MyCall
	}

	homeLat, homeLon, haveHome := homeLocation(cfg.MyGrid, s.MyCall)

	bandSeen := map[string]bool{}
	modeSeen := map[string]bool{}
	ops := map[string]*Count{}
	stations := map[string]*Count{}
	countries := map[string]*Count{}
	var points int64
	mults := 0
	for _, q := range qsos {
		s.Totals.QSOs++
		points += int64(q.Points)
		if q.Mult {
			mults++
			s.BandMults[q.Band]++
		}
		if q.Run {
			s.Totals.RunQSOs++
		}
		bandSeen[q.Band] = true
		modeSeen[q.Mode] = true
		if s.BandMode[q.Band] == nil {
			s.BandMode[q.Band] = map[string]int{}
		}
		s.BandMode[q.Band][q.Mode]++
		last := ref.Sub(q.Time) <= time.Hour && !q.Time.After(ref.Add(time.Minute))
		if ref.Sub(q.Time) <= 10*time.Minute && !q.Time.After(ref.Add(time.Minute)) {
			s.Rates.Last10++
		}
		if last {
			s.Rates.Last60++
		}
		add := func(m map[string]*Count, k string) {
			if k == "" {
				k = "?"
			}
			c := m[k]
			if c == nil {
				c = &Count{Name: k}
				m[k] = c
			}
			c.QSOs++
			c.Pts += q.Points
			if last {
				c.Last++
			}
			if q.Run {
				c.Run++
			}
			if q.Mult {
				c.Mults++
			}
		}
		add(ops, q.Operator)
		add(stations, q.Station)
		ent, _ := cty.Lookup(q.Call)
		if ent.Name != "" {
			add(countries, ent.Name)
			if countries[ent.Name].QSOs == 1 {
				s.Totals.Continents[ent.Continent]++
			}
		}
	}
	s.Totals.Countries = len(countries)
	s.Totals.Points = points
	s.Totals.Mults = mults

	s.Rates.Best60 = bestWindow(qsos, time.Hour)
	if len(qsos) > 1 {
		s.Rates.Hours = qsos[len(qsos)-1].Time.Sub(qsos[0].Time).Hours()
	}

	// The logger's own score is the truth when it's sent; otherwise estimate it.
	var best *Score
	for i := range snap.Scores {
		sc := &snap.Scores[i]
		// Only score reports sent while this contest was on the air, from the same station.
		if sc.Received.Before(s.Active.First.Add(-time.Hour)) || sc.Received.After(s.Active.Last.Add(6*time.Hour)) {
			continue
		}
		if sc.Call != "" && s.MyCall != "" && !strings.EqualFold(sc.Call, s.MyCall) {
			continue
		}
		if best == nil || sc.Received.After(best.Received) {
			best = sc
		}
	}
	if best != nil && best.Score > 0 {
		s.Totals.Score = best.Score
		s.Totals.ScoreSource = "logger"
		s.Totals.ScoreAge = int(now.Sub(best.Received).Seconds())
		if best.Mults > 0 {
			s.Totals.Mults = best.Mults
		}
		if best.Points > 0 {
			s.Totals.Points = best.Points
		}
	} else {
		m := int64(mults)
		if m == 0 {
			m = 1
		}
		s.Totals.Score = points * m
		s.Totals.ScoreSource = "estimate"
	}

	for b := range bandSeen {
		s.Bands = append(s.Bands, b)
	}
	sort.Slice(s.Bands, func(i, j int) bool { return bandIndex(s.Bands[i]) < bandIndex(s.Bands[j]) })
	for _, m := range []string{"CW", "PH", "DG", ""} {
		if modeSeen[m] {
			s.Modes = append(s.Modes, m)
		}
	}

	// Rate timeline: bucket size scales with the contest length.
	if len(qsos) > 0 {
		s.BucketMin = slotMinutes(qsos)
		s.Timeline = buckets(qsos, time.Duration(s.BucketMin)*time.Minute)
		s.Hourly = buckets(qsos, time.Hour)
	}

	s.Operators = sortedCounts(ops, 0)
	s.Stations = sortedCounts(stations, 0)
	s.TopCountries = sortedCounts(countries, 12)

	for i := len(qsos) - 1; i >= 0 && len(s.Recent) < 25; i-- {
		q := qsos[i]
		l := locate(q)
		r := RecentQSO{Time: q.Time, Call: q.Call, Band: q.Band, Mode: q.Mode, Operator: q.Operator,
			Station: q.Station, Exchange: q.Exchange, Country: l.country, Run: q.Run, Mult: q.Mult}
		if haveHome && l.ok {
			r.Dist = distanceKm(homeLat, homeLon, l.lat, l.lon)
		}
		s.Recent = append(s.Recent, r)
	}
	return s
}

// slotMinutes picks a chart time slot that suits the contest's length.
func slotMinutes(qsos []*QSO) int {
	span := qsos[len(qsos)-1].Time.Sub(qsos[0].Time)
	switch {
	case span > 26*time.Hour:
		return 60
	case span > 8*time.Hour:
		return 30
	}
	return 10
}

func buckets(qsos []*QSO, size time.Duration) []Bucket {
	start := qsos[0].Time.Truncate(size)
	end := qsos[len(qsos)-1].Time
	n := int(end.Sub(start)/size) + 1
	if n > 2000 {
		n = 2000
	}
	out := make([]Bucket, n)
	for i := range out {
		out[i] = Bucket{Start: start.Add(time.Duration(i) * size), ByBand: map[string]int{}}
	}
	for _, q := range qsos {
		i := int(q.Time.Sub(start) / size)
		if i >= 0 && i < n {
			out[i].ByBand[q.Band]++
			out[i].Total++
		}
	}
	return out
}

func sortedCounts(m map[string]*Count, limit int) []Count {
	out := []Count{}
	for _, c := range m {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].QSOs != out[j].QSOs {
			return out[i].QSOs > out[j].QSOs
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func BuildMap(snap Snapshot, cfg Config) *MapData {
	md := &MapData{Version: snap.Version, Points: []MapPoint{}}
	active := pickContest(listContests(snap.QSOs), cfg.Contest)
	if active == nil {
		if lat, lon, ok := homeLocation(cfg.MyGrid, cfg.MyCall); ok {
			md.Home = &[2]float64{lat, lon}
		}
		return md
	}
	qsos := filterContest(snap.QSOs, active.Key)
	mycall := cfg.MyCall
	if mycall == "" {
		mycall = qsos[len(qsos)-1].MyCall
	}
	if lat, lon, ok := homeLocation(cfg.MyGrid, mycall); ok {
		md.Home = &[2]float64{lat, lon}
	}
	for _, q := range qsos {
		l := locate(q)
		if !l.ok {
			continue
		}
		md.Points = append(md.Points, MapPoint{Lat: l.lat, Lon: l.lon, Call: q.Call, Band: q.Band, Mode: q.Mode, Time: q.Time.Unix(), Where: l.where})
	}
	return md
}
