#!/usr/bin/env python3
"""La narración de la guía: Qwen3-TTS en el computador, o ElevenLabs.

Con Qwen3-TTS (libre, local):

    preparar/.venv/bin/python guia/narrar.py disenar                      # tres voces candidatas
    preparar/.venv/bin/python guia/narrar.py generar --voz 2              # la guía general
    preparar/.venv/bin/python guia/narrar.py generar --voz 2 --carpeta guia/pasos/1-instalar

Con ElevenLabs (mejor voz; usa caracteres de tu cuenta):

    export ELEVENLABS_API_KEY=...        # o la clave guardada en voces/claves.json
    preparar/.venv/bin/python guia/narrar.py disenar --motor elevenlabs   # tres candidatas
    preparar/.venv/bin/python guia/narrar.py elegir 2                     # guarda la voz en tu cuenta
    preparar/.venv/bin/python guia/narrar.py generar --motor elevenlabs --carpeta guia/pasos/1-instalar
    preparar/.venv/bin/python guia/narrar.py generar --motor elevenlabs --voz-id <id> ...   # otra voz

Sin clave en el entorno ni en voces/claves.json, la pide en la terminal (sin
mostrarla ni guardarla).

La voz se diseña a partir de una descripción diciendo la primera escena de la
guía; con Qwen, la elegida se clona (modelo Base) para todas las demás; con
ElevenLabs, queda como una voz de tu cuenta. Así la narración suena a una sola
persona, en la guía y en los videos de cada paso.
Cada escena queda en <carpeta>/video/assets/narracion/escena-N.wav, a −16 LUFS;
duraciones.json dice cuánto dura cada una y palabras-N.json cuándo se dice cada
palabra (faster-whisper con Qwen; los tiempos que devuelve ElevenLabs con
ElevenLabs), para que lo que aparece en pantalla llegue con ella.
"""
from __future__ import annotations

import argparse
import base64
import getpass
import json
import os
import re
import subprocess
import sys
from pathlib import Path

AQUI = Path(__file__).resolve().parent
VOZ = AQUI / "voz"
CARPETA = AQUI    # la de guion.md; --carpeta la cambia
ELEVEN_VOZ = VOZ / "elevenlabs.json"      # la voz elegida: {"voz": id, "nombre": ...}
ELEVEN_DISENOS = VOZ / "elevenlabs"        # las candidatas
ELEVEN_DESCRIPCION = ("Narrador joven latinoamericano, voz cálida y clara, cercana y segura, "
                      "ritmo pausado de tutorial, dicción limpia, grabación de estudio sin ruido.")
DESCRIPCION = ("Voz de narrador joven y cercano, español latinoamericano neutro, timbre cálido y claro, "
               "ritmo pausado de tutorial, tono amable y seguro, sin dramatismo.")


def escenas() -> list[str]:
    texto = (CARPETA / "guion.md").read_text(encoding="utf-8")
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


def palabras(wav: Path, texto: str) -> list[dict]:
    """Cuándo se dice cada palabra de la escena."""
    from faster_whisper import WhisperModel
    global _whisper
    if "_whisper" not in globals():
        _whisper = WhisperModel("small", device="cpu", compute_type="int8")
    import numpy as np
    # ffmpeg decodifica, no PyAV: el lector propio de faster-whisper falla
    # con algunas versiones de PyAV (lo mismo que en preparar/alinear.py).
    crudo = subprocess.run(["ffmpeg", "-v", "error", "-i", str(wav), "-ac", "1", "-ar", "16000",
                            "-f", "f32le", "-"], check=True, capture_output=True).stdout
    segmentos, _ = _whisper.transcribe(np.frombuffer(crudo, dtype=np.float32), language="es",
                                       word_timestamps=True, condition_on_previous_text=False)
    return [{"w": w.word.strip(), "a": round(w.start, 2)} for s in segmentos for w in s.words or []]


