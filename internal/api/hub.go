package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

/*
Hub holds every reading the studio knows and the one being recorded.

A Server is one reading: its marcas.json, its takes, its renders. The hub sits
in front of it, swaps it for another when the reader picks a different reading,
and answers what no single reading can: which readings exist, which books are
on the shelf, and what a reading would look like before it is prepared.

The preparation itself is preparar/lector.py. The hub runs it rather than
reimplementing it, so the command line and the console can never disagree about
what a lectura.toml means.
*/
type Hub struct {
	cfg HubConfig

	mu      sync.RWMutex
	current *Server
	handler http.Handler
	id      string // folder name under LecturasDir, or "" for one outside it

	voices voices
}

// HubConfig is the studio's configuration plus where readings and books live.
type HubConfig struct {
	Config
	LecturasDir string // one folder per reading, each with its lectura.toml
	LibrosDir   string // the PDFs, uploaded or copied in
	Python      string // interpreter with PyMuPDF
	Lector      string // path to preparar/lector.py
	Voz         string // path to preparar/voz.py, which makes the quotes' voices
	VocesDir    string // designed voices, local voices and the ElevenLabs key
}

// NewHub opens the initial reading.
func NewHub(cfg HubConfig) (*Hub, error) {
	for _, dir := range []string{cfg.LecturasDir, cfg.LibrosDir, cfg.VocesDir, cfg.MusicDir} {
		if dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}
	h := &Hub{cfg: cfg}
	dir, data := cfg.ReadingDir, cfg.DataDir
	if !prepared(dir) {
		// No reading named, or not prepared yet: the first prepared one under
		// lecturas/, and with none at all, an empty studio whose console sends
		// the reader to create one. A fresh clone starts here.
		dir, data = "", ""
		entries, _ := os.ReadDir(cfg.LecturasDir)
		for _, e := range entries {
			if candidate := filepath.Join(cfg.LecturasDir, e.Name()); e.IsDir() && prepared(candidate) {
				dir, data = candidate, filepath.Join(candidate, "estudio")
				break
			}
		}
	}
	if dir == "" {
		log.Printf("studio: no prepared reading yet; create one at /lecturas")
		h.handler = h.emptyRoutes()
		return h, nil
	}
	if data == "" {
		data = filepath.Join(dir, "estudio")
	}
	if err := h.open(dir, data); err != nil {
		return nil, err
	}
	return h, nil
}

func prepared(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "marcas.json"))
	return err == nil
}

// emptyRoutes stand in for a reading before there is any: the interface
// still loads, and every question about the reading answers 409, which the
// console reads as «go to Lecturas and make one».
func (h *Hub) emptyRoutes() http.Handler {
	pages := revalidatePages(http.FileServer(http.Dir(h.cfg.WebDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusConflict, errors.New("todavía no hay una lectura abierta: crea una en Lecturas"))
			return
		}
		pages.ServeHTTP(w, r)
	})
}

// open makes dir the reading being recorded, with its takes in dataDir.
func (h *Hub) open(dir, dataDir string) error {
	cfg := h.cfg.Config
	cfg.ReadingDir, cfg.DataDir = dir, dataDir
	srv, err := New(cfg)
	if err != nil {
		return err
	}
	id := ""
	if rel, err := filepath.Rel(h.cfg.LecturasDir, dir); err == nil && !strings.Contains(rel, "..") && !strings.Contains(rel, string(filepath.Separator)) {
		id = rel
	}
	h.mu.Lock()
	h.current, h.handler, h.id = srv, srv.Routes(), id
	h.mu.Unlock()
	log.Printf("studio: reading %s · takes in %s", dir, dataDir)
	return nil
}

