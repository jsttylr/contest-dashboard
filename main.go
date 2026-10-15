package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

var appVersion = "0.2.0"

func main() {
	dir := flag.String("data", dataDir(), "folder for settings and the contact log")
	noBrowser := flag.Bool("no-browser", false, "don't open the dashboard in a browser on start")
	demo := flag.Bool("demo", false, "start the made-up demo contest")
	remote := flag.Bool("remote-settings", os.Getenv("CONTEST_DASHBOARD_REMOTE_SETTINGS") == "1",
		"allow settings to be changed from other computers")
	flag.Parse()

	cfgStore, err := LoadConfig(filepath.Join(*dir, "config.json"))
	if err != nil {
		fatal("Contest Dashboard can't read its settings file in %s:\n%v", *dir, err)
	}
	cfg := cfgStore.Get()
	url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.WebPort)

	// Already running? Just show the dashboard again.
	if alreadyRunning(cfg.WebPort) {
		if !*noBrowser {
			openBrowser(url)
		}
		return
	}
	setupLogFile(filepath.Join(*dir, "contest-dashboard.log"))
	logs.Add("app", "Contest Dashboard %s, data in %s", appVersion, *dir)

	store, err := OpenStore(filepath.Join(*dir, "contacts.jsonl"))
	if err != nil {
		fatal("Contest Dashboard can't open its contact log in %s:\n%v", *dir, err)
	}
	listener := NewListener(store)
	listener.Start(cfg.UDPPorts)

	app := &App{cfg: cfgStore, store: store, listener: listener, demo: &Demo{}, dataDir: *dir,
		remoteSettings: *remote, quit: make(chan struct{})}
	if *demo {
		app.demo.Start(func(b []byte) { listener.handle(b, "demo") })
	}

	// The dashboard is for everyone on the network (spectators, a TV in the shack),
	// so listen on all addresses. Settings stay locked to this computer; see canChange.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.WebPort))
	if err != nil {
		fatal("Contest Dashboard can't open its web page on port %d:\n%v", cfg.WebPort, err)
	}
	srv := &http.Server{Handler: app.handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)

	fmt.Printf("\nDashboard: %s\nLeave this running. Press Ctrl+C to stop.\n\n", url)
	if !*noBrowser {
		openBrowser(url)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-app.quit:
	}
	logs.Add("app", "stopping")
	app.demo.Stop()
	listener.Stop()
	store.Close()
}

func alreadyRunning(port int) bool {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// setupLogFile keeps a log next to the settings, since the Windows build has no console window.
func setupLogFile(path string) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.Rename(path, path+".old")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, quietWriter{os.Stderr}))
}

// quietWriter ignores errors so a missing console never stops file logging.
type quietWriter struct{ w io.Writer }

func (q quietWriter) Write(p []byte) (int, error) { q.w.Write(p); return len(p), nil }

func fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	showError(msg)
	os.Exit(1)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
