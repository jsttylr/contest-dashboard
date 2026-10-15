package main

import (
	_ "embed"
	"encoding/json"
	"hash/fnv"
	"math"
	"regexp"
	"strings"
)

// Country data comes from AD1C's country files (cty.dat), via the MIT-licensed
// @ham2k/lib-country-files package. See data/LICENSE-country-files.md.
//
//go:embed data/cty.json
var ctyJSON []byte

type Entity struct {
	Prefix    string
	Name      string
	Continent string
	CQZone    int
	Lat, Lon  float64
}

type ctyMatch struct {
	ent    int
	cqZone int
}

type Cty struct {
	ents   []Entity
	prefix map[string]ctyMatch
	exact  map[string]ctyMatch
	maxLen int
}

var cty = mustLoadCty()

func mustLoadCty() *Cty {
	var raw struct {
		E [][]any          `json:"e"`
		P map[string][]int `json:"p"`
		X map[string][]int `json:"x"`
	}
	if err := json.Unmarshal(ctyJSON, &raw); err != nil {
		panic(err)
	}
	c := &Cty{prefix: map[string]ctyMatch{}, exact: map[string]ctyMatch{}}
	for _, e := range raw.E {
		c.ents = append(c.ents, Entity{
			Prefix: e[0].(string), Name: e[1].(string), Continent: e[2].(string),
			CQZone: int(e[3].(float64)), Lat: e[4].(float64), Lon: e[5].(float64),
		})
	}
	conv := func(in map[string][]int, out map[string]ctyMatch) {
		for k, v := range in {
			m := ctyMatch{ent: v[0]}
			if len(v) > 1 {
				m.cqZone = v[1]
			}
			out[k] = m
			if len(k) > c.maxLen {
				c.maxLen = len(k)
			}
		}
	}
	conv(raw.P, c.prefix)
	conv(raw.X, c.exact)
	return c
}

// Lookup finds the DXCC entity for a callsign, longest prefix first.
func (c *Cty) Lookup(call string) (Entity, bool) {
	call = strings.ToUpper(strings.TrimSpace(call))
	if call == "" {
		return Entity{}, false
	}
	if m, ok := c.exact[call]; ok {
		return c.entity(m), true
	}
	base := lookupBase(call)
	for n := min(len(base), c.maxLen); n > 0; n-- {
		if m, ok := c.prefix[base[:n]]; ok {
			return c.entity(m), true
		}
	}
	return Entity{}, false
}

func (c *Cty) entity(m ctyMatch) Entity {
	e := c.ents[m.ent]
	if m.cqZone > 0 {
		e.CQZone = m.cqZone
	}
	return e
}

// lookupBase picks the part of a portable call that decides the country:
// "VP2V/W1AW" -> "VP2V", "W1AW/P" -> "W1AW", "W1AW/KH6" -> "KH6".
func lookupBase(call string) string {
	parts := strings.Split(call, "/")
	if len(parts) == 1 {
		return call
	}
	best := ""
	for _, p := range parts {
		switch p {
		case "", "P", "M", "MM", "AM", "QRP", "A", "R", "B", "LH":
			continue
		}
		if len(p) == 1 && p[0] >= '0' && p[0] <= '9' {
			continue
		}
		if best == "" || len(p) < len(best) {
			best = p
		}
	}
	if best == "" {
		return parts[0]
	}
	return best
}

var gridRE = regexp.MustCompile(`^[A-Ra-r]{2}[0-9]{2}([A-Xa-x]{2})?$`)

// GridToLatLon returns the centre of a 4 or 6 character Maidenhead locator.
func GridToLatLon(grid string) (lat, lon float64, ok bool) {
	g := strings.TrimSpace(grid)
	if len(g) > 6 {
		g = g[:6]
	}
	if !gridRE.MatchString(g) {
		return 0, 0, false
	}
	g = strings.ToUpper(g[:2]) + g[2:4] + strings.ToLower(g[4:])
	lon = float64(g[0]-'A')*20 - 180
	lat = float64(g[1]-'A')*10 - 90
	lon += float64(g[2]-'0') * 2
	lat += float64(g[3] - '0')
	if len(g) == 6 {
		lon += float64(g[4]-'a') * (2.0 / 24)
		lat += float64(g[5]-'a') * (1.0 / 24)
		return lat + 0.5/24, lon + 1.0/24, true
	}
	return lat + 0.5, lon + 1, true
}

