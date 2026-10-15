package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFiles embed.FS

type App struct {
	cfg            *ConfigStore
	store          *Store
	listener       *Listener
	demo           *Demo
	tiles          *TileCache
	dataDir        string
	remoteSettings bool
	quit           chan struct{}
	quitOnce       sync.Once

	cacheMu sync.Mutex
	cache   map[string]cachedSummary
}

type cachedSummary struct {
	version int64
	built   time.Time
	cfg     Config
	sum     *Summary
}

// viewConfig is the saved settings, with the contest picked on the page (if any) on top.
func (a *App) viewConfig(req *http.Request) Config {
	c := a.cfg.Get()
	if k := req.URL.Query().Get("contest"); k != "" {
		c.Contest = k
	}
	return c
}

func (a *App) summary(c Config) *Summary {
	snapVersion, _ := a.store.Wait()
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if a.cache == nil {
		a.cache = map[string]cachedSummary{}
	}
	if cs, ok := a.cache[c.Contest]; ok && cs.version == snapVersion && cs.cfg.MyGrid == c.MyGrid &&
		cs.cfg.MyCall == c.MyCall && cs.cfg.Units == c.Units && time.Since(cs.built) < 5*time.Second {
		return cs.sum
	}
	s := BuildSummary(a.store.Snapshot(), c, time.Now().UTC())
	a.cache[c.Contest] = cachedSummary{version: snapVersion, built: time.Now(), cfg: c, sum: s}
	return s
}

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.Handle("/tiles/", a.tiles)

	mux.HandleFunc("/api/summary", func(w http.ResponseWriter, req *http.Request) {
		c := a.viewConfig(req)
		s := *a.summary(c)
		s.Units = c.Units
		writeJSON(w, map[string]any{"summary": s, "title": c.Title, "map_tiles": c.MapTiles})
	})

	mux.HandleFunc("/api/map", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, BuildMap(a.store.Snapshot(), a.viewConfig(req)))
	})

	mux.HandleFunc("/api/operators", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, BuildOps(a.store.Snapshot(), a.viewConfig(req), time.Now().UTC()))
	})

	mux.HandleFunc("/api/operator", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, BuildOpDetail(a.store.Snapshot(), a.viewConfig(req), req.URL.Query().Get("op"), time.Now().UTC()))
	})

	// Server-sent events: the page refreshes itself whenever something changes.
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, req *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		for {
			v, ch := a.store.Wait()
			fmt.Fprintf(w, "data: %d\n\n", v)
			fl.Flush()
			select {
			case <-req.Context().Done():
				return
			case <-ch:
			case <-time.After(30 * time.Second): // keep-alive, and lets rates tick down when quiet
			}
			// At most one refresh a second, however busy the band is.
			select {
			case <-req.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, map[string]any{
			"version":         appVersion,
			"listener":        a.listener.Stats(),
			"demo":            a.demo.Running(),
			"data_dir":        a.dataDir,
			"ips":             localIPs(),
			"can_change":      a.canChange(req),
			"remote_settings": a.remoteSettings,
		})
	})

	mux.HandleFunc("/api/log", func(w http.ResponseWriter, req *http.Request) {
		since, _ := strconv.ParseInt(req.URL.Query().Get("since"), 10, 64)
		writeJSON(w, logs.Since(since))
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			writeJSON(w, a.cfg.Get())
			return
		}
		if !a.allowChange(w, req) {
			return
		}
		old := a.cfg.Get()
		var c Config
		if err := json.NewDecoder(req.Body).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if c.WebPort <= 0 || c.WebPort > 65535 {
			c.WebPort = old.WebPort
		}
		if err := a.cfg.Set(c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		nc := a.cfg.Get()
		if fmt.Sprint(nc.UDPPorts) != fmt.Sprint(old.UDPPorts) {
			a.listener.Start(nc.UDPPorts)
		}
		logs.Add("app", "settings saved")
		msg := "saved"
		if nc.WebPort != old.WebPort {
			msg = "saved; the new web port is used after the program restarts"
		}
		a.store.mu.Lock()
		a.store.bumpLocked()
		a.store.mu.Unlock()
		writeJSON(w, map[string]string{"ok": msg})
	})

	mux.HandleFunc("/api/import", func(w http.ResponseWriter, req *http.Request) {
		if !a.allowChange(w, req) {
			return
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, 64<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := strings.TrimSuffix(filepath.Base(req.URL.Query().Get("name")), filepath.Ext(req.URL.Query().Get("name")))
		qs, err := ParseADIF(string(body), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.store.AddQSOs(qs)
		logs.Add("app", "imported %d contacts from %s", len(qs), name)
		writeJSON(w, map[string]any{"ok": fmt.Sprintf("imported %d contacts", len(qs)), "count": len(qs)})
	})

	mux.HandleFunc("/api/contest/delete", func(w http.ResponseWriter, req *http.Request) {
		if !a.allowChange(w, req) {
			return
		}
		var body struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Key == "" {
			http.Error(w, "which contest?", http.StatusBadRequest)
			return
		}
		n := a.store.DropContest(body.Key)
		logs.Add("app", "deleted %d contacts from %s", n, body.Key)
		writeJSON(w, map[string]any{"ok": fmt.Sprintf("deleted %d contacts", n)})
	})

	mux.HandleFunc("/api/demo", func(w http.ResponseWriter, req *http.Request) {
		if !a.allowChange(w, req) {
			return
		}
		var body struct {
			On bool `json:"on"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		if body.On {
			a.demo.Start(func(b []byte) { a.listener.handle(b, "demo") })
		} else {
			a.demo.Stop()
		}
		writeJSON(w, map[string]any{"demo": a.demo.Running()})
	})

	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, req *http.Request) {
		if !a.allowChange(w, req) {
			return
		}
		writeJSON(w, map[string]string{"ok": "stopping"})
		a.quitOnce.Do(func() { close(a.quit) })
	})

	return recoverer(mux)
}

// canChange: anyone on the network can watch the dashboard, but only this computer
// changes settings, unless remote settings were switched on (the Docker image does).
func (a *App) canChange(req *http.Request) bool {
	if a.remoteSettings {
		return true
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (a *App) allowChange(w http.ResponseWriter, req *http.Request) bool {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return false
	}
	// A custom header can't be sent cross-site without a preflight we never allow,
	// so other web pages can't poke these settings.
	if req.Header.Get("X-Contest-Dashboard") != "1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	if !a.canChange(req) {
		http.Error(w, "settings can only be changed on the computer running the dashboard", http.StatusForbidden)
		return false
	}
	return true
}

func recoverer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				logs.Add("app", "BUG in %s %s: %v\n%s", req.Method, req.URL.Path, e, debug.Stack())
				http.Error(w, fmt.Sprintf("internal error: %v", e), http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, req)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

// localIPs lists this computer's LAN addresses, so the settings page can say
// what address other computers should open.
func localIPs() []string {
	out := []string{}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, ad := range addrs {
			if ipn, ok := ad.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}
