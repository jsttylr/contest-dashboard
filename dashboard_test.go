package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// A real-shaped N1MM+ contactinfo packet.
const sampleContact = `<?xml version="1.0" encoding="utf-8"?>
<contactinfo>
	<app>N1MM</app>
	<contestname>CWOPS</contestname>
	<contestnr>73</contestnr>
	<timestamp>2020-01-17 16:43:38</timestamp>
	<mycall>W2XYZ</mycall>
	<band>3.5</band>
	<rxfreq>352519</rxfreq>
	<txfreq>352519</txfreq>
	<operator></operator>
	<mode>CW</mode>
	<call>W1AW</call>
	<countryprefix>K</countryprefix>
	<wpxprefix>W1</wpxprefix>
	<stationprefix>W2XYZ</stationprefix>
	<continent>NA</continent>
	<snt>599</snt>
	<sntnr>5</sntnr>
	<rcv>599</rcv>
	<rcvnr>0</rcvnr>
	<gridsquare></gridsquare>
	<exchange1></exchange1>
	<section>CT</section>
	<comment></comment>
	<qth></qth>
	<name>HIRAM</name>
	<power></power>
	<misctext></misctext>
	<zone>0</zone>
	<prec></prec>
	<ck>0</ck>
	<ismultiplier1>1</ismultiplier1>
	<ismultiplier2>0</ismultiplier2>
	<ismultiplier3>0</ismultiplier3>
	<points>1</points>
	<radionr>1</radionr>
	<run1run2>1</run1run2>
	<RoverLocation></RoverLocation>
	<RadioInterfaced>1</RadioInterfaced>
	<NetworkedCompNr>0</NetworkedCompNr>
	<IsOriginal>True</IsOriginal>
	<NetBiosName>DESKTOP-ABC</NetBiosName>
	<IsRunQSO>0</IsRunQSO>
	<StationName>CONTEST-PC</StationName>
	<ID>f9ffac4fcd3e479ca86e137df1338531</ID>
	<IsClaimedQso>True</IsClaimedQso>
</contactinfo>`

func TestParseContact(t *testing.T) {
	p, err := ParseN1MM([]byte(sampleContact), time.Now())
	if err != nil || p == nil || p.Kind != "contact" {
		t.Fatalf("got %+v, %v", p, err)
	}
	q := p.QSO
	if q.Call != "W1AW" || q.Band != "80m" || q.Mode != "CW" || q.FreqKHz != 3525.19 {
		t.Errorf("call/band/mode/freq wrong: %+v", q)
	}
	if q.Operator != "W2XYZ" || q.Station != "CONTEST-PC" || !q.Mult || q.Points != 1 || q.Run {
		t.Errorf("op/station/mult/points/run wrong: %+v", q)
	}
	if q.ID != "f9ffac4fcd3e479ca86e137df1338531" || contestKey(q) != "CWOPS #73" {
		t.Errorf("id/contest wrong: %+v", q)
	}
	if !q.Time.Equal(time.Date(2020, 1, 17, 16, 43, 38, 0, time.UTC)) {
		t.Errorf("time %v", q.Time)
	}
	l := locate(q)
	if !l.ok || l.where != "CT" || l.country != "United States" {
		t.Errorf("location %+v", l)
	}
}

func TestParseDelete(t *testing.T) {
	p, _ := ParseN1MM([]byte(`<?xml version="1.0" encoding="utf-8"?><contactdelete><app>N1MM</app><timestamp>2020-01-17 16:43:38</timestamp><call>W1AW</call><contestnr>73</contestnr><StationName>CONTEST-PC</StationName><ID>f9ffac4fcd3e479ca86e137df1338531</ID></contactdelete>`), time.Now())
	if p.Kind != "delete" || p.DeleteID != "f9ffac4fcd3e479ca86e137df1338531" {
		t.Fatalf("%+v", p)
	}
}

