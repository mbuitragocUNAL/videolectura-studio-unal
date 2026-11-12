import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from hf import Video  # noqa: E402

AQUI = Path(__file__).resolve().parent
v = Video(AQUI, 2, "Videolectura Studio · Preparar la lectura")
pasos = ["Libro", "Páginas y rango", "Figuras y láminas", "Citas", "Datos del video"]
v.portada(1, "Preparar la lectura", "Qué parte del libro vas a leer.")
v.captura(2, "PREPARAR", "El libro", [("nueva-estante", [(0, 960, 540, 1244 / 1920), ("estante", 420, 200, 1.6)])],
          lado=("pasos", [("", t, p) for t, p in zip(pasos, [0, None, None, None, None])]))
v.captura(3, "PREPARAR", "Páginas y rango", [("preparar-libro", [(0, 410, 260, 1.5), ("empieza", 400, 720, 1.45),
                                                                   ("lavado", 960, 540, 1244 / 1920)])],
          lado=("pasos", [("", t, p) for t, p in zip(pasos, [None, 0, None, None, None])]))
v.captura(4, "PREPARAR", "Figuras y láminas", [("preparar-figuras", [(0, 450, 380, 1.35), ("fijas", 450, 860, 1.35)])],
          lado=("pasos", [("", t, p) for t, p in zip(pasos, [None, None, 0, None, None])]))
v.captura(5, "PREPARAR", "Citas", [("preparar-texto", [(0, 700, 450, 1.0)]), ("preparar-citas", [("personaje", 390, 400, 1.55)])],
          lado=("pasos", [("", t, p) for t, p in zip(pasos, [None, None, None, 0, None])]))
v.captura(6, "PREPARAR", "Datos del video", [("preparar-datos", [(0, 390, 330, 1.55), ("estima", 390, 420, 1.55)])],
          lado=("pasos", [("", t, p) for t, p in zip(pasos, [None, None, None, None, 0])]),
          nota=("Guardar y preparar: la lectura queda lista para grabar.", "Guardas"))
v.cierre(7, "SIGUIENTE · VOCES Y MÚSICA", "docs/2-preparar.md")
v.escribir()
