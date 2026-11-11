// Command studio serves the recording console, the presentation window and the
// editor for a reading, and renders the finished video.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"studio/internal/api"
	"studio/internal/project"
)

func main() {
	cfg := api.HubConfig{}
	var addr string

	flag.StringVar(&addr, "addr", envOr("STUDIO_ADDR", ":8080"), "address to listen on")
	flag.StringVar(&cfg.ReadingDir, "reading", envOr("STUDIO_READING_DIR", ""), "reading to open (holds marcas.json); empty opens the first prepared one under -lecturas")
	flag.StringVar(&cfg.DataDir, "data", envOr("STUDIO_DATA_DIR", ""), "directory for takes, session and output; empty is the reading's estudio/")
	flag.StringVar(&cfg.WebDir, "web", envOr("STUDIO_WEB_DIR", "/app/web"), "directory holding the user interface")
	flag.IntVar(&cfg.Height, "height", 1080, "render height in pixels")
	flag.Float64Var(&cfg.Gain, "gain", 1.0, "quote volume relative to the narration")
	flag.Float64Var(&cfg.Loudness, "loudness", -16, "LUFS every take is levelled to in the render; 0 leaves them as recorded")
	flag.StringVar(&cfg.LecturasDir, "lecturas", envOr("STUDIO_LECTURAS_DIR", "lecturas"), "directory with one folder per reading")
	flag.StringVar(&cfg.LibrosDir, "libros", envOr("STUDIO_LIBROS_DIR", "libros"), "directory holding the books' PDFs")
	flag.StringVar(&cfg.Python, "python", envOr("STUDIO_PYTHON", "python3"), "Python interpreter with PyMuPDF")
	flag.StringVar(&cfg.Lector, "lector", envOr("STUDIO_LECTOR", "preparar/lector.py"), "path to preparar/lector.py")
	flag.StringVar(&cfg.Voz, "voz", envOr("STUDIO_VOZ", "preparar/voz.py"), "path to preparar/voz.py, which makes the quotes' synthetic voices")
	flag.StringVar(&cfg.MusicDir, "musica", envOr("STUDIO_MUSICA_DIR", "musica"), "directory with the background tracks readings may use")
	flag.StringVar(&cfg.VocesDir, "voces", envOr("STUDIO_VOCES_DIR", "voces"), "directory for designed voices, local voices and the ElevenLabs key")
	pausa := flag.Float64("pausa", project.DefaultPauses.MaxPause,
		"seconds a pause inside a take may last before the render shortens it; 0 leaves takes untouched")
	alinear := flag.String("alinear", envOr("STUDIO_ALINEAR", "preparar/alinear.py"),
		"path to preparar/alinear.py, which transcribes takes to time the figures; empty to estimate instead")
	flag.Parse()
	if *pausa > 0 {
		cfg.Pauses = project.DefaultPauses
		cfg.Pauses.MaxPause = *pausa
	}
	if *alinear != "" {
		cfg.Aligner = []string{cfg.Python, *alinear}
	}

	server, err := api.NewHub(cfg)
	if err != nil {
		log.Fatalf("studio: %v", err)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// Uploading a long take is slow and must not be cut off, so the write
		// and read timeouts stay open; the header timeout guards the handshake.
		IdleTimeout: 2 * time.Minute,
	}

	go func() {
		fmt.Printf("studio listening on %s\n  reading: %s\n  data:    %s\n", addr, cfg.ReadingDir, cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("studio: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("studio: shutdown: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