func TestParseRadio(t *testing.T) {
	p, err := ParseN1MM([]byte(`<?xml version="1.0" encoding="utf-8"?>
<RadioInfo><app>N1MM</app><StationName>CW-STATION</StationName><RadioNr>2</RadioNr><Freq>1402500</Freq><TXFreq>1402500</TXFreq><Mode>CW</Mode><OpCall>N1KDO</OpCall><IsRunning>True</IsRunning><FocusEntry>00000</FocusEntry><EntryWindowHwnd>0</EntryWindowHwnd><Antenna>2</Antenna><Rotors></Rotors><FocusRadioNr>1</FocusRadioNr><IsStereo>False</IsStereo><IsSplit>False</IsSplit><ActiveRadioNr>1</ActiveRadioNr><IsTransmitting>False</IsTransmitting><FunctionKeyCaption></FunctionKeyCaption><RadioName>K3</RadioName><AuxAntSelected>-1</AuxAntSelected><AuxAntSelectedName></AuxAntSelectedName><IsConnected>True</IsConnected></RadioInfo>`), time.Now())
	if err != nil || p.Kind != "radio" {
		t.Fatal(p, err)
	}
	r := p.Radio
	if r.FreqKHz != 14025 || r.Band != "20m" || r.RadioNr != 2 || !r.Running || r.Operator != "N1KDO" {
		t.Errorf("%+v", r)
	}
}

func TestScoreTotals(t *testing.T) {
	cases := []struct {
		name, xml   string
		score       int64
		qsos, mults int
	}{
		{"total row", `<dynamicresults><contest>CQ-WW-CW</contest><call>W1AW</call><breakdown>
			<qso band="40" mode="CW">10</qso><qso band="total" mode="ALL">25</qso>
			<mult band="total" mode="ALL" type="zone">20</mult><mult band="total" mode="ALL" type="country">30</mult>
			<point band="total" mode="ALL">70</point></breakdown><score>3500</score></dynamicresults>`, 3500, 25, 50},
		{"only per band rows", `<dynamicresults><contest>NAQP-SSB</contest><call>W1AW</call><breakdown>
			<qso band="20" mode="PH">10</qso><qso band="40" mode="PH">5</qso>
			<mult band="20" mode="PH">7</mult><mult band="40" mode="PH">3</mult></breakdown><score>150</score></dynamicresults>`, 150, 15, 10},
		{"no mults at all", `<dynamicresults><contest>SPRINT</contest><call>W1AW</call><breakdown>
			<qso band="total" mode="ALL">42</qso></breakdown><score>42</score></dynamicresults>`, 42, 42, 0},
	}
	for _, c := range cases {
		p, err := ParseN1MM([]byte(c.xml), time.Now())
		if err != nil || p.Kind != "score" {
			t.Fatalf("%s: %v", c.name, err)
		}
		if p.Score.Score != c.score || p.Score.QSOs != c.qsos || p.Score.Mults != c.mults {
			t.Errorf("%s: got %+v", c.name, p.Score)
		}
	}
}

func TestCtyAndGrid(t *testing.T) {
	for call, want := range map[string]string{
		"W1AW": "United States", "DL1ABC": "Fed. Rep. of Germany", "JA1XYZ": "Japan",
		"VP2V/W1AW": "British Virgin Islands", "W1AW/P": "United States", "KH6/W1AW": "Hawaii",
	} {
		e, ok := cty.Lookup(call)
		if !ok || e.Name != want {
			t.Errorf("%s: got %q want %q", call, e.Name, want)
		}
	}
	lat, lon, ok := GridToLatLon("FN31pr")
	if !ok || lat < 41.7 || lat > 41.8 || lon < -72.75 || lon > -72.6 {
		t.Errorf("FN31pr -> %v %v", lat, lon)
	}
}

func TestStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contacts.jsonl")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ParseN1MM([]byte(sampleContact), time.Now())
	s.Handle(p)
	p2, _ := ParseN1MM([]byte(sampleContact), time.Now()) // same ID again: replaced, not doubled
	s.Handle(p2)
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Snapshot().QSOs); n != 1 {
		t.Fatalf("want 1 contact after restart, got %d", n)
	}
	s.Handle(&Packet{Kind: "delete", DeleteID: p.QSO.ID})
	s.Close()
	s, _ = OpenStore(path)
	if n := len(s.Snapshot().QSOs); n != 0 {
		t.Fatalf("delete didn't stick, %d left", n)
	}
}

func TestADIF(t *testing.T) {
	log := `Exported by N1MM <EOH>
<CALL:4>W1AW <QSO_DATE:8>20251129 <TIME_ON:6>120102 <BAND:3>20m <FREQ:9>14.025000 <MODE:2>CW <CONTEST_ID:8>CQ-WW-CW <OPERATOR:5>W2XYZ <EOR>
<CALL:6>DL1ABC <QSO_DATE:8>20251129 <TIME_ON:4>1203 <BAND:3>20M <MODE:3>SSB <EOR>`
	qs, err := ParseADIF(log, "mylog")
	if err != nil || len(qs) != 2 {
		t.Fatal(qs, err)
	}
	if qs[0].Contest != "CQ-WW-CW" || qs[0].FreqKHz != 14025 || qs[0].Mode != "CW" || qs[1].Contest != "mylog" || qs[1].Band != "20m" || qs[1].Mode != "PH" {
		t.Errorf("%+v %+v", qs[0], qs[1])
	}
}

func TestSummary(t *testing.T) {
	s, _ := OpenStore("")
	base := time.Date(2025, 11, 29, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 90; i++ {
		s.Handle(&Packet{Kind: "contact", QSO: &QSO{ID: itoa(i), Contest: "TEST", Call: "DL1A" + itoa(i), Band: "20m", Mode: "CW",
			Operator: "OP" + itoa(i%2), Time: base.Add(time.Duration(i) * time.Minute), Points: 3, Mult: i%10 == 0}})
	}
	now := base.Add(90 * time.Minute)
	sum := BuildSummary(s.Snapshot(), Config{}, now)
	if sum.Totals.QSOs != 90 || sum.Totals.Mults != 9 || sum.Totals.Score != 270*9 || sum.Totals.ScoreSource != "estimate" {
		t.Errorf("totals %+v", sum.Totals)
	}
	if sum.Rates.Last60 != 60 || sum.Rates.Best60 != 60 || sum.Rates.Last10 != 10 {
		t.Errorf("rates %+v", sum.Rates)
	}
	if len(sum.Operators) != 2 || sum.Operators[0].QSOs != 45 {
		t.Errorf("operators %+v", sum.Operators)
	}
	// A score report from a different day belongs to another contest.
	s.Handle(&Packet{Kind: "score", Score: &Score{Score: 999, Received: now.Add(48 * time.Hour)}})
	if sum = BuildSummary(s.Snapshot(), Config{}, now); sum.Totals.ScoreSource != "estimate" {
		t.Errorf("score from another day was used: %+v", sum.Totals)
	}
	// The logger's own score wins over the estimate.
	s.Handle(&Packet{Kind: "score", Score: &Score{Score: 12345, Mults: 11, Received: now}})
	sum = BuildSummary(s.Snapshot(), Config{}, now)
	if sum.Totals.Score != 12345 || sum.Totals.Mults != 11 || sum.Totals.ScoreSource != "logger" {
		t.Errorf("logger score not used: %+v", sum.Totals)
	}
}

func TestOperatorRates(t *testing.T) {
	base := time.Date(2025, 11, 29, 12, 0, 0, 0, time.UTC)
	mk := func(min int, run bool) *QSO {
		return &QSO{Call: "DL1A", Time: base.Add(time.Duration(min) * time.Minute), Run: run, Operator: "W1ABC"}
	}
	// Running for 4 minutes, searching for 4, then an hour's break before one more run contact.
	qs := []*QSO{mk(0, true), mk(2, true), mk(4, true), mk(6, false), mk(8, false), mk(68, true)}
	st := computeOpStats("W1ABC", qs, 12, base.Add(70*time.Minute))
	if st.RunQSOs != 4 || st.SPQSOs != 2 {
		t.Errorf("run/sp counts %d/%d", st.RunQSOs, st.SPQSOs)
	}
	if int(st.RunRate+0.5) != 60 || int(st.SPRate+0.5) != 30 || int(st.AvgRate+0.5) != 45 {
		t.Errorf("rates run %.1f sp %.1f avg %.1f", st.RunRate, st.SPRate, st.AvgRate)
	}
	if st.Best10 != 5 || st.Best60 != 5 || st.Share != 50 || st.Last60 != 1 {
		t.Errorf("best10 %d best60 %d share %.0f last60 %d", st.Best10, st.Best60, st.Share, st.Last60)
	}
}

func TestTileCache(t *testing.T) {
	hits := 0
	png := []byte("\x89PNG\r\n\x1a\nfake")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	}))
	defer up.Close()
	tileSources["test"] = up.URL + "/{z}/{x}/{y}.png"
	defer delete(tileSources, "test")
	tc := NewTileCache(t.TempDir())
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		tc.ServeHTTP(rec, httptest.NewRequest("GET", "/tiles/test/2/1/3", nil))
		if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), png) {
			t.Fatalf("got %d %q", rec.Code, rec.Body.String())
		}
	}
	if hits != 1 {
		t.Errorf("second request should come from the disk cache, upstream hit %d times", hits)
	}
	for _, bad := range []string{"/tiles/test/2/4/0", "/tiles/test/9/0/0", "/tiles/nope/1/0/0", "/tiles/test/1/../0"} {
		rec := httptest.NewRecorder()
		tc.ServeHTTP(rec, httptest.NewRequest("GET", bad, nil))
		if rec.Code != 404 {
			t.Errorf("%s: want 404, got %d", bad, rec.Code)
		}
	}
}

