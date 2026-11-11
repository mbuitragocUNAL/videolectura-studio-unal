package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"studio/internal/render"
)

/*
The background tracks (lineamiento 2.j), in one folder for every reading:
music is usually licensed to whoever downloaded it, so like the books it stays
out of git and each person keeps their own. A reading names its track in
lectura.toml's [musica], and the render lays it under the narration.
*/

var musicExt = map[string]bool{".mp3": true, ".m4a": true, ".wav": true, ".ogg": true, ".flac": true, ".aac": true, ".opus": true}

func (h *Hub) musicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/musica", h.listMusic)
	mux.HandleFunc("POST /api/musica", h.uploadMusic)
	mux.HandleFunc("GET /api/musica/{name}", h.musicFile)
}

func (h *Hub) musicPath(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") ||
		!musicExt[strings.ToLower(filepath.Ext(name))] {
		return "", fmt.Errorf("«%s» no es una pista de música", name)
	}
	return filepath.Join(h.cfg.MusicDir, name), nil
}

type musicEntry struct {
	Name     string  `json:"nombre"`
	Bytes    int64   `json:"bytes"`
	Duration float64 `json:"dur"`
}

func (h *Hub) listMusic(w http.ResponseWriter, r *http.Request) {
	out := []musicEntry{}
	entries, _ := os.ReadDir(h.cfg.MusicDir)
	for _, e := range entries {
		if e.IsDir() || !musicExt[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		d, _ := render.ProbeDuration(filepath.Join(h.cfg.MusicDir, e.Name()))
		out = append(out, musicEntry{Name: e.Name(), Bytes: info.Size(), Duration: d})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	writeJSON(w, http.StatusOK, out)
}

func (h *Hub) uploadMusic(w http.ResponseWriter, r *http.Request) {
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
	path, err := h.musicPath(filepath.Base(header.Filename))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.MkdirAll(h.cfg.MusicDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	d, err := render.ProbeDuration(path)
	if err != nil || d <= 0 {
		os.Remove(path)
		writeError(w, http.StatusUnprocessableEntity, errors.New("ese archivo no se puede leer como audio"))
		return
	}
	info, _ := os.Stat(path)
	writeJSON(w, http.StatusOK, musicEntry{Name: filepath.Base(path), Bytes: info.Size(), Duration: d})
}

func (h *Hub) musicFile(w http.ResponseWriter, r *http.Request) {
	path, err := h.musicPath(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	http.ServeFile(w, r, path)
}
