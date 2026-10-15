package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// QSO is one logged contact, in the shape the dashboard needs.
type QSO struct {
	ID        string    `json:"id"`
	Contest   string    `json:"contest"`
	ContestNr string    `json:"contest_nr,omitempty"`
	Time      time.Time `json:"time"`
	MyCall    string    `json:"mycall,omitempty"`
	Call      string    `json:"call"`
	Band      string    `json:"band"`
	FreqKHz   float64   `json:"freq_khz,omitempty"`
	Mode      string    `json:"mode"` // CW, PH or DG
	RawMode   string    `json:"raw_mode,omitempty"`
	Operator  string    `json:"op,omitempty"`
	Station   string    `json:"station,omitempty"`
	RadioNr   int       `json:"radio,omitempty"`
	Run       bool      `json:"run,omitempty"`
	Points    int       `json:"points,omitempty"`
	Mult      bool      `json:"mult,omitempty"`
	Prefix    string    `json:"wpx,omitempty"`
	Grid      string    `json:"grid,omitempty"`
	Section   string    `json:"section,omitempty"`
	Exchange  string    `json:"exch,omitempty"`
	Name      string    `json:"name,omitempty"`
	Source    string    `json:"src,omitempty"` // n1mm, adif, demo
}

// RadioInfo is N1MM's live status for one radio on one logging computer.
type RadioInfo struct {
	Station   string    `json:"station"`
	RadioNr   int       `json:"radio"`
	Name      string    `json:"name,omitempty"`
	FreqKHz   float64   `json:"freq_khz"`
	TXFreqKHz float64   `json:"tx_freq_khz,omitempty"`
	Band      string    `json:"band"`
	Mode      string    `json:"mode"`
	Operator  string    `json:"op,omitempty"`
	Running   bool      `json:"running"`
	TX        bool      `json:"tx"`
	Connected bool      `json:"connected"`
	Antenna   string    `json:"antenna,omitempty"`
	Seen      time.Time `json:"seen"`
}

// Score is the logger's own score report (N1MM "dynamicresults").
type Score struct {
	Contest  string    `json:"contest"`
	Call     string    `json:"call"`
	Score    int64     `json:"score"`
	QSOs     int       `json:"qsos"`
	Points   int64     `json:"points"`
	Mults    int       `json:"mults"`
	Received time.Time `json:"received"`
}

// Packet is whatever one UDP datagram turned into.
type Packet struct {
	Kind     string // contact, replace, delete, radio, score
	QSO      *QSO
	DeleteID string
	Radio    *RadioInfo
	Score    *Score
}

// ParseN1MM reads one N1MM+ UDP broadcast. DXLog and TR4W send the same format.
func ParseN1MM(data []byte, now time.Time) (*Packet, error) {
	root, fields, err := flatXML(data)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(root) {
	case "contactinfo", "contactreplace":
		q := qsoFromFields(fields)
		if q.Call == "" {
			return nil, fmt.Errorf("contact without a callsign")
		}
		kind := "contact"
		if strings.EqualFold(root, "contactreplace") {
			kind = "replace"
		}
		return &Packet{Kind: kind, QSO: q}, nil
	case "contactdelete":
		id := fields["id"]
		if id == "" {
			id = fallbackID(fields["call"], fields["timestamp"], "")
		}
		return &Packet{Kind: "delete", DeleteID: id}, nil
	case "radioinfo":
		return &Packet{Kind: "radio", Radio: radioFromFields(fields, now)}, nil
	case "dynamicresults":
		s, err := parseScore(data)
		if err != nil {
			return nil, err
		}
		s.Received = now
		return &Packet{Kind: "score", Score: s}, nil
	}
	return nil, nil // spots, lookups, app info: not used yet
}

