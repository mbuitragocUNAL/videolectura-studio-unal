// Package api exposes the studio over HTTP: the reading to record, the session
// being built, the takes themselves, and the renders.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"studio/internal/project"
	"studio/internal/render"
)

// Config is where the studio finds and puts things.
type Config struct {
	ReadingDir string // holds marcas.json, paginas/, audio/tts/
	DataDir    string // holds session.json, takes/, output/
	WebDir     string // the UI
	Height     int    // render height
	Gain       float64
	// Loudness every take is levelled to, in LUFS; zero leaves takes as
	// recorded.
	Loudness float64
	// Aligner is the command that transcribes a take (preparar/alinear.py
	// behind its interpreter). Empty places figures by the text's length.
	Aligner []string
	// MusicDir holds the background tracks a reading may name.
	MusicDir string
	// Pauses is how much silence the render leaves at each end of a take
	// and inside it; the zero value leaves takes as recorded.
	Pauses project.PauseRules
}

// Server wires the pieces together.
type Server struct {
	cfg     Config
	reading *project.Reading
	// typeset is the passage set in the book's type (tipografia.json), which
	// the render scrolls instead of showing the book's pages; nil for a
	// reading prepared without it.
	typeset *project.Typeset
	store   *project.Store
	runner  *render.Runner
}

// New loads the reading and session and returns a ready server.
func New(cfg Config) (*Server, error) {
	reading, err := project.LoadReading(cfg.ReadingDir)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{cfg.DataDir, filepath.Join(cfg.DataDir, "takes"), filepath.Join(cfg.DataDir, "output")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	store, err := project.OpenStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	typeset, err := project.LoadTypeset(cfg.ReadingDir)
	if err != nil {
		log.Printf("studio: %v; the render will show the book's pages", err)
	}
	s := &Server{cfg: cfg, reading: reading, typeset: typeset, store: store, runner: render.NewRunner()}
	s.repairTakes()
	for _, warning := range reading.MisplacedTakes(store.Snapshot().Takes) {
		log.Printf("studio: %s", warning)
	}
	return s, nil
}

// repairTakes gives a duration back to takes that were saved without one.
//
// Before the upload path remuxed its files, every take arrived as a live-muxed
// WebM and ffprobe answered N/A, which was stored as zero. Zero is not a small
// number here: the montage draws cuts as a percentage of it, the console adds
// it up as the runtime the rules care about, and the render keeps spans of it.
// A recording that is plainly there reads as 00:00 everywhere. Fix them once,
// at start, instead of asking anyone to record them again.
func (s *Server) repairTakes() {
	sess := s.store.Snapshot()
	repaired := map[int]project.Take{}

	for _, t := range sess.Takes {
		if t.Duration > 0 {
			continue
		}
		path := filepath.Join(s.cfg.DataDir, "takes", t.File)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if rescued, err := render.Salvage(path); err != nil {
			log.Printf("studio: %v", err)
		} else if rescued {
			log.Printf("studio: take %d had its header out of place; salvaged it", t.ID)
		}
		if err := render.Remux(path); err != nil {
			log.Printf("studio: %v", err)
		}
		duration, err := render.ProbeDuration(path)
		if err != nil || duration <= 0 {
			log.Printf("studio: take %d still has no duration: %v", t.ID, err)
			continue
		}
		t.Duration = duration
		if info, err := os.Stat(path); err == nil {
			t.Bytes = info.Size()
		}
		repaired[t.ID] = t
		log.Printf("studio: recovered take %d · %.2f s", t.ID, duration)
	}

	if len(repaired) == 0 {
		return
	}
	if _, err := s.store.Update(func(sess *project.Session) error {
		for i, t := range sess.Takes {
			if fixed, ok := repaired[t.ID]; ok {
				sess.Takes[i] = fixed
			}
		}
		return nil
	}); err != nil {
		log.Printf("studio: could not save the recovered takes: %v", err)
	}
}

// Routes builds the HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/reading", s.getReading)
	mux.HandleFunc("GET /api/session", s.getSession)
	mux.HandleFunc("PUT /api/session/timeline", s.putTimeline)

	mux.HandleFunc("POST /api/takes", s.postTake)
	mux.HandleFunc("PATCH /api/takes/{id}", s.patchTake)
	mux.HandleFunc("DELETE /api/takes/{id}", s.deleteTake)
	mux.HandleFunc("GET /api/takes/{id}/media", s.getTakeMedia)

	mux.HandleFunc("POST /api/renders", s.postRender)
	mux.HandleFunc("GET /api/renders", s.listRenders)
	mux.HandleFunc("GET /api/renders/{id}", s.getRender)
	mux.HandleFunc("GET /api/renders/{id}/events", s.renderEvents)
	mux.HandleFunc("GET /api/renders/{id}/output", s.getRenderOutput)

	// The reading's own assets: page images and the synthesised quotes.
	mux.Handle("GET /assets/", http.StripPrefix("/assets/",
		http.FileServer(http.Dir(s.cfg.ReadingDir))))
	mux.Handle("GET /", revalidatePages(http.FileServer(http.Dir(s.cfg.WebDir))))

	return logging(mux)
}

