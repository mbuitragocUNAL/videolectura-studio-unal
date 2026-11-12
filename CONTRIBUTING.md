# Colaborar con Videolectura Studio

Este estudio nació para una lectura del curso y creció con cada corrección del
profesor. **Está abierto a todos los del curso** (y a quien le sirva): úsalo,
rómpelo, cuéntanos qué falló y llévalo a donde se te ocurra. No hace falta saber
Go ni Vue para aportar: un buen reporte de un problema, una lectura probada con
otro libro o una idea bien explicada valen tanto como el código.

## Formas de aportar

- **Usarlo y contar cómo te fue.** Abre un [issue](https://github.com/mbuitragocUNAL/videolectura-studio-unal/issues) con lo que funcionó y lo que no. Di qué libro y qué páginas, en qué sistema, y qué esperabas que pasara.
- **Reportar un problema.** Lo más útil: los pasos para que se repita, una captura, y lo que dice `docker logs estoc-studio`.
- **Proponer una idea.** Abre un issue que empiece por «Idea:». Cuenta el problema que resuelve antes que la solución.
- **Mejorar la documentación.** Si algo de `docs/` no fue claro, cámbialo: es Markdown.
- **Escribir código.** Ver abajo.

## Ideas para empezar

Cosas que harían falta y nadie ha hecho todavía:

- **Notas al pie (2.h).** Mostrar la nota sola en pantalla cuando el texto llega a su llamada, como se hace con las figuras.
- **Otros libros y cursos.** Probar con el libro de Lenguajes de Programación y otros PDF: dónde falla la detección de párrafos, figuras o secciones.
- **Linux y Windows.** Está probado en macOS. Hace falta probar el flujo completo en Linux y en Windows con WSL, y documentarlo.
- **Qwen3-TTS más rápido en el contenedor**, o un motor local liviano para quien no tiene un Mac.
- **Un editor de tiempos de las figuras**, para mover a mano el instante en que aparece una figura cuando la transcripción se equivoca.
- **Grabar varias tomas de un párrafo y elegir la mejor** desde el montaje, en vez de quedarse siempre con la última.
- **Música por secciones**, o una pista distinta para cada parte del capítulo.
- **Exportar subtítulos** (`.srt`) a partir de las transcripciones que el render ya hace.
- **Más videos de la guía** (`guia/`): hechos con HyperFrames, se arman desde un guion en Markdown.

## Para desarrollar

Todo corre en Docker, pero para trabajar en el código conviene tener:

- Go 1.24+ (el servidor y el render: `cmd/`, `internal/`, sin dependencias externas)
- Node 22+ y pnpm (la consola: `frontend/`, Nuxt 4 + Nuxt UI)
- Python 3.12+ con `preparar/requirements.txt` (la preparación: `preparar/`)
- ffmpeg

```bash
go test ./...                                    # servidor y render
(cd frontend && pnpm install && pnpm test && pnpm typecheck)
python3 -m venv preparar/.venv
preparar/.venv/bin/python -m pip install -r preparar/requirements.txt
preparar/.venv/bin/python -m unittest preparar/test_componer.py preparar/test_voz.py
```

Para ver los cambios de la consola en vivo, con el estudio abierto:
`docker compose --profile dev up frontend` y abre <http://localhost:3300>.

[`docs/manual.md`](docs/manual.md) explica por qué cada parte es como es (por
qué una toma es un párrafo, cómo se alinean las figuras con la voz, cómo se
compone el texto). Léelo antes de cambiar algo grande.

## Cómo enviar un cambio

1. Haz un *fork* y una rama con un nombre que diga qué hace (`notas-al-pie`, `arreglo-windows`).
2. Cambios pequeños y enfocados; si tocas el comportamiento, agrega o ajusta una prueba.
3. Que pasen las pruebas de arriba.
4. Abre un *pull request* contando qué cambia, por qué, y cómo lo probaste (con qué lectura).

## Algunas reglas

- **Nada con derechos de autor en el repositorio:** ni PDF de libros, ni tomas, ni música, ni capturas con páginas del libro. El `.gitignore` ya los deja fuera; revisa antes de hacer *commit*.
- **Nada de claves:** la de ElevenLabs vive en `voces/claves.json`, fuera de git.
- **Los lineamientos del curso mandan.** Si un cambio afecta cómo queda el video, cita el numeral (2.d, 2.f…) que cumple.
- El código y los comentarios del servidor están en inglés; la interfaz, la documentación y los mensajes al usuario, en español.

Gracias por aportar.