def generar(voz: int) -> None:
    import soundfile as sf
    SALIDA = CARPETA / "video" / "assets" / "narracion"
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
        (SALIDA / f"palabras-{k}.json").write_text(
            json.dumps(palabras(SALIDA / f"escena-{k}.wav", texto), ensure_ascii=False), encoding="utf-8")
        print(f"  escena {k}: {dur} s", flush=True)
    crudo.unlink(missing_ok=True)
    (SALIDA / "duraciones.json").write_text(json.dumps(duraciones), encoding="utf-8")


# ---- ElevenLabs --------------------------------------------------------------
# El cliente es el del estudio (preparar/voz.py): la misma clave, los mismos
# errores en español.

def _eleven():
    sys.path.insert(0, str(AQUI.parent / "preparar"))
    import voz as vz
    if not os.environ.get("SSL_CERT_FILE"):
        try:    # el Python de python.org en macOS no trae los certificados
            import certifi
            os.environ["SSL_CERT_FILE"] = certifi.where()
        except ImportError:
            pass
    for intento in (1, 2):
        clave, _ = vz.clave_eleven()
        try:
            if not clave:
                raise vz.ErrorVoz("no hay clave")
            c = vz.eleven_cuenta()
            print(f"  ElevenLabs: {c['usados']} de {c['limite']} caracteres usados este mes", flush=True)
            return vz
        except vz.ErrorVoz as e:
            # Una clave restringida a voz puede no tener permiso de leer la
            # cuenta: eso no impide narrar.
            if clave and "invalid" not in str(e).lower() and "no hay clave" not in str(e):
                print(f"  ElevenLabs: no pude ver los caracteres de la cuenta ({e})", flush=True)
                return vz
            if intento == 2 or not sys.stdin.isatty():
                raise SystemExit(f"ElevenLabs: {e}. Pasa la clave en ELEVENLABS_API_KEY.")
            print(f"  ElevenLabs: {e}")
            os.environ["ELEVENLABS_API_KEY"] = getpass.getpass(
                "  Clave de ElevenLabs (no se muestra ni se guarda): ").strip()
    return vz


def disenar_eleven(descripcion: str) -> None:
    vz = _eleven()
    texto = escenas()[0]
    r = vz.eleven("POST", "/v1/text-to-voice/design", consulta={"output_format": "mp3_44100_128"},
                  cuerpo={"voice_description": f"Habla en español. {descripcion}", "text": texto,
                          "model_id": "eleven_multilingual_ttv_v2"})
    ELEVEN_DISENOS.mkdir(parents=True, exist_ok=True)
    candidatas = []
    for i, p in enumerate(r.get("previews", []), start=1):
        (ELEVEN_DISENOS / f"candidata-{i}.mp3").write_bytes(base64.b64decode(p["audio_base_64"]))
        candidatas.append({"n": i, "id": p["generated_voice_id"]})
        print(f"  candidata {i}: {ELEVEN_DISENOS / f'candidata-{i}.mp3'}", flush=True)
    (ELEVEN_DISENOS / "disenos.json").write_text(json.dumps(
        {"descripcion": descripcion, "texto": r.get("text") or texto, "candidatas": candidatas},
        ensure_ascii=False, indent=1), encoding="utf-8")


