import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from hf import Video  # noqa: E402

AQUI = Path(__file__).resolve().parent
v = Video(AQUI, 3, "Videolectura Studio · Voces y música")
v.portada(1, "Voces y música", "Una voz por personaje, y una pista de fondo.")
v.lista(2, "LINEAMIENTO 2.i", "Tres formas de dar voz", [
    ("API", "ElevenLabs, con tu propia clave", "propia"),
    ("LOCAL", "Qwen3-TTS, libre, en tu computador", "modelo"),
    ("AUDIO", "Un audio tuyo", "audio"),
], pie=("Cada personaje suena igual en todas sus citas.", "personaje"))
v.captura(3, "VOCES", "Describe y conecta", [("preparar-citas", [(0, 390, 540, 1.4), ("clave", 300, 400, 1.8)])],
          lado=("rotulos", [("", "Cómo suena: edad, tono, acento", "edad"), ("", "La clave queda solo en tu computador", "clave")]))
v.captura(4, "VOCES", "Diseñar y generar", [("preparar-citas", [(0, 330, 530, 1.8), ("Generar", 390, 600, 1.6)])],
          lado=("rotulos", [("", "Diseñar: tres candidatas", "Diseñar"), ("", "Escuchas y eliges una", "escuchas"),
                            ("", "Generar: todos los clips, al volumen de tu voz", "Generar")]))
v.terminal(5, "QWEN EN UN MAC", "Desde la terminal", [
    ("$ ", './estudio voces disenar lecturas/22 --personaje Electrón --motor qwen', "diseñar"),
    ("$ ", './estudio voces elegir lecturas/22 --personaje Electrón --candidato 2', "elegir"),
    ("$ ", "./estudio voces generar lecturas/22", "generar"),
], chips=[("Mucho más rápido", "rápido")])
v.captura(6, "LINEAMIENTO 2.j", "Música de fondo", [("preparar-citas", [(0, 390, 990, 1.6)])],
          lado=("rotulos", [("", "Opcional: eliges o subes una pista", "Eliges"), ("", "14 dB bajo tu voz", "decibeles"),
                            ("", "Se repite y baja cuando hablas", "repite"), ("", "Con licencia, y el crédito en la descripción", "licencia")]))
v.cierre(7, "SIGUIENTE · GRABAR", "docs/3-voces-y-musica.md")
v.escribir()
