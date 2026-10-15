package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const demoContest = "DEMO-CQ-WW-SSB"

// Demo makes up a contest and feeds it through the same packet reader as real
// N1MM broadcasts, so the dashboard can be tried without a logger running.
type Demo struct {
	mu      sync.Mutex
	stop    chan struct{}
	handler func([]byte)
}

func (d *Demo) Running() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stop != nil
}

func (d *Demo) Start(handler func([]byte)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stop != nil {
		return
	}
	d.stop = make(chan struct{})
	d.handler = handler
	go safely("demo", func() { runDemo(d.stop, handler) })
	logs.Add("demo", "demo contest started")
}

func (d *Demo) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stop != nil {
		close(d.stop)
		d.stop = nil
		logs.Add("demo", "demo contest stopped")
	}
}

var demoEntities = []struct {
	pfx    string
	weight int
}{
	{"K", 30}, {"W", 25}, {"N", 10}, {"VE", 6}, {"DL", 10}, {"G", 6}, {"F", 5}, {"I", 6}, {"EA", 6},
	{"ON", 3}, {"PA", 4}, {"OK", 4}, {"SP", 5}, {"UA", 6}, {"UA9", 3}, {"OH", 3}, {"SM", 3}, {"LY", 2},
	{"YO", 2}, {"HA", 3}, {"S5", 2}, {"9A", 2}, {"JA", 8}, {"BY", 2}, {"HL", 2}, {"VK", 3}, {"ZL", 2},
	{"PY", 4}, {"LU", 3}, {"CE", 1}, {"XE", 2}, {"KP4", 2}, {"ZS", 1}, {"CN", 1}, {"EA8", 2}, {"CT", 2},
	{"4X", 1}, {"A6", 1}, {"VU", 1}, {"HS", 1}, {"YB", 1}, {"KH6", 1}, {"KL7", 1}, {"TF", 1}, {"OX", 1},
}

var demoStates = []string{"MA", "NY", "PA", "OH", "TX", "CA", "FL", "WA", "CO", "IL", "GA", "MN", "AZ", "TN", "VA", "ON", "QC", "BC"}

func demoCall(r *rand.Rand) (call, state string) {
	total := 0
	for _, e := range demoEntities {
		total += e.weight
	}
	n := r.Intn(total)
	pfx := ""
	for _, e := range demoEntities {
		if n < e.weight {
			pfx = e.pfx
			break
		}
		n -= e.weight
	}
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	suf := ""
	for i := 0; i < 1+r.Intn(3); i++ {
		suf += string(letters[r.Intn(26)])
	}
	digit := fmt.Sprint(r.Intn(10))
	if strings.ContainsAny(pfx[len(pfx)-1:], "0123456789") {
		digit = ""
	}
	if pfx == "K" || pfx == "W" || pfx == "N" || pfx == "VE" {
		state = demoStates[r.Intn(len(demoStates))]
		if pfx == "VE" {
			state = demoStates[len(demoStates)-3+r.Intn(3)]
		}
	}
	return pfx + digit + suf, state
}

