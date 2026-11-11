package api

import (
	"net/http"
	"path/filepath"
	"testing"
)

// A fresh clone has no reading. The studio still starts: the interface loads,
// the readings list answers, and anything about «the» reading answers 409 so
// the console can send the reader to create one.
func TestHubStartsWithoutAReading(t *testing.T) {
	dir := t.TempDir()
	h, err := NewHub(HubConfig{
		Config:      Config{WebDir: dir},
		LecturasDir: filepath.Join(dir, "lecturas"),
		LibrosDir:   filepath.Join(dir, "libros"),
	})
	if err != nil {
		t.Fatalf("a studio with no reading did not start: %v", err)
	}
	routes := h.Routes()
	if rec := call(t, routes, "GET", "/api/reading", ""); rec.Code != http.StatusConflict {
		t.Errorf("/api/reading answered %d, want 409", rec.Code)
	}
	if rec := call(t, routes, "GET", "/api/lecturas", ""); rec.Code != http.StatusOK {
		t.Errorf("/api/lecturas answered %d: %s", rec.Code, rec.Body)
	}
}