// Routes is the hub's own API in front of the current reading's.
func (h *Hub) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/lecturas", h.listReadings)
	mux.HandleFunc("POST /api/lecturas/activa", h.activate)
	mux.HandleFunc("GET /api/lecturas/{id}/config", h.readingConfig)
	mux.HandleFunc("PUT /api/lecturas/{id}", h.saveReading)
	mux.HandleFunc("GET /api/lecturas/{id}/libro.pdf", h.readingBook)
	mux.HandleFunc("POST /api/analizar", h.analyse)
	mux.HandleFunc("GET /api/libros", h.listBooks)
	mux.HandleFunc("POST /api/libros", h.uploadBook)
	mux.HandleFunc("GET /api/libros/{name}/info", h.bookInfo)
	mux.HandleFunc("GET /api/libros/{name}/archivo", h.bookFile)
	h.voiceRoutes(mux)
	h.musicRoutes(mux)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		next := h.handler
		h.mu.RUnlock()
		next.ServeHTTP(w, r)
	}))
	return mux
}

/* ---------- readings ---------- */

var (
	tomlNumber = regexp.MustCompile(`(?m)^numero\s*=\s*(\d+)`)
	tomlTitle  = regexp.MustCompile(`(?m)^titulo\s*=\s*"((?:[^"\\]|\\.)*)"`)
	tomlPages  = regexp.MustCompile(`(?m)^paginas\s*=\s*"([^"]*)"`)
	validID    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,80}$`)
)

type readingEntry struct {
	ID       string `json:"id"`
	Number   int    `json:"numero"`
	Title    string `json:"titulo"`
	Pages    string `json:"paginas"`
	Prepared bool   `json:"preparada"`
	Active   bool   `json:"activa"`
	Takes    int    `json:"tomas"`
}

func (h *Hub) listReadings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	activeID, activeDir := h.id, ""
	if h.current != nil {
		activeDir = h.current.cfg.ReadingDir
	}
	h.mu.RUnlock()

	out := []readingEntry{}
	entries, _ := os.ReadDir(h.cfg.LecturasDir)
	for _, e := range entries {
		dir := filepath.Join(h.cfg.LecturasDir, e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "lectura.toml"))
		if !e.IsDir() || err != nil {
			continue
		}
		entry := readingEntry{ID: e.Name(), Active: e.Name() == activeID}
		if m := tomlNumber.FindSubmatch(raw); m != nil {
			fmt.Sscan(string(m[1]), &entry.Number)
		}
		if m := tomlTitle.FindSubmatch(raw); m != nil {
			entry.Title = strings.ReplaceAll(string(m[1]), `\"`, `"`)
		}
		if m := tomlPages.FindSubmatch(raw); m != nil {
			entry.Pages = string(m[1])
		}
		_, err = os.Stat(filepath.Join(dir, "marcas.json"))
		entry.Prepared = err == nil
		takes, _ := filepath.Glob(filepath.Join(dir, "estudio", "takes", "take-*"))
		entry.Takes = len(takes)
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })

	writeJSON(w, http.StatusOK, map[string]any{
		"lecturas": out,
		"activa":   activeID,
		// A reading outside the folder (lectura 07) has no id; say what it is.
		"carpetaActiva": map[bool]string{true: filepath.Base(activeDir), false: ""}[activeDir != ""],
	})
}

func (h *Hub) readingDir(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("«%s» no es un nombre de lectura válido", id)
	}
	return filepath.Join(h.cfg.LecturasDir, id), nil
}

func (h *Hub) activate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	dir, err := h.readingDir(body.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "marcas.json")); err != nil {
		writeError(w, http.StatusConflict, errors.New("esa lectura todavía no está preparada"))
		return
	}
	if err := h.open(dir, filepath.Join(dir, "estudio")); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"activa": body.ID})
}

func (h *Hub) readingConfig(w http.ResponseWriter, r *http.Request) {
	dir, err := h.readingDir(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.runLector(w, nil, "config", dir)
}

// saveReading writes the reading's lectura.toml and prepares it. If it is the
// one being recorded, the studio reloads it so the console sees the change.
func (h *Hub) saveReading(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dir, err := h.readingDir(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body, err := h.requestConfig(r, dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := h.lector(r.Context(), body, "guardar", dir)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	h.mu.RLock()
	active := h.id == id
	h.mu.RUnlock()
	if active {
		if err := h.open(dir, filepath.Join(dir, "estudio")); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func (h *Hub) analyse(w http.ResponseWriter, r *http.Request) {
	dir := h.cfg.LecturasDir
	if id := r.URL.Query().Get("id"); id != "" {
		if d, err := h.readingDir(id); err == nil {
			dir = d
		}
	}
	body, err := h.requestConfig(r, dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.runLector(w, body, "analizar")
}

// requestConfig reads {config} from the console and rewrites a book chosen
// from the shelf (a bare file name) as a path relative to the reading's
// folder, which stays valid on the host and in the container alike.
func (h *Hub) requestConfig(r *http.Request, dir string) ([]byte, error) {
	var body struct {
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("bad request: %w", err)
	}
	if lectura, ok := body.Config["lectura"].(map[string]any); ok {
		if name, ok := lectura["libro"].(string); ok && name != "" && !strings.ContainsAny(name, `/\~`) {
			if rel, err := filepath.Rel(dir, filepath.Join(h.cfg.LibrosDir, name)); err == nil {
				lectura["libro"] = filepath.ToSlash(rel)
			}
		}
	}
	return json.Marshal(map[string]any{"config": body.Config, "carpeta": dir})
}

// readingBook serves the PDF a reading points at, so the console can show its
// pages. Only that file: the path comes from the reading's own lectura.toml.
func (h *Hub) readingBook(w http.ResponseWriter, r *http.Request) {
	dir, err := h.readingDir(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := h.lector(r.Context(), nil, "config", dir)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var cfg struct {
		Path string `json:"libroRuta"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil || !strings.HasSuffix(strings.ToLower(cfg.Path), ".pdf") {
		writeError(w, http.StatusNotFound, errors.New("la lectura no tiene libro"))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, cfg.Path)
}

