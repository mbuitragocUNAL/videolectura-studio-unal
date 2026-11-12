import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from hf import Video  # noqa: E402

AQUI = Path(__file__).resolve().parent
v = Video(AQUI, 4, "Videolectura Studio · Grabar")
v.portada(1, "Grabar", "Una toma es un párrafo.")
v.captura(2, "LA CONSOLA", "Lo que ves", [("consola", [(0, 960, 540, 1244 / 1920), ("izquierda", 700, 620, 1.0),
                                                       ("línea", 960, 120, 1.3), ("derecha", 1735, 560, 1.5)])],
          lado=("rotulos", [("", "El pasaje, en párrafos", "izquierda"), ("", "La línea de tiempo", "línea"),
                            ("", "El panel de grabación", "derecha")]))
v.captura(3, "ANTES DE GRABAR", "La presentación", [("presentador", [(0, 960, 420, 0.9)])],
          lado=("rotulos", [("", "Maximizada: solo el texto", "maximizas"), ("", "Abre el micrófono", "micrófono"),
                            ("", "Audífonos puestos", "audífonos")]))
v.captura(4, "GRABAR", "Un párrafo, una toma", [("consola", [(0, 1735, 540, 1.6)])],
          lado=("teclas", [("R", "grabar", "R"), ("S", "parar", "S"), ("Av Pág", "el siguiente", "Avance")]),
          nota=("¿Te equivocaste? La toma nueva reemplaza a la anterior.", "equivocas"))
v.lista(5, "FIGURAS Y CITAS", "Mientras lees", [
    ("2.d", "Figura nueva: termina la frase y lee su pie", "figura"),
    ("2.d", "…y el texto que tenga la imagen", "texto"),
    ("2.i", "Cita: lee hasta donde empieza y pulsa N", "citas"),
    ("2.i", "Silencio mientras suena", "silencio"),
])
v.lista(6, "CONSEJOS", "Para que suene bien", [
    ("", "Lee a tu ritmo: los silencios largos se recortan solos", "ritmo"),
    ("", "Misma distancia al micrófono", "distancia"),
    ("", "Medio segundo antes de parar", "medio"),
])
v.cierre(7, "SIGUIENTE · MONTAR Y RENDERIZAR", "docs/4-grabar.md")
v.escribir()