/* ---------- reading and session ---------- */

func (s *Server) getReading(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.reading)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Snapshot())
}

func (s *Server) putTimeline(w http.ResponseWriter, r *http.Request) {
	var body project.Timeline
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.Update(func(sess *project.Session) error {
		sess.Timeline = body
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

/* ---------- takes ---------- */

// takeMeta is the JSON part of a take upload.
type takeMeta struct {
	Label     string               `json:"label"`
	From      float64              `json:"from"`
	To        float64              `json:"to"`
	Paragraph int                  `json:"paragraph"`
	Plate     bool                 `json:"plate"`
	Figure    string               `json:"figure"`
	Cues      []project.Cue        `json:"cues"`
	Levels    []project.Level      `json:"levels"`
	Trace     []project.TracePoint `json:"trace"`
}

// postTake accepts a recording as multipart/form-data: a "meta" field carrying
// the JSON above and a "file" field with the media.
func (s *Server) postTake(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("malformed upload: %w", err))
		return
	}
	// The metadata may arrive as a plain form value or, from clients that treat
	// it as a document, as a file part. Reading only the first silently drops
	// the take's label, cues and envelope, which is a miserable way to find out.
	raw := r.FormValue("meta")
	if raw == "" {
		if f, _, err := r.FormFile("meta"); err == nil {
			defer f.Close()
			if b, err := io.ReadAll(io.LimitReader(f, 8<<20)); err == nil {
				raw = string(b)
			}
		}
	}
	var meta takeMeta
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("bad meta: %w", err))
			return
		}
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing file: %w", err))
		return
	}
	defer file.Close()

	// The id is allocated under the store's lock so two uploads cannot claim it.
	var id int
	if _, err := s.store.Update(func(sess *project.Session) error {
		id = sess.NextTakeID()
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	name := fmt.Sprintf("take-%02d%s", id, extensionOf(header.Filename))
	path := filepath.Join(s.cfg.DataDir, "takes", name)
	dst, err := os.Create(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	written, copyErr := io.Copy(dst, file)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(path)
		writeError(w, http.StatusInternalServerError, errors.Join(copyErr, closeErr))
		return
	}

	// MediaRecorder hands over a live-muxed WebM: no duration in the header and
	// no seek index. Remux before anything reads it, so the editor can scrub it
	// and ffprobe can measure it. If the remux fails the original still plays,
	// so keep it and let the probe try anyway.
	//
	// Salvage runs first and is usually a no-op: it only does anything when the
	// header is not where it belongs, which means the browser sent a file no
	// player would open.
	if rescued, err := render.Salvage(path); err != nil {
		log.Printf("studio: %v", err)
	} else if rescued {
		log.Printf("studio: take %d arrived with its header out of place; salvaged it", id)
	}
	if err := render.Remux(path); err != nil {
		log.Printf("studio: %v", err)
	}
	if info, statErr := os.Stat(path); statErr == nil {
		written = info.Size()
	}

	// Trust the file, not the browser: MediaRecorder's own duration metadata is
	// frequently missing or wrong. A take that lands with duration zero looks
	// empty in every scope and renders to nothing, so say so out loud rather
	// than storing the zero in silence.
	duration, err := render.ProbeDuration(path)
	if err != nil {
		log.Printf("studio: take %d saved without a duration: %v", id, err)
		duration = 0
	}

	take := project.Take{
		ID: id, File: name, Label: meta.Label,
		Duration: duration, Bytes: written,
		From: meta.From, To: meta.To, Paragraph: meta.Paragraph, Plate: meta.Plate,
		Figure: meta.Figure,
		Cues:   meta.Cues, Levels: meta.Levels, Trace: meta.Trace,
		RecordedAt: time.Now().UTC(),
	}
	out, err := s.store.Update(func(sess *project.Session) error {
		sess.Takes = append(sess.Takes, take)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"take": take, "session": out})
}

// takePatch carries the editable parts of a take.
type takePatch struct {
	Label *string        `json:"label"`
	Cuts  *[]project.Cut `json:"cuts"`
	Cues  *[]project.Cue `json:"cues"`
}

func (s *Server) patchTake(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var body takePatch
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.Update(func(sess *project.Session) error {
		take, ok := sess.TakeByID(id)
		if !ok {
			return errNotFound
		}
		if body.Label != nil {
			take.Label = *body.Label
		}
		if body.Cuts != nil {
			take.Cuts = project.MergeCuts(*body.Cuts)
		}
		if body.Cues != nil {
			take.Cues = *body.Cues
		}
		return nil
	})
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteTake(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var file string
	out, err := s.store.Update(func(sess *project.Session) error {
		for i, t := range sess.Takes {
			if t.ID != id {
				continue
			}
			file = t.File
			sess.Takes = append(sess.Takes[:i], sess.Takes[i+1:]...)
			// Drop it from the timeline too, or the render would look for a
			// take that is no longer there.
			kept := sess.Timeline.Entries[:0]
			for _, e := range sess.Timeline.Entries {
				if e.TakeID != id {
					kept = append(kept, e)
				}
			}
			sess.Timeline.Entries = kept
			return nil
		}
		return errNotFound
	})
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if file != "" {
		os.Remove(filepath.Join(s.cfg.DataDir, "takes", file))
	}
	writeJSON(w, http.StatusOK, out)
}

// getTakeMedia serves the recording. http.ServeContent handles range requests,
// which is what lets the editor scrub without downloading the whole take.
func (s *Server) getTakeMedia(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sess := s.store.Snapshot()
	take, ok := sess.TakeByID(id)
	if !ok {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	path := filepath.Join(s.cfg.DataDir, "takes", take.File)
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.ServeContent(w, r, take.File, info.ModTime(), f)
}

// plateFile is the colour plate the passage sends the reader to. Lineamiento
// 3.2.3 wants it on screen whole, with its caption and nothing else, which is
// why the render letterboxes it rather than cropping it to the frame.
const plateFile = "laminaF-446.png"

// plateHold is how long the lámina stays up when nobody recorded a reading of
// its caption. Lineamiento 3.2.3 asks for two or three seconds after the
// caption; with no caption read, three is the whole of it.
const plateHold = 3.0

/*
passageEnd is where the assigned reading stops, in strip pixels.

It is measured, not derived. marcas.py places each line by interpolating
between page boundaries, and that interpolation drifts badly over the last
page: it puts the closing line at 21687 when the ink actually ends at 20864.
Trusting it left «Subjetividad objetiva: geometría proyectiva» and the section
after it on screen through the final paragraph, which is text nobody reads.

The number comes from scanning the strip for the last row of ink before the
111-pixel gap that precedes that heading. It belongs to this passage, like the
rest of what marcas.py produces.
*/
const passageEnd = 20880

/*
frameTop is where the window sits so the paragraph being read is centred in it.

Anchoring on the paragraph's first line instead — four tenths of the way down,
as the reader had it on screen — spends the top of the frame on the paragraph
before and pushes the end of a long one off the bottom. On screen that was
right, because the reader's eye moved down the page as they read; a still has
no eye to follow, so the thing being read belongs in the middle of it.

A paragraph too tall to fit is shown from its start, since the alternative is
losing its opening.
*/
func frameTop(p project.Paragraph, stripHeight, window int) int {
	top := p.Start * float64(stripHeight)
	if p.Height()*float64(stripHeight) >= float64(window) {
		return int(top) - window/20
	}
	middle := (p.Start + p.Bottom) / 2 * float64(stripHeight)
	return int(middle) - window/2
}

/* ---------- renders ---------- */

func (s *Server) postRender(w http.ResponseWriter, r *http.Request) {
	sess := s.store.Snapshot()
	takes := sess.OrderedTakes()
	if len(takes) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("there are no takes to render"))
		return
	}

	strip, err := render.LoadStrip(s.cfg.ReadingDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	plan := render.Plan{
		Output:   filepath.Join(s.cfg.DataDir, "output", "final.mp4"),
		WorkDir:  filepath.Join(s.cfg.DataDir, "output", "pieces"),
		Loudness: s.cfg.Loudness,
	}
	if m := s.reading.Music; m != nil && m.File != "" {
		path := filepath.Join(s.cfg.MusicDir, filepath.Base(m.File))
		if _, err := os.Stat(path); err != nil {
			writeError(w, http.StatusConflict, fmt.Errorf("no encuentro la música «%s» en %s", m.File, s.cfg.MusicDir))
			return
		}
		below := m.Below
		if below <= 0 {
			below = 14
		}
		plan.Music = &render.Music{File: path, Below: below, Duck: m.Duck, FadeIn: 2, FadeOut: 4}
	}
	// Start from an empty workspace. Pieces are numbered by their place in
	// the video, so a render with fewer takes than the last one leaves higher
	// numbers behind — and the leftovers look exactly like real output when
	// you go in to check what was produced. They never reached the finished
	// video, which is assembled from this run's own list, but they cost an
	// hour of chasing a truncation that was not there.
	if err := os.RemoveAll(plan.WorkDir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.MkdirAll(plan.WorkDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// The page each take shows, drawn from the book's own images rather than
	// taken from the screen recording. A window this shape is exactly 16:9, so
	// it fills a 1920x1080 frame edge to edge with nothing stretched and no
	// bars — which is what the rules ask for and what the tab capture, at
	// whatever size the window happened to be, could not give.
	window := strip.Width * 9 / 16
	paragraphs := s.reading.Paragraphs()

	plan.Prepare = func(step func(string)) ([]render.TakePlan, error) {
		if s.cfg.Pauses.MaxPause > 0 || s.cfg.Pauses.Head > 0 {
			step("ajustando las pausas")
			takes = s.tightenPauses(takes)
		}
		s.alignTakes(takes, paragraphs, step)
		step("dibujando las páginas")
		return s.figurePlan(takes, strip, plan.WorkDir, window, paragraphs)
	}

	job, err := s.runner.Start(plan)
	if errors.Is(err, render.ErrBusy) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job.Snapshot())
}

// figureHold is one figure shown on its own, in silence, after a paragraph.
type figureHold struct {
	frame string
}

/*
figurePlan lays the takes out as video pieces, with the figures between them.

Lineamiento 3.2.3 for every figure or lámina the author names:

  - first mention (2.d): the figure whole, with its caption, and the caption is
    read. That is a take of its own, recorded from the figure's row; until it
    is recorded the render holds the captioned figure in silence, because a
    figure shown in silence beats a figure missing.
  - every later mention (2.e): the figure again without its caption, two or
    three seconds, nothing read. Always a silent hold.
  - after either, the page comes back with a return arrow at the paragraph
    where the reading picks up.

Takes are paragraphs, so a figure goes after the paragraph whose sentence names
it. Lectura 07 has no events, only its single plate, and keeps its old flow.
*/
func (s *Server) figurePlan(takes []project.Take, strip *render.Strip, workDir string,
	window int, paragraphs []project.Paragraph) ([]render.TakePlan, error) {

	figure := func(id string, caption bool) string {
		if id == "" {
			return filepath.Join(s.cfg.ReadingDir, "paginas", plateFile)
		}
		if caption {
			id += "-pie"
		}
		return filepath.Join(s.cfg.ReadingDir, "figuras", id+".png")
	}
	hold := plateHold
	if s.reading.Screen.FigureHold > 0 {
		hold = s.reading.Screen.FigureHold
	}
	end := passageEnd
	if s.reading.PassageEnd > 0 {
		end = s.reading.PassageEnd
	}

	// Lineamiento 2.f: while a figure is being talked about it stays on the
	// right, with the text on the left at the size it has everywhere else.
	// The page fills the frame edge to edge when nothing is fixed; with a
	// figure beside it, it cannot. So a reading with fixed figures widens
	// every frame by the same amount, enough for the text column plus the
	// figure's share, and the text stays one size from start to finish.
	margin := 0
	textFrom, textTo := 0, strip.Width
	if len(s.reading.Fixed) > 0 && s.reading.TextBlock.X1 > s.reading.TextBlock.X0 {
		share := s.reading.Screen.FigureWidth
		if share <= 0 || share >= 0.8 {
			share = 0.38
		}
		margin = strip.Width / 40
		textFrom = max(0, int(s.reading.TextBlock.X0)-margin)
		textTo = min(strip.Width, int(s.reading.TextBlock.X1)+margin)
		window = max(window, int(float64(textTo-textFrom)/(1-share))*9/16)
	}
	frameWidth := window * 16 / 9

	recorded := map[string]bool{}
	for _, t := range takes {
		if t.Plate {
			recorded[t.Figure] = true
		}
	}

	// What each paragraph sends the reader to, in the order the text does.
	holds := map[int][]figureHold{}
	if len(s.reading.Events) > 0 {
		for _, e := range s.reading.Events {
			p := s.reading.ParagraphOfLine(e.Line)
			if p < 0 {
				continue
			}
			// Shown inside the paragraph's own take, where the text sends
			// the viewer to it (2.d, 2.e).
			if p < len(paragraphs) && s.reading.HandledInline(paragraphs[p], e, recorded) {
				continue
			}
			switch {
			case e.Mode == project.ModeBrief:
				holds[p] = append(holds[p], figureHold{figure(e.Figure, false)})
			case !recorded[e.Figure]:
				holds[p] = append(holds[p], figureHold{figure(e.Figure, true)})
			}
		}
	} else if s.reading.Plate != nil && !recorded[""] {
		if p := s.reading.ParagraphOfLine(s.reading.Plate.Line); p >= 0 {
			holds[p] = append(holds[p], figureHold{figure("", true)})
		}
	}

	var out []render.TakePlan
	afterFigure := false
	for _, t := range takes {
		// A take is one still, or, when its paragraph moves from one fixed
		// figure to another, a still per figure, each with the rest of the
		// sound cut away. Splitting by cuts keeps the audio sample-exact.
		pieces := []takePiece{{frame: figure(t.Figure, true), fit: true, cuts: t.Cuts}}
		if !t.Plate {
			var err error
			if s.typeset != nil {
				pieces, err = s.scrollPieces(t, workDir, paragraphs, afterFigure, figure, recorded, hold)
			} else {
				pieces, err = s.pagePieces(t, strip, workDir, window, frameWidth, end,
					textFrom, textTo, paragraphs, afterFigure, figure, recorded, hold)
			}
			if err != nil {
				return nil, err
			}
		}

		for _, piece := range pieces {
			if piece.silent {
				out = append(out, render.TakePlan{
					Frame: piece.frame, Fit: true, Silent: true, Duration: piece.seconds, Height: s.cfg.Height,
				})
				continue
			}
			kept, _ := project.RetimeCues(t.Cues, piece.cuts)
			var cues []render.QuoteCue
			for _, c := range kept {
				cues = append(cues, render.QuoteCue{
					File: filepath.Join(s.cfg.ReadingDir, "audio", "tts", c.Quote+".mp3"),
					At:   c.At,
				})
			}
			out = append(out, render.TakePlan{
				Source:   filepath.Join(s.cfg.DataDir, "takes", t.File),
				Frame:    piece.frame,
				Fit:      piece.fit,
				Duration: t.Duration,
				Cuts:     piece.cuts,
				Cues:     cues,
				Gain:     s.cfg.Gain,
				Height:   s.cfg.Height,
				Scroll:   piece.scroll,
			})
		}
		afterFigure = t.Plate

		if !t.Plate {
			for _, h := range holds[t.Paragraph] {
				out = append(out, render.TakePlan{
					Frame: h.frame, Fit: true, Silent: true, Duration: hold, Height: s.cfg.Height,
				})
				afterFigure = true
			}
		}
	}
	return out, nil
}

// tightenPauses adds to each take the cuts that trim its ends and shorten its
// long pauses (project.PauseCuts). They live only in this render: the take
// keeps the cuts the reader made, and the console still shows those alone.
// A quote plays over a silence in the take, so that silence is never cut.
func (s *Server) tightenPauses(takes []project.Take) []project.Take {
	clip := map[string]float64{}
	for _, q := range s.reading.Quotes {
		clip[q.ID] = q.Duration
	}
	out := make([]project.Take, len(takes))
	for i, t := range takes {
		out[i] = t
		db, err := render.SpeechLevels(filepath.Join(s.cfg.DataDir, "takes", t.File))
		if err != nil {
			log.Printf("studio: pauses of take %d: %v", t.ID, err)
			continue
		}
		var protect []project.Span
		for _, c := range t.Cues {
			protect = append(protect, project.Span{From: c.At - 0.1, To: c.At + clip[c.Quote] + 0.3})
		}
		auto := project.PauseCuts(db, render.LevelStep, t.Duration, protect, s.cfg.Pauses)
		out[i].Cuts = project.MergeCuts(append(append([]project.Cut(nil), t.Cuts...), auto...))
	}
	return out
}

func (s *Server) listRenders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.runner.List())
}

func (s *Server) getRender(w http.ResponseWriter, r *http.Request) {
	job, ok := s.runner.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	snap := job.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"job": snap, "log": job.Log()})
}