def elegir_eleven(n: int) -> None:
    vz = _eleven()
    info = json.loads((ELEVEN_DISENOS / "disenos.json").read_text(encoding="utf-8"))
    c = next((c for c in info["candidatas"] if c["n"] == n), None)
    if not c:
        raise SystemExit(f"No hay candidata {n}; vuelve a diseñar.")
    nombre = "Narrador · Videolectura Studio"
    r = vz.eleven("POST", "/v1/text-to-voice", cuerpo={
        "voice_name": nombre, "voice_description": info["descripcion"], "generated_voice_id": c["id"],
        "labels": {"proyecto": "videolectura-studio", "uso": "guia"}})
    ELEVEN_VOZ.write_text(json.dumps({"voz": r["voice_id"], "nombre": nombre, "candidata": n},
                                     ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"  voz guardada: {r['voice_id']}")


def _palabras_de(alineacion: dict) -> list[dict]:
    """De los tiempos por carácter de ElevenLabs, cuándo empieza cada palabra."""
    out, palabra, inicio = [], "", 0.0
    for ch, t in zip(alineacion["characters"], alineacion["character_start_times_seconds"]):
        if ch.isspace():
            if palabra:
                out.append({"w": palabra, "a": round(inicio, 2)})
            palabra = ""
        else:
            if not palabra:
                inicio = t
            palabra += ch
    if palabra:
        out.append({"w": palabra, "a": round(inicio, 2)})
    return out


def generar_eleven(voz_id: str) -> None:
    vz = _eleven()
    if not voz_id:
        try:
            voz_id = json.loads(ELEVEN_VOZ.read_text(encoding="utf-8"))["voz"]
        except FileNotFoundError:
            raise SystemExit("No hay voz de ElevenLabs: diseña y elige una, o pasa --voz-id.") from None
    SALIDA = CARPETA / "video" / "assets" / "narracion"
    SALIDA.mkdir(parents=True, exist_ok=True)
    textos = escenas()
    crudo = VOZ / "crudo.mp3"
    duraciones = []
    for k, texto in enumerate(textos, start=1):
        r = vz.eleven("POST", f"/v1/text-to-speech/{voz_id}/with-timestamps",
                      consulta={"output_format": "mp3_44100_128"},
                      cuerpo={"text": texto, "model_id": vz.eleven_modelo(), "seed": 1234,
                              # el contexto de las escenas vecinas: la entonación
                              # sigue de una a otra, como una sola lectura
                              "previous_text": textos[k - 2] if k > 1 else None,
                              "next_text": textos[k] if k < len(textos) else None,
                              "voice_settings": {"stability": 0.5, "similarity_boost": 0.8,
                                                 "style": 0.0, "use_speaker_boost": True}})
        crudo.write_bytes(base64.b64decode(r["audio_base64"]))
        dur = normalizar(crudo, SALIDA / f"escena-{k}.wav")
        duraciones.append(dur)
        (SALIDA / f"palabras-{k}.json").write_text(
            json.dumps(_palabras_de(r["alignment"]), ensure_ascii=False), encoding="utf-8")
        print(f"  escena {k}: {dur} s", flush=True)
    crudo.unlink(missing_ok=True)
    (SALIDA / "duraciones.json").write_text(json.dumps(duraciones), encoding="utf-8")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = ap.add_subparsers(dest="orden", required=True)
    d = sub.add_parser("disenar")
    d.add_argument("--motor", choices=["qwen", "elevenlabs"], default="qwen")
    d.add_argument("--descripcion", default="", help="cómo suena el narrador (ElevenLabs)")
    e = sub.add_parser("elegir", help="guarda una candidata de ElevenLabs como la voz de la guía")
    e.add_argument("candidata", type=int)
    g = sub.add_parser("generar")
    g.add_argument("--motor", choices=["qwen", "elevenlabs"], default="qwen")
    g.add_argument("--voz", type=int, help="la candidata de Qwen que se clona")
    g.add_argument("--voz-id", default="", help="ElevenLabs: una voz de tu cuenta (por defecto, la elegida)")
    g.add_argument("--carpeta", default="", help="la del guion.md (por defecto, la guía general)")
    a = ap.parse_args()
    global CARPETA
    if getattr(a, "carpeta", ""):
        CARPETA = Path(a.carpeta).resolve()
    if a.orden == "disenar":
        disenar_eleven(a.descripcion or ELEVEN_DESCRIPCION) if a.motor == "elevenlabs" else disenar()
    elif a.orden == "elegir":
        elegir_eleven(a.candidata)
    elif a.motor == "elevenlabs":
        generar_eleven(a.voz_id)
    else:
        if a.voz is None:
            ap.error("con Qwen, --voz dice qué candidata clonar")
        generar(a.voz)
    return 0


if __name__ == "__main__":
    sys.exit(main())
