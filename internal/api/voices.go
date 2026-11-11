package api

import (
	"bufio"
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
	"strings"
	"sync"
	"time"
)

/*
The voices of the quotes (lineamiento 2.i).

Each quote is read by a synthetic voice, the same one for a character all
through the reading. preparar/voz.py does the work: it designs a character's
voice from its description, makes the clips with ElevenLabs or Qwen3-TTS, and
takes clips recorded elsewhere. The hub runs it the way it runs lector.py, so
`./estudio voces` on the command line and the console never disagree.

Designing and generating can take minutes (Qwen on a CPU, a long reading), so
they run as one background job at a time that the console follows. Afterwards
the reading is prepared again: lector.py reads the clips' durations into
marcas.json, which is what the recording console and the render use.
*/

// voiceJob is the design or generation running now, or the last one.
type voiceJob struct {
	ID        string          `json:"id"`
	Reading   string          `json:"lectura"`
	Kind      string          `json:"tipo"` // "generar" or "disenar"
	Character string          `json:"personaje,omitempty"`
	Started   time.Time       `json:"inicio"`
	Done      bool            `json:"terminado"`
	Step      string          `json:"paso"`
	Finished  int             `json:"hecho"`
	Total     int             `json:"total"`
	Result    json.RawMessage `json:"resultado,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type voices struct {
	mu     sync.Mutex
	job    *voiceJob
	cancel context.CancelFunc
}

var validClip = regexp.MustCompile(`^[0-9]{2,3}-[a-z0-9-]{1,80}$`)

func (h *Hub) voiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/voces/motores", h.voiceEngines)
	mux.HandleFunc("PUT /api/voces/clave", h.voiceKey)
	mux.HandleFunc("GET /api/voces/cuenta", h.voiceAccount)
	mux.HandleFunc("GET /api/voces/lista", h.voiceList)
	mux.HandleFunc("GET /api/voces/archivo/{path...}", h.voiceFile)
	mux.HandleFunc("GET /api/voces/trabajo", h.voiceJobStatus)
	mux.HandleFunc("DELETE /api/voces/trabajo", h.voiceJobCancel)
	mux.HandleFunc("GET /api/lecturas/{id}/voces", h.voiceState)
	mux.HandleFunc("POST /api/lecturas/{id}/voces/generar", h.voiceGenerate)
	mux.HandleFunc("POST /api/lecturas/{id}/voces/disenar", h.voiceDesign)
	mux.HandleFunc("POST /api/lecturas/{id}/voces/elegir", h.voiceChoose)
	mux.HandleFunc("GET /api/lecturas/{id}/voces/audio/{clip}", h.voiceClip)
	mux.HandleFunc("POST /api/lecturas/{id}/voces/subir/{clip}", h.voiceUpload)
}

/* ---------- running voz.py ---------- */

func (h *Hub) vozCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if h.cfg.Voz == "" {
		return nil, errors.New("el estudio no tiene preparar/voz.py configurado")
	}
	cmd := exec.CommandContext(ctx, h.cfg.Python, append([]string{h.cfg.Voz, "--json"}, args...)...)
	cmd.Env = append(os.Environ(), "STUDIO_VOCES_DIR="+h.cfg.VocesDir)
	return cmd, nil
}

// vozLine is one line voz.py prints with --json.
type vozLine struct {
	Progress *struct {
		Step     string `json:"paso"`
		Finished int    `json:"hecho"`
		Total    int    `json:"total"`
	} `json:"avance"`
	Answer json.RawMessage `json:"respuesta"`
	Error  string          `json:"error"`
}

// voz runs a quick voz.py command and returns its answer.
func (h *Hub) voz(ctx context.Context, args ...string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return h.vozRun(ctx, nil, args...)
}

// vozRun runs voz.py, handing each progress line to progress.
func (h *Hub) vozRun(ctx context.Context, progress func(step string, finished, total int), args ...string) (json.RawMessage, error) {
	cmd, err := h.vozCommand(ctx, args...)
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var answer json.RawMessage
	var failure string
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 64<<10), 16<<20)
	for scan.Scan() {
		var line vozLine
		if json.Unmarshal(scan.Bytes(), &line) != nil {
			continue
		}
		switch {
		case line.Progress != nil && progress != nil:
			progress(line.Progress.Step, line.Progress.Finished, line.Progress.Total)
		case line.Answer != nil:
			answer = line.Answer
		case line.Error != "":
			failure = line.Error
		}
	}
	runErr := cmd.Wait()
	if s := strings.TrimSpace(stderr.String()); s != "" {
		log.Printf("voz %s: %s", args[0], lastLines(s, 8))
	}
	switch {
	case failure != "":
		return nil, errors.New(failure)
	case ctx.Err() != nil:
		return nil, errors.New("cancelado")
	case runErr != nil:
		return nil, fmt.Errorf("voz.py %s: %v: %s", args[0], runErr, lastLines(stderr.String(), 3))
	}
	return answer, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (h *Hub) answerVoz(w http.ResponseWriter, r *http.Request, args ...string) {
	out, err := h.voz(r.Context(), args...)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

/* ---------- engines and the ElevenLabs key ---------- */

func (h *Hub) voiceEngines(w http.ResponseWriter, r *http.Request) {
	h.answerVoz(w, r, "motores")
}

func (h *Hub) voiceAccount(w http.ResponseWriter, r *http.Request) {
	h.answerVoz(w, r, "cuenta")
}

func (h *Hub) voiceList(w http.ResponseWriter, r *http.Request) {
	motor := r.URL.Query().Get("motor")
	if motor != "qwen" {
		motor = "elevenlabs"
	}
	h.answerVoz(w, r, "voces", "--motor", motor)
}

// voiceKey keeps the ElevenLabs key in voces/claves.json, readable only by
// its owner and outside git. It never goes back to the browser: the console
// only learns whether there is one and its last four characters.
func (h *Hub) voiceKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   *string `json:"clave"`
		Model *string `json:"modelo"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.MkdirAll(h.cfg.VocesDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	path := filepath.Join(h.cfg.VocesDir, "claves.json")
	keys := map[string]string{}
	if raw, err := os.ReadFile(path); err == nil {
		json.Unmarshal(raw, &keys)
	}
	if body.Key != nil {
		if k := strings.TrimSpace(*body.Key); k == "" {
			delete(keys, "elevenlabs")
		} else {
			keys["elevenlabs"] = k
		}
	}
	if body.Model != nil {
		if m := strings.TrimSpace(*body.Model); m == "" {
			delete(keys, "elevenlabs_modelo")
		} else {
			keys["elevenlabs_modelo"] = m
		}
	}
	raw, _ := json.MarshalIndent(keys, "", " ")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	os.Chmod(path, 0o600)
	h.answerVoz(w, r, "motores")
}

// voiceFile serves a designed candidate or a local voice's reference, from
// the voices folder and nothing outside it. The keys are never served.
func (h *Hub) voiceFile(w http.ResponseWriter, r *http.Request) {
	rel := filepath.Clean(r.PathValue("path"))
	ext := strings.ToLower(filepath.Ext(rel))
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") || (ext != ".mp3" && ext != ".wav") {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(h.cfg.VocesDir, rel))
}

/* ---------- a reading's voices ---------- */

func (h *Hub) voiceState(w http.ResponseWriter, r *http.Request) {
	dir, err := h.readingDir(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.answerVoz(w, r, "estado", dir)
}

func (h *Hub) voiceClip(w http.ResponseWriter, r *http.Request) {
	dir, err := h.readingDir(r.PathValue("id"))
	clip := r.PathValue("clip")
	if err != nil || !validClip.MatchString(clip) {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(dir, "audio", "tts", clip+".mp3"))
}

func (h *Hub) voiceGenerate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dir, err := h.readingDir(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Clips []string `json:"clips"`
		Force bool     `json:"forzar"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	args := []string{"generar", dir}
	for _, c := range body.Clips {
		if !validClip.MatchString(c) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("«%s» no es un clip", c))
			return
		}
	}
	if len(body.Clips) > 0 {
		args = append(args, "--clips", strings.Join(body.Clips, ","))
	}
	if body.Force {
		args = append(args, "--forzar")
	}
	h.startVoiceJob(w, &voiceJob{Reading: id, Kind: "generar"}, dir, args)
}

func (h *Hub) voiceDesign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dir, err := h.readingDir(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Character   string `json:"personaje"`
		Motor       string `json:"motor"`
		Description string `json:"descripcion"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Character == "" || (body.Motor != "elevenlabs" && body.Motor != "qwen") {
		writeError(w, http.StatusBadRequest, errors.New("falta el personaje o el motor"))
		return
	}
	args := []string{"disenar", dir, "--personaje", body.Character, "--motor", body.Motor}
	if body.Description != "" {
		args = append(args, "--descripcion", body.Description)
	}
	h.startVoiceJob(w, &voiceJob{Reading: id, Kind: "disenar", Character: body.Character}, dir, args)
}

func (h *Hub) voiceChoose(w http.ResponseWriter, r *http.Request) {
	dir, err := h.readingDir(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Character string `json:"personaje"`
		Candidate string `json:"candidato"`
	}
	if err := decode(r, &body); err != nil || body.Character == "" || body.Candidate == "" {
		writeError(w, http.StatusBadRequest, errors.New("falta el personaje o la candidata"))
		return
	}
	h.answerVoz(w, r, "elegir", dir, "--personaje", body.Character, "--candidato", body.Candidate)
}

func (h *Hub) voiceUpload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dir, err := h.readingDir(id)
	clip := r.PathValue("clip")
	if err != nil || !validClip.MatchString(clip) {
		writeError(w, http.StatusBadRequest, errors.New("clip no válido"))
		return
	}
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
	tmp, err := os.CreateTemp("", "voz-*"+strings.ToLower(filepath.Ext(header.Filename)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(tmp.Name())
	_, copyErr := io.Copy(tmp, file)
	if err := errors.Join(copyErr, tmp.Close()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out, err := h.voz(r.Context(), "subir", dir, "--clip", clip, tmp.Name())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if err := h.refreshVoices(r.Context(), id, dir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// refreshVoices prepares the reading again so marcas.json carries the clips'
// durations, and reloads it if it is the one being recorded.
func (h *Hub) refreshVoices(ctx context.Context, id, dir string) error {
	if _, err := h.lector(ctx, nil, "preparar", dir); err != nil {
		return fmt.Errorf("los clips están, pero no pude volver a preparar la lectura: %w", err)
	}
	h.mu.RLock()
	active := h.id == id
	h.mu.RUnlock()
	if active {
		return h.open(dir, filepath.Join(dir, "estudio"))
	}
	return nil
}

/* ---------- the background job ---------- */

func (h *Hub) startVoiceJob(w http.ResponseWriter, job *voiceJob, dir string, args []string) {
	h.voices.mu.Lock()
	if h.voices.job != nil && !h.voices.job.Done {
		running := *h.voices.job
		h.voices.mu.Unlock()
		writeError(w, http.StatusConflict, fmt.Errorf("ya hay un trabajo de voces en curso (%s, %s)", running.Kind, running.Reading))
		return
	}
	job.ID = fmt.Sprintf("v%d", time.Now().UnixNano())
	job.Started = time.Now()
	job.Step = "empezando"
	// Qwen on a CPU is slow; two hours is far more than a reading needs.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	h.voices.job, h.voices.cancel = job, cancel
	snapshot := *job
	h.voices.mu.Unlock()

	go func() {
		defer cancel()
		update := func(f func(j *voiceJob)) {
			h.voices.mu.Lock()
			f(job)
			h.voices.mu.Unlock()
		}
		out, err := h.vozRun(ctx, func(step string, finished, total int) {
			update(func(j *voiceJob) { j.Step, j.Finished, j.Total = step, finished, total })
		}, args...)
		if err == nil && job.Kind == "generar" {
			update(func(j *voiceJob) { j.Step = "leyendo las duraciones" })
			err = h.refreshVoices(context.Background(), job.Reading, dir)
		}
		update(func(j *voiceJob) {
			j.Done, j.Result = true, out
			if err != nil {
				j.Error = err.Error()
			}
		})
		log.Printf("voces: %s %s terminado (%v)", job.Kind, job.Reading, err)
	}()
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (h *Hub) voiceJobStatus(w http.ResponseWriter, r *http.Request) {
	h.voices.mu.Lock()
	defer h.voices.mu.Unlock()
	if h.voices.job == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, h.voices.job)
}

func (h *Hub) voiceJobCancel(w http.ResponseWriter, r *http.Request) {
	h.voices.mu.Lock()
	defer h.voices.mu.Unlock()
	if h.voices.cancel != nil && h.voices.job != nil && !h.voices.job.Done {
		h.voices.cancel()
	}
	w.WriteHeader(http.StatusNoContent)
}
