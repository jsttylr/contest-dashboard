package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseADIF reads an ADIF log (for example N1MM's File > Export to ADIF) so a past
// contest can be looked at on the dashboard. fallbackContest names logs without CONTEST_ID.
func ParseADIF(data string, fallbackContest string) ([]*QSO, error) {
	// Skip the header, if any.
	if i := strings.Index(strings.ToLower(data), "<eoh>"); i >= 0 {
		data = data[i+5:]
	}
	var out []*QSO
	rec := map[string]string{}
	for {
		lt := strings.IndexByte(data, '<')
		if lt < 0 {
			break
		}
		gt := strings.IndexByte(data[lt:], '>')
		if gt < 0 {
			break
		}
		tag := data[lt+1 : lt+gt]
		data = data[lt+gt+1:]
		parts := strings.Split(tag, ":")
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "eor" {
			if q := qsoFromADIF(rec, fallbackContest); q != nil {
				out = append(out, q)
			}
			rec = map[string]string{}
			continue
		}
		if len(parts) < 2 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || n < 0 {
			continue
		}
		// ADIF lengths count bytes; most logs are plain ASCII anyway.
		if n > len(data) {
			n = len(data)
		}
		rec[name] = data[:n]
		data = data[n:]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no contacts found; is this an ADIF (.adi) file?")
	}
	return out, nil
}

func qsoFromADIF(r map[string]string, fallbackContest string) *QSO {
	call := strings.ToUpper(strings.TrimSpace(r["call"]))
	if call == "" {
		return nil
	}
	t, err := time.ParseInLocation("20060102 150405", r["qso_date"]+" "+padTime(r["time_on"]), time.UTC)
	if err != nil {
		return nil
	}
	q := &QSO{
		Contest:  firstNonEmpty(r["contest_id"], fallbackContest),
		Time:     t,
		MyCall:   strings.ToUpper(firstNonEmpty(r["station_callsign"], r["owner_callsign"])),
		Call:     call,
		RawMode:  strings.ToUpper(firstNonEmpty(r["submode"], r["mode"])),
		Operator: strings.ToUpper(firstNonEmpty(r["operator"], r["station_callsign"])),
		Station:  firstNonEmpty(r["app_n1mm_netbiosname"], r["my_rig"]),
		RadioNr:  atoi(r["app_n1mm_radio_nr"]),
		Run:      isTrue(r["app_n1mm_isrunqso"]),
		Points:   atoi(firstNonEmpty(r["app_n1mm_points"], r["points"])),
		Mult:     isTrue(r["app_n1mm_mult1"]) || isTrue(r["app_n1mm_mult2"]) || isTrue(r["app_n1mm_mult3"]),
		Prefix:   r["pfx"],
		Grid:     r["gridsquare"],
		Section:  firstNonEmpty(r["arrl_sect"], r["state"], r["ve_prov"]),
		Exchange: firstNonEmpty(r["srx_string"], r["srx"]),
		Name:     r["name"],
		Source:   "adif",
	}
	q.Mode = modeGroup(firstNonEmpty(r["mode"], r["submode"]))
	if f := atof(r["freq"]); f > 0 {
		q.FreqKHz = f * 1000
	}
	q.Band = strings.ToLower(strings.TrimSpace(r["band"]))
	if q.Band == "" {
		q.Band = bandFromKHz(q.FreqKHz)
	}
	q.ID = "adif|" + fallbackID(call, t.Format(time.RFC3339), q.Band)
	return q
}

func padTime(s string) string {
	s = strings.TrimSpace(s)
	for len(s) < 6 {
		s += "0"
	}
	return s[:6]
}
