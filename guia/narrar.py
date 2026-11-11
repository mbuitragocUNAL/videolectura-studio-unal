#!/usr/bin/env python3
"""La narración de la guía, con Qwen3-TTS en el computador.

    preparar/.venv/bin/python guia/narrar.py disenar            # tres voces candidatas
    preparar/.venv/bin/python guia/narrar.py generar --voz 2    # todas las escenas con la 2

La voz se diseña a partir de una descripción (modelo VoiceDesign) diciendo la
primera escena; la elegida se clona (modelo Base) para todas las demás, así
la narración suena a una sola persona. Cada escena queda en
video/assets/narracion/escena-N.wav, a −16 LUFS, y duraciones.json dice cuánto
dura cada una para montar el video.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

AQUI = Path(__file__).resolve().parent
VOZ = AQUI / "voz"
SALIDA = AQUI / "video" / "assets" / "narracion"
DESCRIPCION = ("Voz de narrador joven y cercano, español latinoamericano neutro, timbre cálido y claro, "
               "ritmo pausado de tutorial, tono amable y seguro, sin dramatismo.")


def escenas() -> list[str]:
    texto = (AQUI / "guion.md").read_text(encoding="utf-8")
    out = []
    for m in re.finditer(r"\*\*Narración:\*\*(.*?)(?=\n\*\*Imagen:\*\*)", texto, re.S):
        out.append(re.sub(r"\s+", " ", m.group(1)).strip().replace("«", "").replace("»", ""))
    return out


def modelo(nombre: str):
    import torch
    from qwen_tts import Qwen3TTSModel
    dev = "mps" if torch.backends.mps.is_available() else "cpu"
    dtype = torch.bfloat16 if dev != "cpu" else torch.float32
    print(f"  cargando {nombre.split('/')[-1]} en {dev}", flush=True)
    return Qwen3TTSModel.from_pretrained(nombre, device_map=dev, dtype=dtype)


def semilla(n: int) -> None:
    import random
    import torch
    random.seed(n)
    torch.manual_seed(n)


def normalizar(entrada: Path, salida: Path) -> float:
    medida = subprocess.run(["ffmpeg", "-hide_banner", "-nostats", "-i", str(entrada), "-af",
                             "loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json", "-f", "null", "-"],
                            capture_output=True, text=True).stderr
    v = json.loads(re.search(r"\{[^{}]*\"input_i\"[^{}]*\}", medida).group())
    filtro = (f"loudnorm=I=-16:TP=-1.5:LRA=11:measured_I={v['input_i']}:measured_TP={v['input_tp']}"
              f":measured_LRA={v['input_lra']}:measured_thresh={v['input_thresh']}"
              f":offset={v['target_offset']}:linear=true")
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", str(entrada), "-af", filtro, "-ar", "48000",
                    "-ac", "1", str(salida)], check=True)
    p = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0",
                        str(salida)], capture_output=True, text=True)
    return round(float(p.stdout.strip()), 2)


def disenar() -> None:
    import soundfile as sf
    VOZ.mkdir(exist_ok=True)
    texto = escenas()[0]
    m = modelo("Qwen/Qwen3-TTS-12Hz-1.7B-VoiceDesign")
    for i in (1, 2, 3):
        semilla(700 + i)
        ondas, sr = m.generate_voice_design(text=texto, language="Spanish", instruct=DESCRIPCION,
                                            max_new_tokens=2048)
        sf.write(str(VOZ / f"candidata-{i}.wav"), ondas[0], sr)
        print(f"  candidata {i}: {len(ondas[0]) / sr:.1f} s", flush=True)
    (VOZ / "referencia.txt").write_text(texto + "\n", encoding="utf-8")


def generar(voz: int) -> None:
    import soundfile as sf
    SALIDA.mkdir(parents=True, exist_ok=True)
    ref = VOZ / f"candidata-{voz}.wav"
    m = modelo("Qwen/Qwen3-TTS-12Hz-1.7B-Base")
    prompt = m.create_voice_clone_prompt(ref_audio=str(ref),
                                         ref_text=(VOZ / "referencia.txt").read_text(encoding="utf-8").strip())
    duraciones = []
    crudo = VOZ / "crudo.wav"
    for k, texto in enumerate(escenas(), start=1):
        semilla(1234)
        ondas, sr = m.generate_voice_clone(text=texto, language="Spanish", voice_clone_prompt=prompt,
                                           max_new_tokens=2048)
        sf.write(str(crudo), ondas[0], sr)
        dur = normalizar(crudo, SALIDA / f"escena-{k}.wav")
        duraciones.append(dur)
        print(f"  escena {k}: {dur} s", flush=True)
    crudo.unlink(missing_ok=True)
    (SALIDA / "duraciones.json").write_text(json.dumps(duraciones), encoding="utf-8")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = ap.add_subparsers(dest="orden", required=True)
    sub.add_parser("disenar")
    g = sub.add_parser("generar")
    g.add_argument("--voz", type=int, required=True)
    a = ap.parse_args()
    disenar() if a.orden == "disenar" else generar(a.voz)
    return 0


if __name__ == "__main__":
    sys.exit(main())
