import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from hf import Video  # noqa: E402

AQUI = Path(__file__).resolve().parent
v = Video(AQUI, 5, "Videolectura Studio · Montar y renderizar")
v.portada(1, "Montar y renderizar", "Del montaje al video final.")
v.captura(2, "MONTAJE", "Opcional", [("consola-montaje", [(0, 960, 540, 1244 / 1920), ("bloque", 960, 120, 1.3),
                                                        ("recortar", 1000, 640, 1.0)])],
          lado=("rotulos", [("", "La lectura completa, una toma por bloque", "bloque"),
                            ("", "Recorta un tropiezo sin regrabar", "recortar"),
                            ("", "Propone; nunca corta solo", "propone")]))
v.captura(3, "RENDER", "Montar el video", [("consola-render", [(0, 960, 540, 1244 / 1920), ("Montar", 460, 420, 1.35)])],
          lado=("rotulos", [("", "Escucha cada toma", "escucha"), ("", "Recorta las pausas largas", "pausas"),
                            ("", "Todas al mismo volumen", "volumen")]))
v.fragmentos(4, "RENDER", "La imagen", [("texto", 0, 0.0), ("figura", "figura", 0.5), ("flecha", "flecha", 1.4), ("fija", "fija", 0.0)],
             [("El texto corre con tu voz", "texto"), ("La figura, con su pie (2.d)", "figura"),
              ("Flecha de regreso", "flecha"), ("Figura fija, mismo tamaño (2.f)", "fija")])
v.lista(5, "ANTES DE ENTREGAR", "Revisa el video", [
    ("2.d", "Las figuras aparecen mientras lees su pie", "figuras"),
    ("2.i", "Las citas suenan en su lugar", "citas"),
    ("5–25", "Dura entre 5 y 25 minutos", "dure"),
])
v.cierre(6, "SIGUIENTE · ENTREGAR", "docs/5-montar-y-renderizar.md")
v.escribir()
