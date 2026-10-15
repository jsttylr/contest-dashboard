package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Config struct {
	WebPort  int    `json:"web_port"`
	UDPPorts []int  `json:"udp_ports"`
	MyCall   string `json:"my_call"`
	MyGrid   string `json:"my_grid"`
	Units    string `json:"units"`     // mi or km
	Contest  string `json:"contest"`   // contest key to show; empty = the latest one
	Title    string `json:"title"`     // optional heading for the dashboard
	MapTiles string `json:"map_tiles"` // esri, osm or offline
}

func defaultConfig() *Config {
	return &Config{WebPort: 8060, UDPPorts: []int{12060, 12061, 12062}, Units: "mi", MapTiles: "esri"}
}

func (c *Config) fillDefaults() {
	d := defaultConfig()
	if c.WebPort == 0 {
		c.WebPort = d.WebPort
	}
	if c.UDPPorts == nil {
		c.UDPPorts = d.UDPPorts
	}
	if c.Units != "km" {
		c.Units = "mi"
	}
	if c.MapTiles != "osm" && c.MapTiles != "offline" {
		c.MapTiles = "esri" // also upgrades the old "online" setting
	}
	c.MyCall = strings.ToUpper(strings.TrimSpace(c.MyCall))
	c.MyGrid = strings.TrimSpace(c.MyGrid)
}

type ConfigStore struct {
	mu   sync.Mutex
	path string
	cfg  *Config
}

func dataDir() string {
	if d := os.Getenv("CONTEST_DASHBOARD_DATA"); d != "" {
		return d
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "contest-dashboard")
}

func LoadConfig(path string) (*ConfigStore, error) {
	s := &ConfigStore{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.cfg = defaultConfig()
		return s, s.save()
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.fillDefaults()
	s.cfg = &c
	return s, nil
}

func (s *ConfigStore) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *s.cfg
	c.UDPPorts = append([]int(nil), s.cfg.UDPPorts...)
	return c
}

func (s *ConfigStore) Set(c Config) error {
	c.fillDefaults()
	s.mu.Lock()
	s.cfg = &c
	s.mu.Unlock()
	return s.save()
}

func (s *ConfigStore) save() error {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
