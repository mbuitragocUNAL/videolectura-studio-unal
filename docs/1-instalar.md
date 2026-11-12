# 1 · Instalar y abrir

[Video de este paso](https://youtu.be/63AtZbNCydY) · [Siguiente: preparar la lectura →](2-preparar.md)

## Lo que necesitas

| | |
|---|---|
| **Docker** | Docker Desktop en macOS o Windows, Docker Engine en Linux. Todo el estudio corre en un contenedor: no hay que instalar Go, Node, Python ni ffmpeg. |
| **Google Chrome** | Para grabar. La consola captura su propia pestaña de presentación, y eso solo funciona bien en Chrome. |
| **Espacio** | Unos 6 GB: la imagen (~3 GB), el modelo de Whisper que se descarga en el primer render (~500 MB) y tus tomas. |
| **El PDF del libro** | Con capa de texto: si puedes seleccionar el texto en un visor de PDF, sirve. Un libro escaneado como imagen no. |
| **Audífonos** | Las citas con voz sintética suenan por ellos mientras grabas, sin que el micrófono las capte. |

Opcional, para las voces de las citas: una clave de **ElevenLabs**, o un Mac con
Apple Silicon para generarlas en local con **Qwen3-TTS** (ver
[Voces y música](3-voces-y-musica.md)).

## Instalar

```bash
git clone https://github.com/mbuitragocUNAL/videolectura-studio-unal
cd videolectura-studio-unal
./estudio abrir
```

La primera vez construye la imagen: tarda unos minutos. Las siguientes, segundos.
Cuando termina abre **<http://localhost:8100>** en Chrome.

> **Windows:** `./estudio` es un script de bash. Córrelo desde WSL (Ubuntu) o
> Git Bash, con Docker Desktop abierto.

## La primera vez

No hay lecturas todavía: el estudio te lleva a **Lecturas**, que dice «Todavía
no hay lecturas. Empieza con «Nueva lectura»». Ese es el paso siguiente.

## Comandos del día a día

| Comando | |
|---|---|
| `./estudio abrir` | Abre el estudio con la primera lectura preparada (o ninguna). |
| `./estudio abrir lecturas/22-…` | Abre el estudio en esa lectura. |
| `./estudio cerrar` | Apaga el contenedor. Tus lecturas y tomas quedan en `lecturas/`. |
| `./estudio preparar lecturas/22-…` | Vuelve a preparar una lectura después de editar su `lectura.toml` a mano. |
| `./estudio voces …` | Las voces de las citas desde la terminal (ver [Voces y música](3-voces-y-musica.md)). |

Para **actualizar** a la última versión: `git pull` y `./estudio abrir` (reconstruye la imagen).

## Dónde queda cada cosa

```
libros/                  los PDF que subas (fuera de git)
lecturas/22-…/           una carpeta por lectura
  lectura.toml           lo que elegiste al preparar: lo único que va a git
  estudio/takes/         tus tomas
  estudio/output/        el video final y los pedazos del render
voces/                   voces diseñadas y la clave de ElevenLabs (fuera de git)
musica/                  pistas de fondo (fuera de git)
```

## Si algo falla

- **«Cannot connect to the Docker daemon»**: abre Docker Desktop y espera a que diga *running*.
- **El puerto 8100 está ocupado**: otro programa lo usa. Ciérralo, o cambia `127.0.0.1:8100` en `docker-compose.yml`.
- **La página no carga después de actualizar**: recarga sin caché (`Cmd/Ctrl + Shift + R`).
- **Ver qué pasa por dentro**: `docker logs estoc-studio`.