// Rough centres of US states, Canadian provinces and ARRL/RAC sections, so
// domestic contests put contacts in the right part of the map.
var regionLatLon = map[string][2]float64{
	// US states
	"AL": {32.8, -86.8}, "AK": {61.4, -150.0}, "AZ": {34.3, -111.7}, "AR": {34.9, -92.4},
	"CA": {37.2, -119.5}, "CO": {39.0, -105.5}, "CT": {41.6, -72.7}, "DE": {39.0, -75.5},
	"DC": {38.9, -77.0}, "FL": {28.6, -82.4}, "GA": {32.7, -83.4}, "HI": {20.8, -156.3},
	"ID": {44.4, -114.6}, "IL": {40.0, -89.2}, "IN": {39.9, -86.3}, "IA": {42.1, -93.5},
	"KS": {38.5, -98.4}, "KY": {37.5, -85.3}, "LA": {31.1, -92.0}, "ME": {45.4, -69.2},
	"MD": {39.0, -76.8}, "MA": {42.3, -71.8}, "MI": {44.3, -85.4}, "MN": {46.3, -94.3},
	"MS": {32.7, -89.7}, "MO": {38.4, -92.5}, "MT": {47.0, -109.6}, "NE": {41.5, -99.8},
	"NV": {39.3, -116.6}, "NH": {43.7, -71.6}, "NJ": {40.2, -74.7}, "NM": {34.4, -106.1},
	"NY": {42.9, -75.5}, "NC": {35.6, -79.4}, "ND": {47.5, -100.5}, "OH": {40.3, -82.8},
	"OK": {35.6, -97.5}, "OR": {43.9, -120.6}, "PA": {40.9, -77.8}, "RI": {41.7, -71.5},
	"SC": {33.9, -80.9}, "SD": {44.4, -100.2}, "TN": {35.9, -86.4}, "TX": {31.5, -99.3},
	"UT": {39.3, -111.7}, "VT": {44.1, -72.7}, "VA": {37.5, -78.9}, "WA": {47.4, -120.5},
	"WV": {38.6, -80.6}, "WI": {44.6, -89.9}, "WY": {43.0, -107.6}, "PR": {18.2, -66.5},
	"VI": {18.3, -64.8},
	// Canadian provinces and RAC sections
	"NS": {45.0, -63.0}, "NB": {46.5, -66.2}, "PE": {46.3, -63.2}, "NL": {48.8, -56.0},
	"QC": {47.0, -71.5}, "ON": {44.5, -79.5}, "MB": {50.5, -98.0}, "SK": {52.0, -106.0},
	"AB": {53.5, -114.5}, "BC": {50.5, -123.0}, "NT": {62.5, -114.4}, "NU": {63.7, -68.5},
	"YT": {60.7, -135.0}, "MAR": {45.5, -64.5}, "ONE": {45.0, -76.0}, "ONN": {48.5, -84.0},
	"ONS": {43.0, -81.0}, "GTA": {43.7, -79.4}, "GH": {44.0, -79.0}, "TER": {62.5, -114.4},
	// ARRL sections that aren't a whole state
	"EMA": {42.2, -71.2}, "WMA": {42.3, -72.6}, "ENY": {42.3, -74.2}, "NLI": {40.8, -73.3},
	"NNJ": {40.8, -74.4}, "NNY": {44.2, -75.0}, "SNJ": {39.7, -74.9}, "WNY": {42.8, -77.6},
	"EPA": {40.5, -75.9}, "WPA": {40.8, -79.4}, "MDC": {39.0, -76.8}, "NFL": {30.2, -82.5},
	"SFL": {26.3, -80.6}, "WCF": {27.9, -82.2}, "NTX": {32.8, -97.0}, "STX": {29.4, -97.7},
	"WTX": {31.8, -102.4}, "EB": {37.8, -122.1}, "LAX": {34.1, -118.2}, "ORG": {33.7, -117.8},
	"SB": {34.6, -120.0}, "SCV": {37.2, -121.9}, "SDG": {32.9, -116.9}, "SF": {38.0, -122.6},
	"SJV": {36.6, -119.6}, "SV": {39.2, -121.4}, "PAC": {21.3, -157.8}, "EWA": {47.4, -118.5},
	"WWA": {47.5, -122.3},
}

func RegionLatLon(s string) (lat, lon float64, ok bool) {
	p, ok := regionLatLon[strings.ToUpper(strings.TrimSpace(s))]
	return p[0], p[1], ok
}

// jitter spreads contacts that share a country or state centre so they don't sit on one dot.
func jitter(call string, spread float64) (dlat, dlon float64) {
	h := fnv.New32a()
	h.Write([]byte(call))
	v := h.Sum32()
	a := float64(v&0xffff) / 65535 * 2 * math.Pi
	r := math.Sqrt(float64(v>>16)/65535) * spread
	return r * math.Sin(a), r * math.Cos(a)
}

// Distance in km along the great circle.
func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp := p2 - p1
	dl := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// homeLocation is the station's own position: the configured grid, or failing that
// the centre of the station callsign's country.
func homeLocation(grid, mycall string) (lat, lon float64, ok bool) {
	if lat, lon, ok := GridToLatLon(grid); ok {
		return lat, lon, true
	}
	if e, ok := cty.Lookup(mycall); ok {
		return e.Lat, e.Lon, true
	}
	return 0, 0, false
}
