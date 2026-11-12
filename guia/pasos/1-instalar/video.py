import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from hf import Video  # noqa: E402

AQUI = Path(__file__).resolve().parent
v = Video(AQUI, 1, "Videolectura Studio · Instalar y abrir")
v.portada(1, "Instalar y abrir", "Unos minutos, y solo la primera vez.")
v.lista(2, "LO QUE NECESITAS", "Cinco cosas", [
    ("", "Docker: corre todo el estudio", "Docker"),
    ("", "Google Chrome, para grabar", "Chrome"),
    ("", "Unos 6 GB libres", "seis"),
    ("", "El PDF del libro, con texto seleccionable", "PDF"),
    ("", "Audífonos, para oír las citas", "audífonos"),
])
v.terminal(3, "INSTALAR", "Tres comandos", [
    ("$ ", "git clone https://github.com/mbuitragocUNAL/videolectura-studio-unal", "clonas"),
    ("$ ", "cd videolectura-studio-unal", "entras"),
    ("$ ", "./estudio abrir", "corres"),
    ("", "  Estudio en http://localhost:8100", "primera"),
], chips=[("Windows: desde WSL", "Windows")])
v.captura(4, "ABRIR", "localhost:8100", [("vacio-lecturas", [(0, 960, 540, 1244 / 1920), ("Lecturas", 900, 240, 1.35)])],
          lado=("rotulos", [("", "Abre en Chrome", "Chrome"), ("", "Sin lecturas todavía", "lecturas"),
                            ("", "Sigue: Nueva lectura", "Nueva")]))
v.terminal(5, "EL DÍA A DÍA", "Apagar y actualizar", [
    ("$ ", "./estudio cerrar", "cerrar"),
    ("", "  # tus lecturas y tomas quedan en lecturas/", "carpeta"),
    ("$ ", "git pull", "pull"),
    ("$ ", "./estudio abrir", "otra"),
])
v.cierre(6, "SIGUIENTE · PREPARAR LA LECTURA", "docs/1-instalar.md")
v.escribir()
