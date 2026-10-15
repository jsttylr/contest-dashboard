package main

import (
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
