"""La composición del pasaje para el video.

    preparar/.venv/bin/python -m unittest preparar/test_componer.py
"""
from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from componer import GUION_BLANDO, _letras, _ubicar, partir, silabear  # noqa: E402


def partida(palabra: str) -> str:
    out, prev = [], 0
    for k in silabear(palabra):
        out.append(palabra[prev:k])
        prev = k
    out.append(palabra[prev:])
    return "-".join(out)


class PruebaComponer(unittest.TestCase):
    def test_silabas(self):
        for palabra, esperada in [
            ("construir", "cons-truir"), ("estructura", "es-truc-tura"), ("perro", "pe-rro"),
            ("orquesta", "or-questa"), ("electrones", "elec-tro-nes"), ("hollín", "ho-llín"),
            ("teoría", "teo-ría"),            # nunca entre vocales
            ("mesa", "mesa"),                 # muy corta
            ("buckyballs", "buckyballs"),     # extranjera
        ]:
            self.assertEqual(partida(palabra), esperada)

    def test_no_parte_dentro_de_etiquetas(self):
        html = '<p style="margin-left: 30pt">La <i>estructura</i> del diamante</p>'
        out = partir(html)
        self.assertIn('<p style="margin-left: 30pt">', out)
        self.assertIn(f"<i>es{GUION_BLANDO}truc{GUION_BLANDO}tura</i>", out)
        # El bloqueo de ligaduras tampoco entra en las etiquetas.
        self.assertIn("margin-left", partir('<p style="margin-left: 30pt">fin</p>'))
        self.assertIn("f\u200cin", partir('<p style="margin-left: 30pt">fin</p>'))

    def test_letras_deshace_ligaduras(self):
        self.assertEqual(_letras("ﬁnal, la ﬂor")[0], "finallaflor")

    def test_ubicar_lineas_en_su_parrafo(self):
        parrafos = [{"n": 1, "dicho": "¡Carbono!"},
                    {"n": 3, "dicho": "Esas tres reglas se despliegan en los mundos de la ciencia de materiales."}]
        lineas = [{"t": "¡Carbono!", "y": 10, "h": 80},
                  {"t": "Esas\xa0tres\xa0reglas\xa0se\xa0des\xad", "y": 100, "h": 70},
                  {"t": "pliegan en los mundos de la cien\xad", "y": 170, "h": 70},
                  {"t": "cia de materiales.", "y": 240, "h": 70}]
        mapa, perdidas = _ubicar(parrafos, lineas)
        self.assertEqual(perdidas, 0)
        self.assertEqual(mapa["1"], [[1, 10, 80]])
        dicho = parrafos[1]["dicho"]
        offs = [x[0] for x in mapa["3"]]
        self.assertEqual(offs[0], 0)
        self.assertEqual(dicho[offs[1]:offs[1] + 7], "pliegan")
        self.assertEqual(dicho[offs[2]:offs[2] + 3], "cia")


if __name__ == "__main__":
    unittest.main()
