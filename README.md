# Videolectura Studio UNAL

Un estudio para grabar las **video-lecturas** del curso de Modelos Estocásticos
y Simulación (Universidad Nacional de Colombia). Tú lees, párrafo por párrafo;
el estudio arma el video y se encarga de que cumpla los lineamientos 3.2.3:
figuras con su pie, figura fija a la derecha, flecha de regreso, citas con voz
sintética, música de fondo y el formato de entrega.

> Hecho por estudiantes para el curso. **No es una herramienta oficial de la
> Universidad Nacional.**

**Guía en video:** [cómo grabar una lectura con Videolectura Studio](https://youtu.be/NcUtmzC_DAw) (2:37)

## Qué hace

| Lineamiento | Qué hace el estudio |
|---|---|
| 2.c La página llena la pantalla | Vuelve a componer el pasaje con la letra del propio libro (extraída del PDF), al mismo tamaño, ocupando el 94 % del ancho. Sin márgenes ni cortes de página. |
| — | El texto **corre con tu voz**: el estudio escucha cada toma (Whisper) y mantiene la línea que lees siempre a la misma altura. |
| 2.d Figura con su pie, la primera vez | La figura aparece sola con su pie **justo cuando empiezas a leerlo**, se queda 2 s y al volver una **flecha** marca dónde sigue la lectura. |
| 2.e Las menciones siguientes | La figura sin pie, 2 s. |
| 2.f Figura fija a la derecha | Mientras se habla de una figura, queda a la derecha y el texto **conserva su tamaño**: se reacomoda en más líneas. |
| 2.i Citas con voz sintética | Cada personaje tiene su voz: ElevenLabs (con tu clave), Qwen3-TTS en tu computador, o un audio tuyo. La cita suena por los audífonos cuando pulsas `N` y el render la mezcla. |
| 2.j Música de fondo | Una pista bajo toda la lectura, 14 dB por debajo de la voz, que baja un poco más cuando hablas. |
| 2.k y 4.3 La entrega | Video 1080p, título en el formato del curso, miniatura (`preparar/miniatura.py`). |

Además: recorta los silencios largos entre párrafos, deja todas las tomas al
mismo volumen, y cada párrafo es una toma: si te equivocas, lo grabas otra vez
y la toma nueva reemplaza a la anterior.

## Requisitos

- **Docker** (Docker Desktop en macOS o Windows).
- **Google Chrome** para grabar (la consola captura su propia pestaña).
- Unos 6 GB libres: la imagen, y el modelo de Whisper que se descarga en el primer render (~500 MB).
- El **PDF del libro**, con capa de texto (un libro escaneado como imagen no sirve).
- Audífonos, para oír las citas sin que el micrófono las capte.
- Opcional: una clave de **ElevenLabs** para las voces de las citas, o un Mac con Apple Silicon para generarlas en local con **Qwen3-TTS**.

Probado en macOS (Apple Silicon). En Windows, el atajo `./estudio` es un script
de bash: úsalo desde WSL o Git Bash.

## Empezar

```bash
git clone https://github.com/mbuitragocUNAL/videolectura-studio-unal
cd videolectura-studio-unal
./estudio abrir
```

Abre <http://localhost:8100>. La primera vez no hay lecturas: el estudio te
lleva a **Lecturas › Nueva lectura**.

## Grabar una lectura, en cuatro pasos

1. **Preparar.** Sube el PDF, elige el capítulo desde su índice y marca con un
   clic dónde empieza y dónde termina tu parte. El estudio encuentra las
   figuras y láminas, cuántas veces se nombran y en qué párrafos quedan fijas.
   Agrega las citas (selecciona el texto o usa una sugerencia), dale una voz a
   cada personaje y, si quieres, una música de fondo. **Guardar y preparar.**
2. **Grabar.** Abre la presentación, ponte los audífonos y graba un párrafo a la vez:

   | Tecla | |
   |---|---|
   | `R` / `S` | grabar / detener |
   | `Av Pág` / `Re Pág` | párrafo siguiente / anterior |
   | `N` | suena la cita de este párrafo |
   | `espacio` | correr o pausar el texto |

   Cuando un párrafo nombra una figura por primera vez, sigue leyendo hasta el
   punto y **lee su pie** (y lo que diga la imagen) en la misma toma.
3. **Montar y renderizar.** En el montaje puedes recortar tropiezos (el estudio
   propone relecturas y silencios largos). **Renderizar** deja el video en
   `lecturas/<tu-lectura>/estudio/output/final.mp4`.
4. **Entregar.** Súbelo a YouTube en 1080p, formato estándar, **no dirigido a
   niños**, con el título `ME0Z Lectura XX Título`. La miniatura:

   ```bash
   preparar/.venv/bin/python preparar/miniatura.py lecturas/22-… \
       --titulo "Belleza cuántica II" --destacado "exuberancia" \
       --linea "Diamante · grafeno · nanotubos · buckyballs" --figura figuras/figura-30.png
   ```

## Voces de las citas

En **Preparar › Citas**, cada personaje tiene una descripción («cómo suena:
edad, oficio, ánimo, acento») y un motor:

- **ElevenLabs**: agrega tu clave en el panel. Se guarda solo en `voces/claves.json`, en tu computador.
- **Qwen3-TTS (local)**: libre y sin cuenta. En el contenedor corre en CPU y es lento; en un Mac:

  ```bash
  python3 -m venv preparar/.venv
  preparar/.venv/bin/python -m pip install -r preparar/requirements.txt -r preparar/requirements-voz.txt
  ./estudio voces disenar lecturas/22-… --personaje "Electrón" --motor qwen
  ./estudio voces elegir  lecturas/22-… --personaje "Electrón" --candidato 2
  ./estudio voces generar lecturas/22-…
  ```
- **Audio propio**: el ícono de subir en cada cita.

**Diseñar** hace tres voces candidatas a partir de la descripción; escuchas y te quedas con una.

## Lo que no va al repositorio

Los libros (`libros/`), tus tomas y renders (`lecturas/*/estudio/`), las voces y
la clave de ElevenLabs (`voces/`), la música (`musica/`) y todo lo que se genera
al preparar. De cada lectura solo se versiona su `lectura.toml`.

## Para saber más

- [`docs/manual.md`](docs/manual.md): el manual técnico, con el porqué de cada decisión (cómo se graba, cómo se monta, las pausas, el texto compuesto, la alineación con Whisper, las voces, la música).
- [`guia/`](guia/): la guía en video, hecha con [HyperFrames](https://github.com/heygen-com/hyperframes) y narrada con Qwen3-TTS (`guion.md`, `narrar.py`, `construir.py`).

## Estructura

```
cmd/studio/        el servidor (Go, sin dependencias externas)
internal/          lectura, sesión, cortes, alineación, render con ffmpeg
frontend/          la consola (Nuxt 4 + Nuxt UI)
preparar/          lector.py (PDF → lectura), componer.py (texto compuesto),
                   alinear.py (Whisper), voz.py (voces), miniatura.py
lecturas/          una carpeta por lectura; solo lectura.toml va a git
guia/              la guía en video
docs/manual.md     el manual técnico
estudio            el atajo: nueva, preparar, abrir, cerrar, voces
```

## Créditos y licencia

Código bajo licencia MIT (ver `LICENSE`). Usa
[PyMuPDF](https://pymupdf.readthedocs.io) (AGPL-3.0) para leer y componer el PDF,
[faster-whisper](https://github.com/SYSTRAN/faster-whisper) para escuchar las tomas,
[Qwen3-TTS](https://github.com/QwenLM/Qwen3-TTS) y ElevenLabs para las voces,
ffmpeg para el video, e IBM Plex Sans (SIL OFL) en la miniatura.
