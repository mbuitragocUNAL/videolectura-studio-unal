import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from hf import Video  # noqa: E402

AQUI = Path(__file__).resolve().parent
v = Video(AQUI, 6, "Videolectura Studio · Entregar")
v.portada(1, "Entregar", "YouTube, descripción y correo.")
v.lista(2, "YOUTUBE · 4.3", "Al subirlo", [
    ("2.k", "Título: ME01 Lectura 22 Belleza cuántica II: exuberancia", "título"),
    ("NO", "No es contenido para niños", "niños"),
    ("", "Video estándar, no Short", "estándar"),
    ("HD", "1080p", "mil ochenta"),
])
v.lista(3, "LA DESCRIPCIÓN", "Lo que conviene poner", [
    ("", "El libro y la sección", "libro"),
    ("2.i", "Crédito de la voz sintética", "voz"),
    ("2.j", "Crédito de la música, como pide su licencia", "música"),
])
v.imagen(4, "OPCIONAL", "La miniatura", "marca/miniatura-lectura22.png",
         [("preparar/miniatura.py hace una con la misma cara para todas tus lecturas.", "miniatura")])
v.lista(5, "EL CORREO · 2.m", "Por último", [
    ("", "Respondes «Ganador y elegibles…»", "respondes"),
    ("", "Solo al profesor", "profesor"),
    ("", "Con el enlace, dentro del plazo", "plazo"),
])
v.cierre(6, "¿UNA IDEA O UN PROBLEMA? ABRE UN ISSUE", "CONTRIBUTING.md")
v.escribir()