// renderEvents streams the job's progress as server-sent events, so the UI can
// show a live bar instead of polling.
func (s *Server) renderEvents(w http.ResponseWriter, r *http.Request) {
	job, ok := s.runner.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	for {
		snap := job.Snapshot()
		raw, _ := json.Marshal(snap)
		fmt.Fprintf(w, "data: %s\n\n", raw)
		flusher.Flush()

		if snap.State == render.StateDone || snap.State == render.StateFailed {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *Server) getRenderOutput(w http.ResponseWriter, r *http.Request) {
	job, ok := s.runner.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	snap := job.Snapshot()
	if snap.State != render.StateDone || snap.Output == "" {
		writeError(w, http.StatusConflict, errors.New("the render is not finished"))
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="final.mp4"`)
	http.ServeFile(w, r, snap.Output)
}

/* ---------- helpers ---------- */

var errNotFound = errors.New("not found")

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// extensionOf keeps the container the browser produced, defaulting to webm.
func extensionOf(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".webm", ".mp4", ".mov", ".mkv":
		return ext
	default:
		return ".webm"
	}
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fmt.Printf("%s %s %s\n", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

// revalidatePages makes the browser ask again for every page, while the
// hashed scripts under /_nuxt/ stay cacheable. Without it a presenter window
// opened before a rebuild kept running the old code.
func revalidatePages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/_nuxt/") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// takePiece is the picture for one stretch of a take, and the cuts that leave
// only that stretch of its sound. A silent piece holds its picture for
// seconds without taking anything from the recording.
type takePiece struct {
	frame   string
	fit     bool
	cuts    []project.Cut
	silent  bool
	seconds float64
	scroll  *render.Scroll // the typeset column moving, over frame
}

// pagePieces lays out a take that reads a paragraph. Most are one frame of
// the page. When the text sends the viewer to a figure, the take is split
// where the reader got there: the figure with its caption while the caption
// is read, a short hold, and the page again with the return arrow (2.d, 2.e);
// and when the paragraph moves from one fixed figure to another, a frame per
// figure (2.f).
func (s *Server) pagePieces(t project.Take, strip *render.Strip, workDir string,
	window, frameWidth, end, textFrom, textTo int, paragraphs []project.Paragraph,
	afterFigure bool, figure func(string, bool) string, separate map[string]bool,
	hold float64) ([]takePiece, error) {

	known := t.Paragraph >= 0 && t.Paragraph < len(paragraphs)
	top := 0
	beats := []project.Beat{{Kind: project.BeatPage, From: 0, To: 1}}
	var said []rune
	if known {
		top = frameTop(paragraphs[t.Paragraph], strip.Height(), window)
		beats, said, _ = s.reading.Beats(paragraphs[t.Paragraph], separate)
	}
	// Never let the window run past the end of the passage. Below it is
	// blank, so a frame that overshoots spends its height on nothing and
	// clips the text off the top instead.
	if top > end-window {
		top = end - window
	}

	// Seconds into the recording for a place in the text. The same place
	// always gives the same instant, so one piece ends exactly where the
	// next begins and no sound is lost or doubled between them.
	// Where Whisper heard it, when the take has been transcribed; otherwise
	// spread by the text's length and moved to the nearest pause.
	heard := func(float64) (float64, bool) { return 0, false }
	if len(beats) > 1 {
		if a, ok := s.alignment(t); ok {
			timer := a.Timer(said)
			heard = func(f float64) (float64, bool) {
				return timer(int(f*float64(len(said)) + 0.5))
			}
		}
	}
	seconds := map[float64]float64{0: 0, 1: t.Duration}
	last := 0.0
	timeOf := func(f float64) float64 {
		at, ok := seconds[f]
		if !ok {
			if at, ok = heard(f); !ok {
				at = t.TimeAt(f)
			}
		}
		at = min(max(at, last), t.Duration)
		seconds[f], last = at, at
		return at
	}

	var pieces []takePiece
	for k, b := range beats {
		switch b.Kind {
		case project.BeatHold:
			pieces = append(pieces, takePiece{frame: figure(b.Figure, b.WithCaption), fit: true,
				silent: true, seconds: hold})
			continue
		case project.BeatCaption:
			from, to := timeOf(b.From), timeOf(b.To)
			pieces = append(pieces, takePiece{frame: figure(b.Figure, true), fit: true,
				cuts: only(t, from, to)})
			continue
		}

		from, to := timeOf(b.From), timeOf(b.To)
		frame := filepath.Join(workDir, fmt.Sprintf("page-%02d.png", t.ID))
		if len(beats) > 1 {
			frame = filepath.Join(workDir, fmt.Sprintf("page-%02d-%d.png", t.ID, k+1))
		}
		opts := render.FrameOpts{Top: top, Height: window, Limit: end, Width: frameWidth}
		if b.Figure != "" {
			opts.Side = figure(b.Figure, false)
			opts.TextFrom, opts.TextTo = textFrom, textTo
			if box := s.reading.FigureBox(b.Figure); len(box) == 4 {
				opts.Hide = []image.Rectangle{image.Rect(box[0], box[1], box[2], box[3])}
			}
		}
		// The return arrow, where the reading picks up after a figure.
		switch {
		case b.Resume > 0:
			opts.Arrow = int(s.lineStart(b.Resume) * float64(strip.Height()))
		case k == 0 && afterFigure && known:
			opts.Arrow = int(paragraphs[t.Paragraph].Start * float64(strip.Height()))
		}
		if err := strip.Render(opts, frame); err != nil {
			return nil, err
		}
		pieces = append(pieces, takePiece{frame: frame, cuts: only(t, from, to)})
	}
	return pieces, nil
}

// only is the take's own cuts plus whatever lies outside from..to.
func only(t project.Take, from, to float64) []project.Cut {
	cuts := append([]project.Cut(nil), t.Cuts...)
	if from > 0 {
		cuts = append(cuts, project.Cut{From: 0, To: from})
	}
	if to < t.Duration {
		cuts = append(cuts, project.Cut{From: to, To: t.Duration})
	}
	return cuts
}

// lineStart is where passage line n sits in the strip, 0..1.
func (s *Server) lineStart(n int) float64 {
	for _, l := range s.reading.Lines {
		if l.Number == n {
			return l.Start
		}
	}
	return 0
}

// alignmentPath is where a take's transcript is kept, beside the takes.
func (s *Server) alignmentPath(t project.Take) string {
	name := strings.TrimSuffix(t.File, filepath.Ext(t.File)) + ".json"
	return filepath.Join(s.cfg.DataDir, "alineacion", name)
}

// alignment is the take's transcript, if there is one newer than the take.
func (s *Server) alignment(t project.Take) (project.Alignment, bool) {
	var a project.Alignment
	path := s.alignmentPath(t)
	info, err := os.Stat(path)
	if err != nil {
		return a, false
	}
	if take, err := os.Stat(filepath.Join(s.cfg.DataDir, "takes", t.File)); err == nil &&
		take.ModTime().After(info.ModTime()) {
		return a, false
	}
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &a) != nil || len(a.Words) == 0 {
		return a, false
	}
	return a, true
}

// alignTakes transcribes the takes whose paragraph shows a figure part of the
// way through, so the render can switch pictures on the word rather than on
// an estimate. Each take is done once; a failure is logged and that take
// falls back to the estimate, because a render with figures a second off
// beats no render at all.
func (s *Server) alignTakes(takes []project.Take, paragraphs []project.Paragraph, step func(string)) {
	if len(s.cfg.Aligner) == 0 {
		return
	}
	recorded := map[string]bool{}
	for _, t := range takes {
		if t.Plate {
			recorded[t.Figure] = true
		}
	}
	type job struct {
		take project.Take
		said []rune
	}
	var todo []job
	for _, t := range takes {
		if t.Plate || t.Paragraph < 0 || t.Paragraph >= len(paragraphs) {
			continue
		}
		beats, said, _ := s.reading.Beats(paragraphs[t.Paragraph], recorded)
		// With the typeset text every take scrolls line by line, so every
		// take is worth hearing; with the pages only a figure in the middle
		// needs it.
		if len(beats) < 2 && s.typeset == nil {
			continue
		}
		if len(said) == 0 || t.KeptDuration() < 2 {
			continue
		}
		if _, ok := s.alignment(t); !ok {
			todo = append(todo, job{t, said})
		}
	}
	for i, j := range todo {
		step(fmt.Sprintf("escuchando la toma %d (%d de %d)", j.take.ID, i+1, len(todo)))
		argv := append(append([]string(nil), s.cfg.Aligner...),
			filepath.Join(s.cfg.DataDir, "takes", j.take.File), s.alignmentPath(j.take),
			"--texto", string(j.said))
		cmd := exec.Command(argv[0], argv[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("studio: no pude transcribir la toma %d: %v: %s", j.take.ID, err,
				strings.TrimSpace(string(out)))
		}
	}
}
