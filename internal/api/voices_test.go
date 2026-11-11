package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A stand-in for voz.py: it prints what the real one prints with --json.
const fakeVoz = `#!/bin/sh
shift # --json
case "$1" in
  motores) echo '{"respuesta": {"elevenlabs": {"clave": true, "final": "abcd"}}}' ;;
  disenar)
    echo '{"avance": {"paso": "candidata 1 de 3", "hecho": 0, "total": 3}}'
    echo 'ruido que no es JSON'
    echo '{"avance": {"paso": "candidata 3 de 3", "hecho": 2, "total": 3}}'
    echo '{"respuesta": {"candidatas": [{"id": "1"}, {"id": "2"}, {"id": "3"}]}}' ;;
  elegir) echo '{"error": "No hay diseños de Glaucón: diseña primero."}'; exit 1 ;;
esac
`

func voiceHub(t *testing.T) (*Hub, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "voz.sh")
	if err := os.WriteFile(script, []byte(fakeVoz), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &Hub{cfg: HubConfig{
		LecturasDir: filepath.Join(dir, "lecturas"),
		VocesDir:    filepath.Join(dir, "voces"),
		Python:      "/bin/sh",
		Voz:         script,
	}}
	mux := http.NewServeMux()
	h.voiceRoutes(mux)
	return h, mux
}

func call(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The key is kept where only its owner can read it, and never sent back.
func TestVoiceKeyStaysOnTheServer(t *testing.T) {
	hub, h := voiceHub(t)
	rec := call(t, h, "PUT", "/api/voces/clave", `{"clave": "  sk_prueba1234  "}`)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "sk_prueba") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	path := filepath.Join(hub.cfg.VocesDir, "claves.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("claves.json: %v, mode %v", err, info.Mode())
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"elevenlabs": "sk_prueba1234"`) {
		t.Errorf("claves.json = %s", raw)
	}
	call(t, h, "PUT", "/api/voces/clave", `{"clave": ""}`)
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "sk_prueba") {
		t.Errorf("the key survived being removed: %s", raw)
	}

	if rec := call(t, h, "GET", "/api/voces/archivo/claves.json", ""); rec.Code != 404 {
		t.Errorf("claves.json was served: %d", rec.Code)
	}
	if rec := call(t, h, "GET", "/api/voces/archivo/../../etc/passwd.mp3", ""); rec.Code == 200 {
		t.Errorf("a path outside the voices folder was served")
	}
}

// Designing runs in the background; the console follows its progress and
// gets the candidates at the end. A second job waits for the first.
func TestVoiceDesignRunsAsAJob(t *testing.T) {
	_, h := voiceHub(t)
	rec := call(t, h, "POST", "/api/lecturas/22-belleza/voces/disenar", `{"personaje": "Sócrates", "motor": "qwen"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	var job voiceJob
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		json.Unmarshal(call(t, h, "GET", "/api/voces/trabajo", "").Body.Bytes(), &job)
		if job.Done {
			break
		}
	}
	if !job.Done || job.Error != "" {
		t.Fatalf("job %+v", job)
	}
	if job.Step != "candidata 3 de 3" || job.Finished != 2 || job.Total != 3 {
		t.Errorf("progress %q %d/%d", job.Step, job.Finished, job.Total)
	}
	if !strings.Contains(string(job.Result), `"candidatas"`) || job.Character != "Sócrates" {
		t.Errorf("result %s", job.Result)
	}
}

// voz.py's errors are written for the person preparing the reading; they
// reach the console as they are.
func TestVoiceErrorsPassThrough(t *testing.T) {
	_, h := voiceHub(t)
	rec := call(t, h, "POST", "/api/lecturas/22-belleza/voces/elegir", `{"personaje": "Glaucón", "candidato": "2"}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "diseña primero") {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, "POST", "/api/lecturas/22-belleza/voces/generar", `{"clips": ["../x"]}`); rec.Code != 400 {
		t.Errorf("a bad clip name was accepted: %d", rec.Code)
	}
}
