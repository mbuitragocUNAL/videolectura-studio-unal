# =========================
# Frontend stage
# =========================
FROM node:24-alpine AS web

RUN corepack enable

WORKDIR /web

# Cache the dependency step: the manifests change far less often than the app.
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./
# ssr:false, so this emits a static bundle the Go binary can serve on its own.
RUN pnpm generate

# =========================
# Build stage
# =========================
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

# GOARCH is deliberately left to the build platform instead of being pinned to
# amd64: on Apple Silicon an emulated ffmpeg encodes several times slower, and
# encoding is the whole point of this image.
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
  go build -trimpath -ldflags="-s -w" \
  -o studio ./cmd/studio

# =========================
# Runtime stage
# =========================
FROM python:3.13-slim

# ffmpeg does the rendering; ffprobe reads the real duration of a take, which
# the browser reports unreliably. Python with PyMuPDF runs preparar/lector.py,
# which the console's «Preparar» page calls to read the book. Debian rather
# than Alpine because PyMuPDF only publishes glibc wheels for every platform.
RUN apt-get update \
  && apt-get install -y --no-install-recommends ffmpeg tzdata ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && ln -sf /usr/share/zoneinfo/America/Bogota /etc/localtime \
  && echo "America/Bogota" > /etc/timezone
COPY preparar/requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir -r /tmp/requirements.txt
# Whisper, to hear where in a take each figure is named. The model is fetched
# on first use into HF_HOME, which compose keeps in a volume.
COPY preparar/requirements-alinear.txt /tmp/requirements-alinear.txt
RUN pip install --no-cache-dir -r /tmp/requirements-alinear.txt
# Qwen3-TTS for the quotes' voices, only when asked for
# (VOZ_LOCAL=1 docker compose build): it adds PyTorch, and on a CPU it is slow.
# On a Mac, ./estudio voces runs it on the GPU instead.
ARG VOZ_LOCAL=0
COPY preparar/requirements-voz.txt /tmp/requirements-voz.txt
RUN if [ "$VOZ_LOCAL" = "1" ]; then \
      pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu \
      && pip install --no-cache-dir -r /tmp/requirements-voz.txt; \
    fi

ENV TZ=America/Bogota

WORKDIR /app

COPY --from=builder /app/studio /app/studio
COPY --from=web /web/.output/public /app/web
COPY preparar/lector.py preparar/componer.py preparar/alinear.py preparar/voz.py /app/preparar/

ENV STUDIO_ADDR=:8080 \
    STUDIO_WEB_DIR=/app/web \
    STUDIO_LECTURAS_DIR=/estudio/lecturas \
    STUDIO_LIBROS_DIR=/estudio/libros \
    STUDIO_PYTHON=python3 \
    STUDIO_LECTOR=/app/preparar/lector.py \
    STUDIO_ALINEAR=/app/preparar/alinear.py \
    STUDIO_VOZ=/app/preparar/voz.py \
    STUDIO_VOCES_DIR=/estudio/voces \
    STUDIO_MUSICA_DIR=/estudio/musica \
    STUDIO_WHISPER_MODEL=small \
    HF_HOME=/cache/huggingface

EXPOSE 8080

CMD ["/app/studio"]