/* ---------- books ---------- */

type bookEntry struct {
	Name  string `json:"nombre"`
	Bytes int64  `json:"bytes"`
}

func (h *Hub) listBooks(w http.ResponseWriter, r *http.Request) {
	out := []bookEntry{}
	entries, _ := os.ReadDir(h.cfg.LibrosDir)
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, bookEntry{Name: e.Name(), Bytes: info.Size()})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Hub) bookPath(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") ||
		!strings.EqualFold(filepath.Ext(name), ".pdf") {
		return "", fmt.Errorf("«%s» no es un libro", name)
	}
	return filepath.Join(h.cfg.LibrosDir, name), nil
}

func (h *Hub) uploadBook(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("malformed upload: %w", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing file: %w", err))
		return
	}
	defer file.Close()
	path, err := h.bookPath(filepath.Base(header.Filename))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	dst, err := os.Create(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, copyErr := io.Copy(dst, file)
	if err := errors.Join(copyErr, dst.Close()); err != nil {
		os.Remove(path)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	h.runLector(w, nil, "libro", path)
}

func (h *Hub) bookInfo(w http.ResponseWriter, r *http.Request) {
	path, err := h.bookPath(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.runLector(w, nil, "libro", path)
}

func (h *Hub) bookFile(w http.ResponseWriter, r *http.Request) {
	path, err := h.bookPath(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, path)
}

/* ---------- lector.py ---------- */

// runLector answers the request with what lector.py printed.
func (h *Hub) runLector(w http.ResponseWriter, stdin []byte, args ...string) {
	out, err := h.lector(context.Background(), stdin, args...)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// lector runs one of lector.py's JSON commands. Its errors come back as
// {"error": "..."} on stdout, in Spanish, written for the person preparing the
// reading; those are passed on as they are.
func (h *Hub) lector(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.cfg.Python, append([]string{h.cfg.Lector}, args...)...)
	cmd.Env = append(os.Environ(), "LIBROS="+h.cfg.LibrosDir)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if stderr.Len() > 0 {
		log.Printf("lector %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	var reply struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(stdout.Bytes(), &reply) == nil && reply.Error != "" {
		return nil, errors.New(reply.Error)
	}
	if runErr != nil {
		return nil, fmt.Errorf("lector.py %s: %v", args[0], runErr)
	}
	return stdout.Bytes(), nil
}
