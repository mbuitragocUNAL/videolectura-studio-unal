#!/usr/bin/env python3
"""Cuándo se dice cada palabra de una toma.

El render pone las figuras donde el texto las nombra: el pie a pantalla
completa mientras se lee, la figura fija a la derecha desde la frase que la
nombra. Una toma no trae tiempos por palabra, así que este script la pasa por
Whisper y escribe, para cada palabra reconocida, cuándo empieza y cuándo
termina. El estudio empareja esas palabras con el texto del párrafo; no hace
falta que el reconocimiento sea perfecto, solo que acierte la mayoría.

    python3 alinear.py TOMA.webm SALIDA.json [--texto "lo que se lee"]

Modelo: STUDIO_WHISPER_MODEL (por defecto «small»). La primera vez se descarga.
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys


def glosario(texto: str, maximo: int = 200) -> str:
    """Nombres, cifras y palabras largas del texto, sin repetir y sin frases."""
    import re
    vistas, out = set(), []
    for i, w in enumerate(re.findall(r"[\wÁÉÍÓÚáéíóúñÑüÜ-]+", texto)):
        clave = w.lower()
        rara = (any(c.isdigit() for c in w) or len(w) >= 10
                or (w[0].isupper() and i > 0 and len(w) > 2))
        if rara and clave not in vistas:
            vistas.add(clave)
            out.append(w)
    return ", ".join(out)[:maximo]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("toma")
    ap.add_argument("salida")
    ap.add_argument("--texto", default="", help="el texto que se lee: ayuda con nombres y términos")
    ap.add_argument("--modelo", default=os.environ.get("STUDIO_WHISPER_MODEL", "small"))
    ap.add_argument("--idioma", default="es")
    a = ap.parse_args()

    try:
        from faster_whisper import WhisperModel
    except ImportError:
        print(json.dumps({"error": "falta faster-whisper: pip install faster-whisper"}))
        return 2

    import numpy as np

    # ffmpeg decodes, not PyAV: faster-whisper's own reader breaks with some
    # PyAV releases, and ffmpeg is already here for everything else.
    crudo = subprocess.run(["ffmpeg", "-v", "error", "-i", a.toma, "-vn", "-ac", "1", "-ar", "16000",
                            "-f", "f32le", "-"], check=True, capture_output=True).stdout
    voz = np.frombuffer(crudo, dtype=np.float32)
    modelo = WhisperModel(a.modelo, device="cpu", compute_type="int8")
    # Las palabras raras del texto como contexto previo: así acierta
    # «buckyball» o «C60». Nunca el texto mismo: Whisper toma lo que ya «oyó»
    # en el contexto como dicho, se salta ese comienzo de la toma o lo
    # inventa, y las figuras de ese tramo quedaban segundos tarde.
    segmentos, _ = modelo.transcribe(
        voz, language=a.idioma, word_timestamps=True, vad_filter=False,
        initial_prompt=glosario(a.texto) or None, condition_on_previous_text=False)
    palabras = []
    for s in segmentos:
        for w in s.words or []:
            palabras.append({"w": w.word.strip(), "a": round(w.start, 3), "b": round(w.end, 3)})

    os.makedirs(os.path.dirname(os.path.abspath(a.salida)), exist_ok=True)
    with open(a.salida, "w", encoding="utf-8") as f:
        json.dump({"modelo": a.modelo, "palabras": palabras}, f, ensure_ascii=False)
    print(json.dumps({"palabras": len(palabras)}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
