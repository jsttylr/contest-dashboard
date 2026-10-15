package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Free map backgrounds that need no account or API key. Tiles are fetched by the
// dashboard (not each browser) and kept on disk, so a map that has been looked at
// once keeps working without internet, for example at Field Day.
var tileSources = map[string]string{
	"esri-dark":  "https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Dark_Gray_Base/MapServer/tile/{z}/{y}/{x}",
	"esri-light": "https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Light_Gray_Base/MapServer/tile/{z}/{y}/{x}",
	"osm":        "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
}

// Deeper zooms aren't useful for a contest map and would fill the disk cache.
const maxTileZoom = 8

type TileCache struct {
	dir    string
	client *http.Client
}

func NewTileCache(dir string) *TileCache {
	return &TileCache{dir: dir, client: &http.Client{Timeout: 15 * time.Second}}
}

// ServeHTTP answers /tiles/{source}/{z}/{x}/{y}.
func (t *TileCache) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/tiles/"), "/")
	if len(parts) != 4 {
		http.NotFound(w, req)
		return
	}
	src, ok := tileSources[parts[0]]
	z, ez := strconv.Atoi(parts[1])
	x, ex := strconv.Atoi(parts[2])
	y, ey := strconv.Atoi(strings.TrimSuffix(parts[3], ".png"))
	if !ok || ez != nil || ex != nil || ey != nil || z < 0 || z > maxTileZoom || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		http.NotFound(w, req)
		return
	}
	path := filepath.Join(t.dir, parts[0], itoa(z), itoa(x), itoa(y)+".img")
	if data, err := os.ReadFile(path); err == nil {
		writeTile(w, data)
		return
	}
	url := strings.NewReplacer("{z}", itoa(z), "{x}", itoa(x), "{y}", itoa(y)).Replace(src)
	up, _ := http.NewRequestWithContext(req.Context(), http.MethodGet, url, nil)
	// OpenStreetMap's rules ask every program to say who it is.
	up.Header.Set("User-Agent", fmt.Sprintf("contest-dashboard/%s (+https://github.com/jsttylr/contest-dashboard)", appVersion))
	resp, err := t.client.Do(up)
	if err != nil {
		http.Error(w, "map tiles unavailable (no internet?)", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		http.Error(w, fmt.Sprintf("map tile server said %s", resp.Status), http.StatusBadGateway)
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		os.WriteFile(path, data, 0o644)
	}
	writeTile(w, data)
}

func writeTile(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Write(data)
}