func demoContact(r *rand.Rand, nr int, t time.Time, station string, radio int, op string, band string, run bool) []byte {
	call, state := demoCall(r)
	ent, _ := cty.Lookup(call)
	freqs := map[string]int{"80m": 3750, "40m": 7150, "20m": 14200, "15m": 21250, "10m": 28450}
	mhz := map[string]string{"80m": "3.5", "40m": "7", "20m": "14", "15m": "21", "10m": "28"}
	f := (freqs[band] + r.Intn(100)) * 100
	mult := r.Intn(6) == 0
	pts := 3
	if ent.Continent == "NA" {
		pts = 2
	}
	if ent.Prefix == "K" {
		pts = 0
	}
	run1 := "False"
	if run {
		run1 = "True"
	}
	m := "False"
	if mult {
		m = "True"
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<contactinfo><app>DEMO</app><contestname>%s</contestname><contestnr>0</contestnr><timestamp>%s</timestamp><mycall>W1DEMO</mycall><band>%s</band><rxfreq>%d</rxfreq><txfreq>%d</txfreq><operator>%s</operator><mode>USB</mode><call>%s</call><continent>%s</continent><snt>59</snt><rcv>59</rcv><exchange1>%d</exchange1><section>%s</section><ismultiplier1>%s</ismultiplier1><points>%d</points><radionr>%d</radionr><IsRunQSO>%s</IsRunQSO><StationName>%s</StationName><ID>demo-%d</ID></contactinfo>`,
		demoContest, t.Format("2006-01-02 15:04:05"), mhz[band], f, f, op, call, ent.Continent, ent.CQZone, state, m, pts, radio, run1, station, nr))
}

func runDemo(stop chan struct{}, handle func([]byte)) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	type stn struct {
		name  string
		radio int
		op    string
		band  string
		run   bool
		next  time.Time
	}
	stations := []*stn{
		{name: "SHACK-PC1", radio: 1, op: "W1ABC", band: "20m", run: true},
		{name: "SHACK-PC2", radio: 2, op: "K1XYZ", band: "40m"},
	}
	// Some operators are quicker than others, so the comparison page has something to show.
	speed := map[string]float64{"W1ABC": 1.0, "K1XYZ": 1.5, "N1QQQ": 0.8, "KB1ZZ": 1.9}
	ops := []string{"W1ABC", "K1XYZ", "N1QQQ", "KB1ZZ"}
	bands := []string{"80m", "40m", "20m", "15m", "10m"}
	nr := int(time.Now().Unix()) * 100000 // unique IDs across demo runs
	totalQ, totalPts, mults := 0, 0, 0

	emit := func(s *stn, t time.Time) {
		nr++
		pkt := demoContact(r, nr, t, s.name, s.radio, s.op, s.band, s.run)
		handle(pkt)
		totalQ++
		if p, err := ParseN1MM(pkt, t); err == nil && p != nil {
			totalPts += p.QSO.Points
			if p.QSO.Mult {
				mults++
			}
		}
	}
	// gap is the time to the station's next contact: quick when running, slower searching,
	// and now and then a longer pause.
	gap := func(s *stn) time.Duration {
		secs := 15 + r.Float64()*25
		if !s.run {
			secs = 70 + r.Float64()*110
		}
		if r.Intn(40) == 0 {
			secs += 600 + r.Float64()*1500
		}
		// Everyone speeds up and slows down over the session (lunch, fatigue...), each on their own cycle.
		phase := float64(len(s.op)+int(s.op[1])) / 3
		slump := 1 + 0.45*math.Sin(2*math.Pi*float64(s.next.Unix())/(100*60)+phase)
		return time.Duration(secs * speed[s.op] * slump * float64(time.Second))
	}
	shuffle := func(s *stn) {
		if r.Intn(25) == 0 {
			s.band = bands[r.Intn(len(bands))]
		}
		if r.Intn(40) == 0 {
			s.run = !s.run
		}
		if r.Intn(25) == 0 {
			// Hand over to an operator who isn't already on the other radio.
			for {
				o := ops[r.Intn(len(ops))]
				if o != stations[0].op && o != stations[1].op {
					s.op = o
					break
				}
			}
		}
	}
	step := func(now time.Time) {
		for {
			s := stations[0]
			if stations[1].next.Before(s.next) {
				s = stations[1]
			}
			if s.next.After(now) {
				return
			}
			emit(s, s.next)
			shuffle(s)
			s.next = s.next.Add(gap(s))
		}
	}

	// Backfill three hours so the charts have something to show straight away.
	now := time.Now().UTC()
	for _, s := range stations {
		s.next = now.Add(-3 * time.Hour).Add(gap(s))
	}
	step(now)

	radioTick := time.NewTicker(2 * time.Second)
	scoreTick := time.NewTicker(20 * time.Second)
	contactTick := time.NewTicker(time.Second)
	defer radioTick.Stop()
	defer scoreTick.Stop()
	defer contactTick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-contactTick.C:
			step(time.Now().UTC())
		case <-radioTick.C:
			for _, s := range stations {
				f := map[string]int{"80m": 3750, "40m": 7150, "20m": 14200, "15m": 21250, "10m": 28450}[s.band]*100 + r.Intn(50)*10
				tx := "False"
				if r.Intn(3) == 0 {
					tx = "True"
				}
				running := "False"
				if s.run {
					running = "True"
				}
				handle([]byte(fmt.Sprintf(`<RadioInfo><app>DEMO</app><StationName>%s</StationName><RadioNr>%d</RadioNr><Freq>%d</Freq><TXFreq>%d</TXFreq><Mode>USB</Mode><OpCall>%s</OpCall><IsRunning>%s</IsRunning><IsTransmitting>%s</IsTransmitting><IsConnected>True</IsConnected><RadioName>Demo radio %d</RadioName><AuxAntSelectedName>Tribander</AuxAntSelectedName></RadioInfo>`,
					s.name, s.radio, f, f, s.op, running, tx, s.radio)))
			}
		case <-scoreTick.C:
			handle([]byte(fmt.Sprintf(`<?xml version="1.0"?><dynamicresults><contest>CQ-WW-SSB</contest><call>W1DEMO</call><breakdown><qso band="total" mode="ALL">%d</qso><point band="total" mode="ALL">%d</point><mult band="total" mode="ALL" type="zone">%d</mult><mult band="total" mode="ALL" type="country">%d</mult></breakdown><score>%d</score><timestamp>%s</timestamp></dynamicresults>`,
				totalQ, totalPts, mults/3, mults-mults/3, totalPts*mults, time.Now().UTC().Format("2006-01-02 15:04:05"))))
		}
	}
}