func TestMaxRatesAndStretches(t *testing.T) {
	base := time.Date(2025, 11, 29, 12, 0, 0, 0, time.UTC)
	var qs []*QSO
	add := func(sec int, run bool, band string) {
		qs = append(qs, &QSO{Call: "DL1A", Band: band, Run: run, Time: base.Add(time.Duration(sec) * time.Second)})
	}
	// 30 run contacts on 20m, one a minute (60/hour), with an S&P contact on the
	// other radio in the middle that must not break the run.
	for i := 0; i < 30; i++ {
		add(i*60, true, "20m")
		if i == 10 {
			add(i*60+30, false, "40m")
		}
	}
	// A 20-minute pause, then 25 fast run contacts on 40m, 30 seconds apart (120/hour).
	for i := 0; i < 25; i++ {
		add(29*60+20*60+i*30, true, "40m")
	}
	st := runStretches(qs)
	if len(st) != 2 || st[0].QSOs != 30 || st[1].QSOs != 25 || st[1].Band != "40m" {
		t.Fatalf("stretches %+v", st)
	}
	if int(st[0].Rate+0.5) != 60 || int(st[1].Rate+0.5) != 120 {
		t.Errorf("stretch rates %.1f %.1f", st[0].Rate, st[1].Rate)
	}
	m := computeMaxRates(qs)
	if m.LongestRun.QSOs != 30 || m.FastestRun.QSOs != 25 || int(m.Best10Q+0.5) != 120 {
		t.Errorf("max rates %+v longest %+v fastest %+v", m, m.LongestRun, m.FastestRun)
	}
	pts := ratePoints(qs)
	if pts[1][2] != 60 || pts[0][1] != 1 || pts[11][1] != 0 {
		t.Errorf("points %v", pts[:12])
	}
}