// flatXML returns the root element name and its direct children's text, keyed in lower case.
// N1MM's tag capitalisation isn't consistent, so this avoids case-sensitive struct tags.
func flatXML(data []byte) (string, map[string]string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	fields := map[string]string{}
	root := ""
	depth := 0
	var cur string
	var buf strings.Builder
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if root != "" {
				break // keep what we got from a truncated packet
			}
			return "", nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				root = t.Name.Local
			} else if depth == 2 {
				cur = strings.ToLower(t.Name.Local)
				buf.Reset()
			}
		case xml.CharData:
			if depth == 2 {
				buf.Write(t)
			}
		case xml.EndElement:
			if depth == 2 {
				fields[cur] = strings.TrimSpace(buf.String())
			}
			depth--
		}
	}
	if root == "" {
		return "", nil, fmt.Errorf("not XML")
	}
	return root, fields, nil
}

func qsoFromFields(f map[string]string) *QSO {
	q := &QSO{
		Contest:   f["contestname"],
		ContestNr: f["contestnr"],
		MyCall:    strings.ToUpper(f["mycall"]),
		Call:      strings.ToUpper(f["call"]),
		RawMode:   strings.ToUpper(f["mode"]),
		Operator:  strings.ToUpper(f["operator"]),
		Station:   firstNonEmpty(f["stationname"], f["netbiosname"]),
		RadioNr:   atoi(f["radionr"]),
		Run:       isTrue(f["isrunqso"]),
		Points:    atoi(f["points"]),
		Mult:      isTrue(f["ismultiplier1"]) || isTrue(f["ismultiplier2"]) || isTrue(f["ismultiplier3"]),
		Prefix:    f["wpxprefix"],
		Grid:      f["gridsquare"],
		Section:   firstNonEmpty(f["section"], f["prec"]),
		Exchange:  firstNonEmpty(f["exchange1"], f["rcv"]),
		Name:      f["name"],
		Source:    "n1mm",
	}
	if q.Operator == "" {
		q.Operator = q.MyCall
	}
	q.Mode = modeGroup(q.RawMode)
	q.Time = parseN1MMTime(f["timestamp"])
	// N1MM sends frequencies in tens of hertz.
	if hz10 := atof(f["rxfreq"]); hz10 > 0 {
		q.FreqKHz = hz10 / 100
	}
	q.Band = bandFromN1MM(f["band"])
	if q.Band == "" {
		q.Band = bandFromKHz(q.FreqKHz)
	}
	q.ID = f["id"]
	if q.ID == "" {
		q.ID = fallbackID(q.Call, f["timestamp"], q.Band)
	}
	return q
}

func radioFromFields(f map[string]string, now time.Time) *RadioInfo {
	r := &RadioInfo{
		Station:   firstNonEmpty(f["stationname"], f["netbiosname"]),
		RadioNr:   atoi(f["radionr"]),
		Name:      f["radioname"],
		FreqKHz:   atof(f["freq"]) / 100,
		TXFreqKHz: atof(f["txfreq"]) / 100,
		Mode:      strings.ToUpper(f["mode"]),
		Operator:  strings.ToUpper(f["opcall"]),
		Running:   isTrue(f["isrunning"]),
		TX:        isTrue(f["istransmitting"]),
		Connected: f["isconnected"] == "" || isTrue(f["isconnected"]),
		Antenna:   firstNonEmpty(f["auxantselectedname"], f["antenna"]),
		Seen:      now,
	}
	r.Band = bandFromKHz(r.FreqKHz)
	return r
}

type xmlCount struct {
	Band  string `xml:"band,attr"`
	Mode  string `xml:"mode,attr"`
	Value string `xml:",chardata"`
}

