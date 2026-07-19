// Package webui serves a small local web page that drives the same
// activity engine the CLI uses, so the tool can be operated by clicking
// instead of remembering flags. It's a plain net/http server rather than a
// native GUI toolkit (Fyne, Gio, ...) on purpose: those pull in cgo on at
// least one target platform, which the build pipeline doesn't support.
package webui

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"fberadicator/internal/activity"
	"fberadicator/internal/browser"
	"fberadicator/internal/session"
)

//go:embed index.html
var assets embed.FS

// Server holds the state of at most one in-progress run at a time — this
// tool drives a single browser session, so there's never a reason to run
// two at once.
type Server struct {
	mu      sync.Mutex
	running bool
	lines   chan string
}

func New() *Server {
	return &Server{}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/modes", s.handleModes)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/stream", s.handleStream)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

type modeInfo struct {
	ID   string `json:"id"`
	Verb string `json:"verb"`
}

func (s *Server) handleModes(w http.ResponseWriter, r *http.Request) {
	modes := activity.Modes()
	infos := make([]modeInfo, 0, len(modes))
	for _, m := range modes {
		cat, _ := activity.Lookup(m)
		infos = append(infos, modeInfo{ID: m, Verb: cat.Verb})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(infos)
}

type runRequest struct {
	Mode   string `json:"mode"`
	DryRun bool   `json:"dryRun"`
	Limit  int    `json:"limit"`
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cat, ok := activity.Lookup(req.Mode)
	if !ok {
		http.Error(w, "unknown mode", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		http.Error(w, "a run is already in progress", http.StatusConflict)
		return
	}
	s.running = true
	s.lines = make(chan string, 2000)
	s.mu.Unlock()

	go s.execute(cat, req.DryRun, req.Limit)

	w.WriteHeader(http.StatusAccepted)
}

// execute runs the same browser.Find -> session.NewBrowserContext ->
// activity.Engine.Run sequence main.go uses for the CLI, just with its
// output routed to the SSE stream instead of stdout.
func (s *Server) execute(cat activity.Category, dryRun bool, limit int) {
	lines := s.lines
	out := &channelWriter{lines: lines}

	defer func() {
		close(lines)
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	chromePath, err := browser.Find()
	if err != nil {
		fmt.Fprintln(out, "Error:", err)
		return
	}
	fmt.Fprintln(out, "Using browser:", chromePath)

	ctx, cancel, err := session.NewBrowserContext(chromePath)
	if err != nil {
		fmt.Fprintln(out, "Error starting browser:", err)
		return
	}
	defer cancel()

	if dryRun {
		fmt.Fprintf(out, "[dry-run] Checking for items in: %s (nothing will be deleted)\n", cat.Name)
	} else {
		fmt.Fprintf(out, "This will act on your own Facebook account. Clearing: %s\n", cat.Name)
	}

	engine := activity.New(ctx, cat, out, dryRun, limit)
	if err := engine.Run(); err != nil {
		fmt.Fprintln(out, "Error:", err)
		return
	}
	fmt.Fprintln(out, "Finished.")
}

// channelWriter adapts io.Writer to the line channel the SSE handler reads
// from, splitting on the same line boundaries the CLI's log lines already
// have (one fmt.Fprintln call per message).
type channelWriter struct {
	lines chan string
}

func (c *channelWriter) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	if line != "" {
		c.lines <- line
	}
	return len(p), nil
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	lines := s.lines
	s.mu.Unlock()
	if lines == nil {
		http.Error(w, "no run in progress", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				fmt.Fprint(w, "event: done\ndata: done\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
