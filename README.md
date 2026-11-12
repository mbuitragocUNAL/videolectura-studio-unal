# Videolectura Studio UNAL

Un estudio para grabar las **video-lecturas** del curso de Modelos Estocásticos
y Simulación (Universidad Nacional de Colombia). Tú lees, párrafo por párrafo;
el estudio arma el video y se encarga de que cumpla los lineamientos 3.2.3:
figuras con su pie, figura fija a la derecha, flecha de regreso, citas con voz
sintética, música de fondo y el formato de entrega.

> Hecho por estudiantes para el curso. **No es una herramienta oficial de la
> Universidad Nacional.** Está abierto: úsalo, mejóralo y llévalo más lejos
> ([cómo colaborar](CONTRIBUTING.md)).

[![Guía en video: Videolectura Studio UNAL](https://img.youtube.com/vi/NcUtmzC_DAw/maxresdefault.jpg)](https://youtu.be/NcUtmzC_DAw)

**Guía general (2:37):** qué es y el recorrido completo. Debajo, un video corto y una página para cada paso;
los siete están en [la lista de reproducción](https://www.youtube.com/playlist?list=PLWAtzq5ujf-c).

## Empezar en un minuto

```bash
git clone https://github.com/mbuitragocUNAL/videolectura-studio-unal
cd videolectura-studio-unal
./estudio abrir
```

Abre <http://localhost:8100> en Chrome. Necesitas **Docker** y **Google Chrome**;
el resto corre en el contenedor.

## Paso a paso

### 1 · Instalar y abrir

[![Paso 1: instalar y abrir](https://img.youtube.com/vi/63AtZbNCydY/maxresdefault.jpg)](https://youtu.be/63AtZbNCydY)

Docker, Chrome, unos 6 GB y el PDF del libro con texto seleccionable. Tres
comandos y el estudio queda en `localhost:8100`; la primera vez te lleva a
crear tu lectura. `./estudio cerrar` lo apaga; `git pull` y `./estudio abrir` lo actualizan.
**[Instalar, paso a paso →](docs/1-instalar.md)**

### 2 · Preparar la lectura

[![Paso 2: preparar la lectura](https://img.youtube.com/vi/jfDIFUssoig/maxresdefault.jpg)](https://youtu.be/jfDIFUssoig)

Eliges el libro y el capítulo desde su índice, marcas con un clic dónde empieza
y termina tu parte, revisas las figuras y láminas que encontró el estudio,
agregas las citas y los datos del video. **Guardar y preparar** compone el
pasaje con la letra del libro, listo para grabar.
**[Preparar, paso a paso →](docs/2-preparar.md)**

### 3 · Voces y música

[![Paso 3: voces y música](https://img.youtube.com/vi/A0JK6u84_pw/maxresdefault.jpg)](https://youtu.be/A0JK6u84_pw)

Cada personaje citado recibe una voz sintética (2.i): **ElevenLabs** con tu clave,
**Qwen3-TTS** libre en tu computador, o un audio tuyo. **Diseñar** propone tres
voces a partir de la descripción; **Generar** hace todos los clips. La música de
fondo (2.j) es opcional y queda 14 dB bajo tu voz.
**[Voces y música, paso a paso →](docs/3-voces-y-musica.md)**

### 4 · Grabar

[![Paso 4: grabar](https://img.youtube.com/vi/Yi8kU5zuZdI/maxresdefault.jpg)](https://youtu.be/Yi8kU5zuZdI)

Una toma es un párrafo: `R` graba, `S` para, `Av Pág` al siguiente. Si te
equivocas, lo grabas otra vez. Cuando nombras una figura por primera vez, lees
su pie (y lo que diga la imagen) en la misma toma; en las citas pulsas `N` y
callas mientras suenan.
**[Grabar, paso a paso →](docs/4-grabar.md)**

### 5 · Montar y renderizar

[![Paso 5: montar y renderizar](https://img.youtube.com/vi/7YBjXaY3VPY/maxresdefault.jpg)](https://youtu.be/7YBjXaY3VPY)

El montaje es opcional: recorta tropiezos sin regrabar. **Montar el video**
escucha cada toma, recorta pausas, iguala el volumen y compone la imagen: el
texto corre con tu voz, las figuras aparecen con su pie al leerlo y vuelven con
flecha. Sale `final.mp4` en 1080p.
**[Montar y renderizar, paso a paso →](docs/5-montar-y-renderizar.md)**

### 6 · Entregar

[![Paso 6: entregar](https://img.youtube.com/vi/j1wxLYfk07M/maxresdefault.jpg)](https://youtu.be/j1wxLYfk07M)

YouTube con el título del curso, no dirigido a niños, formato estándar; los
créditos de la voz y la música en la descripción; una miniatura si quieres; y el
correo solo al profesor, dentro del plazo.
**[Entregar, paso a paso →](docs/6-entregar.md)**

## Qué hace, lineamiento por lineamiento

| Lineamiento | Qué hace el estudio |
|---|---|
| 2.c La página llena la pantalla | Vuelve a componer el pasaje con la letra del propio libro (extraída del PDF), al mismo tamaño, ocupando el 94 % del ancho. Sin márgenes ni cortes de página. |
| — | El texto **corre con tu voz**: el estudio escucha cada toma (Whisper) y mantiene la línea que lees siempre a la misma altura. |
| 2.d Figura con su pie, la primera vez | La figura aparece sola con su pie **justo cuando empiezas a leerlo**, se queda 2 s y al volver una **flecha** marca dónde sigue la lectura. |
| 2.e Las menciones siguientes | La figura sin pie, 2 s. |
| 2.f Figura fija a la derecha | Mientras se habla de una figura, queda a la derecha y el texto **conserva su tamaño**: se reacomoda en más líneas. |
| 2.i Citas con voz sintética | Cada personaje tiene su voz. La cita suena por los audífonos cuando pulsas `N`, y el render la mezcla. |
| 2.j Música de fondo | Una pista bajo toda la lectura, 14 dB por debajo de la voz, que baja un poco cuando hablas. |
| 2.k y 4.3 La entrega | Video 1080p, título en el formato del curso, miniatura (`preparar/miniatura.py`). |

Además: recorta los silencios largos, deja todas las tomas al mismo volumen, y
la toma nueva de un párrafo reemplaza a la anterior.

## Requisitos

- **Docker** (Docker Desktop en macOS o Windows).
- **Google Chrome** para grabar.
- Unos 6 GB libres.
- El **PDF del libro**, con capa de texto (un libro escaneado como imagen no sirve).
- Audífonos.
- Opcional: una clave de **ElevenLabs**, o un Mac con Apple Silicon para **Qwen3-TTS** en local.

Probado en macOS (Apple Silicon). En Windows, usa WSL o Git Bash para `./estudio`.

## Colabora

El estudio es de todos los que lo usen. Cuéntanos cómo te fue, reporta lo que
falle, propone ideas o mándanos código: notas al pie (2.h), soporte probado en
Linux y Windows, un editor de tiempos de las figuras, subtítulos… Hay una lista
de ideas para empezar y las instrucciones para desarrollar en
**[CONTRIBUTING.md](CONTRIBUTING.md)**. Las dudas y los problemas, en
[Issues](https://github.com/mbuitragocUNAL/videolectura-studio-unal/issues).

## Lo que no va al repositorio

Los libros (`libros/`), tus tomas y renders (`lecturas/*/estudio/`), las voces y
la clave de ElevenLabs (`voces/`), la música (`musica/`) y todo lo que se genera
al preparar. De cada lectura solo se versiona su `lectura.toml`.

## Para saber más

- [`docs/`](docs/): una página por paso, y [`docs/manual.md`](docs/manual.md), el manual técnico con el porqué de cada decisión.
- [`guia/`](guia/): los videos de la guía, hechos con [HyperFrames](https://github.com/heygen-com/hyperframes) y narrados con voz sintética (ElevenLabs en los videos de cada paso, Qwen3-TTS en la guía general) a partir de un guion en Markdown; `guia/narrar.py` sirve con ambos.

## Estructura

```
cmd/studio/        el servidor (Go, sin dependencias externas)
internal/          lectura, sesión, cortes, alineación, render con ffmpeg
frontend/          la consola (Nuxt 4 + Nuxt UI)
preparar/          lector.py (PDF → lectura), componer.py (texto compuesto),
                   alinear.py (Whisper), voz.py (voces), miniatura.py
lecturas/          una carpeta por lectura; solo lectura.toml va a git
guia/              los videos de la guía
docs/              una página por paso y el manual técnico
estudio            el atajo: nueva, preparar, abrir, cerrar, voces
```

## Créditos y licencia

Código bajo licencia MIT (ver `LICENSE`). Usa
[PyMuPDF](https://pymupdf.readthedocs.io) (AGPL-3.0) para leer y componer el PDF,
[faster-whisper](https://github.com/SYSTRAN/faster-whisper) para escuchar las tomas,
[Qwen3-TTS](https://github.com/QwenLM/Qwen3-TTS) y ElevenLabs para las voces,
ffmpeg para el video, e IBM Plex Sans (SIL OFL) en la miniatura.
