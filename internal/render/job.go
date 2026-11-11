package render

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"studio/internal/project"
)

// State is where a render job has got to.
type State string

const (
	StateQueued  State = "queued"
	StateRunning State = "running"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

// Plan is a whole render: every take to encode, then the concatenation.
type Plan struct {
	Takes   []TakePlan
	Output  string // final file
	WorkDir string // where the intermediate pieces go
	// Prepare, when set, builds Takes inside the job, so slow work before the
	// encode (transcribing takes) shows as progress instead of holding up the
	// request that started the render. step names what it is doing.
	Prepare func(step func(string)) ([]TakePlan, error)
	// Loudness is the level every take is brought to, in LUFS. Takes recorded
	// on different days come in at different gains; without this the video
	// jumps in volume between them. Zero leaves each take as recorded.
	Loudness float64
	// Music, when set, is laid under the finished reading.
	Music *Music
}

// Total is the expected duration of the finished video, used for progress.
func (p Plan) Total() float64 {
	var total float64
	for _, t := range p.Takes {
		spans := 0.0
		for _, s := range keptSpans(t) {
			spans += s
		}
		total += spans
	}
	return total
}

// Status is a job's public state. It is a separate type from Job on purpose:
// Job carries a mutex, and handing copies of that around is how lock-copy bugs
// start.
type Status struct {
	ID       string    `json:"id"`
	State    State     `json:"state"`
	Progress float64   `json:"progress"` // 0..1
	Step     string    `json:"step"`
	Error    string    `json:"error,omitempty"`
	Output   string    `json:"output,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
}

// Job is a running or finished render.
type Job struct {
	mu      sync.Mutex
	status  Status
	log     []string
	waiters []chan struct{}
}

// Snapshot copies the job's public state.
func (j *Job) Snapshot() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// ID identifies the job.
func (j *Job) ID() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status.ID
}

// Log returns the ffmpeg output collected so far.
func (j *Job) Log() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.log...)
}

// Wait blocks until the job changes state or the context ends, so callers can
// stream updates without polling in a tight loop.
func (j *Job) Wait(ctx context.Context) {
	j.mu.Lock()
	if j.status.State == StateDone || j.status.State == StateFailed {
		j.mu.Unlock()
		return
	}
	ch := make(chan struct{})
	j.waiters = append(j.waiters, ch)
	j.mu.Unlock()

	select {
	case <-ch:
	case <-ctx.Done():
	}
}

func (j *Job) update(fn func(*Status)) {
	j.mu.Lock()
	fn(&j.status)
	waiters := j.waiters
	j.waiters = nil
	j.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
}

func (j *Job) appendLog(line string) {
	j.mu.Lock()
	// Keep the tail only: a long render would otherwise grow without bound.
	if len(j.log) >= 400 {
		j.log = j.log[len(j.log)-399:]
	}
	j.log = append(j.log, line)
	j.mu.Unlock()
}

// Runner owns the render jobs. Only one runs at a time: ffmpeg already uses the
// machine's cores, and two concurrent encodes would just fight each other.
type Runner struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	running bool
}

// NewRunner builds an empty runner.
func NewRunner() *Runner {
	return &Runner{jobs: make(map[string]*Job)}
}

// Get returns a job by id.
func (r *Runner) Get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	return j, ok
}

// List returns every job, newest first.
func (r *Runner) List() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Status, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, j.Snapshot())
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Started.After(out[k].Started) })
	return out
}

// ErrBusy is returned when a render is already in flight.
var ErrBusy = errors.New("a render is already running")

// Start queues a plan and runs it in the background.
func (r *Runner) Start(plan Plan) (*Job, error) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil, ErrBusy
	}
	job := &Job{status: Status{
		ID:      strconv.FormatInt(time.Now().UnixNano(), 36),
		State:   StateQueued,
		Started: time.Now().UTC(),
	}}
	r.jobs[job.status.ID] = job
	r.running = true
	r.mu.Unlock()

	go func() {
		err := r.run(job, plan)
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()

		job.update(func(j *Status) {
			j.Finished = time.Now().UTC()
			if err != nil {
				j.State = StateFailed
				j.Error = err.Error()
				return
			}
			j.State = StateDone
			j.Progress = 1
			j.Step = "listo"
			j.Output = plan.Output
		})
	}()
	return job, nil
}

func (r *Runner) run(job *Job, plan Plan) error {
	if err := os.MkdirAll(plan.WorkDir, 0o755); err != nil {
		return err
	}
	job.update(func(j *Status) { j.State = StateRunning })
	if plan.Prepare != nil {
		takes, err := plan.Prepare(func(step string) {
			job.update(func(j *Status) { j.Step = step })
		})
		if err != nil {
			return err
		}
		plan.Takes = takes
	}
	if len(plan.Takes) == 0 {
		return errors.New("the plan has no takes")
	}

	if plan.Loudness != 0 {
		job.update(func(j *Status) { j.Step = "midiendo el volumen" })
		if err := level(plan.Takes, plan.Loudness); err != nil {
			return err
		}
	}

	total := plan.Total()
	done := 0.0
	pieces := make([]string, 0, len(plan.Takes))

	for i, take := range plan.Takes {
		piece := filepath.Join(plan.WorkDir, fmt.Sprintf("piece-%02d.mov", i+1))
		pieces = append(pieces, piece)

		step := fmt.Sprintf("toma %d de %d", i+1, len(plan.Takes))
		job.update(func(j *Status) { j.Step = step })

		args := BuildArgs(take)
		if args.Empty {
			pieces = pieces[:len(pieces)-1]
			continue
		}
		cmd := args.CommandLine(piece)
		base := done
		err := runFFmpeg(job, cmd, func(seconds float64) {
			if total <= 0 {
				return
			}
			p := (base + seconds) / total
			if p > 1 {
				p = 1
			}
			job.update(func(j *Status) { j.Progress = p })
		})
		if err != nil {
			return fmt.Errorf("take %d: %w", i+1, err)
		}
		for _, s := range keptSpans(take) {
			done += s
		}
	}

	// Even one piece goes through the join: its sound is PCM, which is
	// encoded to AAC only here, once, for the whole video.
	job.update(func(j *Status) { j.Step = "empalmando" })
	if err := concat(job, pieces, plan.Output); err != nil {
		return err
	}

	if plan.Music != nil {
		job.update(func(j *Status) { j.Step = "mezclando la música" })
		if err := addMusic(job, plan.Output, *plan.Music, plan.Loudness); err != nil {
			return fmt.Errorf("music: %w", err)
		}
	}

	if d, err := ProbeDuration(plan.Output); err == nil {
		job.update(func(j *Status) { j.Duration = d })
	}
	return nil
}

// keptSpans is the rendered length of each surviving stretch of a take.
func keptSpans(t TakePlan) []float64 {
	var out []float64
	for _, s := range project.KeptSpans(t.Cuts, t.Duration) {
		out = append(out, s.Length())
	}
	return out
}

func concat(job *Job, pieces []string, output string) error {
	list := filepath.Join(filepath.Dir(pieces[0]), "pieces.txt")
	var b strings.Builder
	for _, p := range pieces {
		// The list is read relative to its own directory.
		fmt.Fprintf(&b, "file '%s'\n", filepath.Base(p))
	}
	if err := os.WriteFile(list, []byte(b.String()), 0o644); err != nil {
		return err
	}
	cmd := []string{
		"ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-i", list,
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", output,
	}
	return runFFmpeg(job, cmd, nil)
}

// runFFmpeg executes a command, forwarding stderr into the job log and parsing
// the -progress stream on stdout so callers can report how far along it is.
func runFFmpeg(job *Job, argv []string, onProgress func(seconds float64)) error {
	cmd := exec.Command(argv[0], argv[1:]...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	var stdout io.ReadCloser
	if onProgress != nil {
		if stdout, err = cmd.StdoutPipe(); err != nil {
			return err
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			job.appendLog(sc.Text())
		}
	}()

	if stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(stdout)
			for sc.Scan() {
				line := sc.Text()
				if v, ok := strings.CutPrefix(line, "out_time_us="); ok {
					if us, err := strconv.ParseFloat(v, 64); err == nil && us >= 0 {
						onProgress(us / 1e6)
					}
				}
			}
		}()
	}

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		tail := job.Log()
		if n := len(tail); n > 6 {
			tail = tail[n-6:]
		}
		return fmt.Errorf("ffmpeg failed: %w: %s", err, strings.Join(tail, " / "))
	}
	return nil
}

// ProbeDuration asks ffprobe how long a file really is. The browser's own
// metadata for a MediaRecorder capture is frequently missing or wrong, so the
// file is the only trustworthy source.
// ProbeDuration is how long a recording runs, in seconds.
//
// MediaRecorder writes WebM as a live stream: the header carries no Duration,
// so the container answers N/A and the cheap probe fails. Remux first and this
// never happens; the packet walk is the safety net for a file that reached
// disk some other way. A zero duration is the worst possible answer — every
// derived number in the studio divides by it — so it is an error, never a
// value.
func ProbeDuration(path string) (float64, error) {
	if d, err := probeFormat(path); err == nil && d > 0 {
		return d, nil
	}
	d, err := probePackets(path)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("probing %s: the file declares no duration and holds no packets", filepath.Base(path))
	}
	return d, nil
}

func probeFormat(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=nokey=1:noprint_wrappers=1", path).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// probePackets demuxes the file and takes the end of the last packet on any
// stream. It reads the whole container, so it is slow on a long take — which
// is why it only runs when the header has already refused to answer.
func probePackets(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "packet=pts_time,duration_time",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, fmt.Errorf("probing %s: %w", filepath.Base(path), err)
	}
	end := 0.0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		pts, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		if len(fields) > 1 {
			if d, err := strconv.ParseFloat(fields[1], 64); err == nil {
				pts += d
			}
		}
		if pts > end {
			end = pts
		}
	}
	return end, nil
}

// ebmlMagic opens every Matroska and WebM file ever written.
var ebmlMagic = []byte{0x1a, 0x45, 0xdf, 0xa3}

// salvageWindow bounds the search for a displaced header. A recorder that
// misbehaves puts a chunk or two in front of the header, not megabytes, and
// scanning the whole file would eventually match these four bytes inside the
// compressed video by chance.
const salvageWindow = 16 << 20

// Salvage rescues a recording whose header is not at the start of the file.
//
// A browser that hands over its chunks out of order — or a stray chunk from a
// recording that had not finished when the next one began — produces a file
// with a foreign block in front of the EBML header. Nothing will open it:
// ffprobe refuses it, the montage draws nothing, the render skips it. But the
// recording itself is usually intact from the header onwards, and half an hour
// of reading is worth recovering even in part.
//
// It reports whether it changed anything, and it only changes anything when
// what it produced actually probes. A file it cannot improve is left exactly
// as it was.
func Salvage(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	head := make([]byte, salvageWindow)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}
	head = head[:n]

	if bytes.HasPrefix(head, ebmlMagic) {
		return false, nil
	}
	offset := bytes.Index(head, ebmlMagic)
	if offset <= 0 {
		return false, fmt.Errorf("salvaging %s: no EBML header in the first %d bytes", filepath.Base(path), len(head))
	}

	tmp := path + ".salvage" + filepath.Ext(path)
	out, err := os.Create(tmp)
	if err != nil {
		return false, err
	}
	if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
		out.Close()
		os.Remove(tmp)
		return false, err
	}
	_, copyErr := io.Copy(out, f)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmp)
		return false, errors.Join(copyErr, closeErr)
	}

	// Only keep the salvaged file if it is actually readable. Guessing at an
	// offset and then trusting the guess would turn a damaged take into a
	// destroyed one.
	if err := Remux(tmp); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("salvaging %s: what followed the header did not remux: %w", filepath.Base(path), err)
	}
	if d, err := probeFormat(tmp); err != nil || d <= 0 {
		os.Remove(tmp)
		return false, fmt.Errorf("salvaging %s: what followed the header has no duration", filepath.Base(path))
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, err
	}
	return true, nil
}

// Remux rewrites a recording's container without touching the streams, so the
// file gains the Duration and the seek index MediaRecorder never wrote.
//
// This is not a nicety. Chrome cannot seek inside a live-muxed WebM — the
// element reports an Infinite duration and scrubbing goes nowhere — and
// ffprobe cannot read its length either. Both start working the moment the
// file has been through here, and the streams are copied, so it costs a disk
// pass and changes not a single sample.
func Remux(path string) error {
	tmp := path + ".remux" + filepath.Ext(path)
	out, err := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-i", path, "-c", "copy", tmp).CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("remuxing %s: %w: %s", filepath.Base(path), err, strings.TrimSpace(string(out)))
	}
	info, statErr := os.Stat(tmp)
	if statErr != nil || info.Size() == 0 {
		os.Remove(tmp)
		return fmt.Errorf("remuxing %s produced nothing", filepath.Base(path))
	}
	return os.Rename(tmp, path)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// level sets each take's gain so its speech sits at target LUFS. Every piece
// cut from one recording gets the same gain, measured over the whole
// recording, so a take split around a figure does not change level mid-way.
func level(takes []TakePlan, target float64) error {
	measured := map[string]float64{}
	for i := range takes {
		t := &takes[i]
		if t.Silent || t.Source == "" {
			continue
		}
		lufs, ok := measured[t.Source]
		if !ok {
			var err error
			if lufs, err = Loudness(t.Source); err != nil {
				return fmt.Errorf("measuring %s: %w", filepath.Base(t.Source), err)
			}
			measured[t.Source] = lufs
		}
		// Nothing measurable: a take of silence, or too short to gate.
		if math.IsInf(lufs, 0) || math.IsNaN(lufs) || lufs < -70 {
			continue
		}
		t.Level = math.Max(-20, math.Min(25, target-lufs))
	}
	return nil
}

// Loudness is a recording's integrated loudness (EBU R128), in LUFS.
func Loudness(path string) (float64, error) {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-nostats", "-i", path,
		"-vn", "-af", "ebur128=framelog=quiet", "-f", "null", "-").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("%w: %s", err, lastLine(string(out)))
	}
	return parseIntegrated(string(out))
}

// parseIntegrated reads «I: -18.5 LUFS» from the summary ebur128 prints last.
func parseIntegrated(log string) (float64, error) {
	summary := log
	if i := strings.LastIndex(log, "Integrated loudness:"); i >= 0 {
		summary = log[i:]
	}
	for _, line := range strings.Split(summary, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "I:" {
			return strconv.ParseFloat(f[1], 64)
		}
	}
	return 0, errors.New("ffmpeg printed no integrated loudness")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