// parseScore reads the logger's score report. The headline <score> is the logger's own
// calculation and always wins; the breakdown is only used for QSO, point and mult totals.
func parseScore(data []byte) (*Score, error) {
	var x struct {
		Contest   string `xml:"contest"`
		Call      string `xml:"call"`
		Score     string `xml:"score"`
		Breakdown struct {
			QSO   []xmlCount `xml:"qso"`
			Point []xmlCount `xml:"point"`
			Mult  []xmlCount `xml:"mult"`
		} `xml:"breakdown"`
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := d.Decode(&x); err != nil {
		return nil, err
	}
	return &Score{
		Contest: x.Contest,
		Call:    strings.ToUpper(x.Call),
		Score:   int64(atof(x.Score)),
		QSOs:    int(totalOf(x.Breakdown.QSO)),
		Points:  int64(totalOf(x.Breakdown.Point)),
		Mults:   int(totalOf(x.Breakdown.Mult)),
	}, nil
}

// totalOf adds up a breakdown. Contests lay these out differently: some send a
// band="total" mode="ALL" row, some only per-band or per-mode rows, and mults can
// come as several rows (zones, countries...) that all need adding. Prefer the most
// summarised rows that exist so nothing is counted twice.
func totalOf(rows []xmlCount) float64 {
	isTotal := func(s string) bool { s = strings.ToLower(s); return s == "total" || s == "all" || s == "" }
	sum := func(keep func(xmlCount) bool) (float64, bool) {
		var t float64
		found := false
		for _, r := range rows {
			if keep(r) {
				t += atof(r.Value)
				found = true
			}
		}
		return t, found
	}
	if t, ok := sum(func(r xmlCount) bool { return isTotal(r.Band) && isTotal(r.Mode) }); ok {
		return t
	}
	if t, ok := sum(func(r xmlCount) bool { return isTotal(r.Band) }); ok {
		return t
	}
	if t, ok := sum(func(r xmlCount) bool { return isTotal(r.Mode) }); ok {
		return t
	}
	t, _ := sum(func(xmlCount) bool { return true })
	return t
}

func parseN1MMTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "1/2/2006 3:04:05 PM"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(s), time.UTC); err == nil {
			return t
		}
	}
	return time.Now().UTC().Truncate(time.Second)
}

func fallbackID(call, ts, band string) string {
	return strings.ToUpper(call) + "|" + ts + "|" + band
}

var n1mmBands = map[string]string{
	"1.8": "160m", "3.5": "80m", "5": "60m", "7": "40m", "10": "30m", "14": "20m",
	"18": "17m", "21": "15m", "24": "12m", "28": "10m", "50": "6m", "70": "4m",
	"144": "2m", "222": "1.25m", "420": "70cm", "432": "70cm", "902": "33cm", "1240": "23cm", "1296": "23cm",
}

func bandFromN1MM(s string) string {
	s = strings.TrimSpace(s)
	if b, ok := n1mmBands[s]; ok {
		return b
	}
	if v := atof(s); v > 0 {
		return bandFromKHz(v * 1000)
	}
	return ""
}

var bandEdges = []struct {
	lo, hi float64
	name   string
}{
	{1800, 2000, "160m"}, {3500, 4000, "80m"}, {5000, 5500, "60m"}, {7000, 7300, "40m"},
	{10100, 10150, "30m"}, {14000, 14350, "20m"}, {18068, 18168, "17m"}, {21000, 21450, "15m"},
	{24890, 24990, "12m"}, {28000, 29700, "10m"}, {50000, 54000, "6m"}, {70000, 71000, "4m"},
	{144000, 148000, "2m"}, {222000, 225000, "1.25m"}, {420000, 450000, "70cm"},
	{902000, 928000, "33cm"}, {1240000, 1300000, "23cm"},
}

func bandFromKHz(k float64) string {
	for _, b := range bandEdges {
		if k >= b.lo && k <= b.hi {
			return b.name
		}
	}
	return ""
}

// modeGroup folds logger modes into the three contest groups.
func modeGroup(m string) string {
	switch strings.ToUpper(m) {
	case "CW", "CW-R":
		return "CW"
	case "SSB", "USB", "LSB", "AM", "FM", "PH", "PHONE":
		return "PH"
	case "":
		return ""
	}
	return "DG"
}

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "y":
		return true
	}
	return false
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
