#!/usr/bin/env python3
"""Las voces de las citas (lineamiento 2.i).

Cada cita textual se lee con voz sintética, la misma voz para un mismo
personaje en toda la lectura. lector.py deja el texto de cada cita en
audio/tts/NN-personaje.txt; este programa pone al lado el NN-personaje.mp3:

  - con ElevenLabs, con la clave de quien graba;
  - con Qwen3-TTS, en el computador, sin cuenta ni red después de la descarga;
  - o con un audio que uno mismo grabó o generó en otra parte («subir»).

Una voz se diseña una vez a partir de la descripción del personaje: salen tres
candidatas, se escucha y se elige una, y esa queda para todas sus citas (y para
otras lecturas en las que aparezca). Cada clip lleva al lado un .json con la
voz y el texto con que se hizo; así se sabe cuáles hay que rehacer cuando
cambia la cita o la voz, y los demás no se vuelven a pagar.

Todos los clips quedan a la misma sonoridad que la narración del render
(−16 LUFS), para que ninguna cita suene más fuerte que quien lee.

    ./estudio voces estado   lecturas/22-…
    ./estudio voces disenar  lecturas/22-… --personaje "Sócrates" --motor qwen
    ./estudio voces elegir   lecturas/22-… --personaje "Sócrates" --motor qwen --candidato 2
    ./estudio voces generar  lecturas/22-…            (las que faltan o cambiaron)
    ./estudio voces subir    lecturas/22-… --clip 03-glaucon ~/glaucon.wav

Con --json responde JSON, que es lo que usa la consola.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import tomllib
import unicodedata
import urllib.error
import urllib.parse
import urllib.request
import zlib
from pathlib import Path

RAIZ = Path(__file__).resolve().parent.parent
VOCES_DIR = Path(os.environ.get("STUDIO_VOCES_DIR") or RAIZ / "voces")
ELEVEN_URL = os.environ.get("ELEVENLABS_URL", "https://api.elevenlabs.io").rstrip("/")
LUFS = float(os.environ.get("STUDIO_VOZ_LUFS", "-16"))

MOTORES = ("elevenlabs", "qwen", "manual")
ELEVEN_MODELO = "eleven_multilingual_v2"
QWEN_DISENO = os.environ.get("STUDIO_QWEN_DISENO", "Qwen/Qwen3-TTS-12Hz-1.7B-VoiceDesign")
QWEN_BASE = os.environ.get("STUDIO_QWEN_BASE", "Qwen/Qwen3-TTS-12Hz-1.7B-Base")
CANDIDATAS = 3

JSON = False  # --json: una línea JSON por avance y la respuesta al final


class ErrorVoz(Exception):
    pass


def slug(s: str) -> str:
    s = unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode()
    return re.sub(r"[^a-z0-9]+", "-", s.lower()).strip("-") or "voz"


def avisar(**datos) -> None:
    """Un paso de un trabajo largo: la consola lo muestra como progreso."""
    if JSON:
        print(json.dumps({"avance": datos}, ensure_ascii=False), flush=True)
    else:
        hecho, total = datos.get("hecho"), datos.get("total")
        cuenta = f"[{hecho}/{total}] " if total else ""
        print(f"  {cuenta}{datos.get('paso', '')}", flush=True)


def responder(obj, texto: str = "") -> None:
    if JSON:
        print(json.dumps({"respuesta": obj}, ensure_ascii=False), flush=True)
    elif texto:
        print(texto)
    else:
        print(json.dumps(obj, ensure_ascii=False, indent=1))


# ----------------------------------------------------------------------------
# la lectura: sus citas y la voz de cada personaje
# ----------------------------------------------------------------------------

def lectura(carpeta: Path) -> tuple[list[dict], dict[str, dict]]:
    """Los clips de marcas.json y la tabla [voces] de lectura.toml."""
    marcas = carpeta / "marcas.json"
    if not marcas.exists():
        raise ErrorVoz("La lectura no está preparada: falta marcas.json.")
    clips = json.loads(marcas.read_text(encoding="utf-8")).get("clips", [])
    toml = carpeta / "lectura.toml"
    voces = {}
    if toml.exists():
        voces = tomllib.loads(toml.read_text(encoding="utf-8")).get("voces", {})
    return clips, voces


def asignacion(voces: dict, personaje: str) -> dict:
    v = voces.get(personaje, {})
    motor = v.get("motor") or "manual"
    return {"motor": motor if motor in MOTORES else "manual", "voz": v.get("voz", ""),
            "vozNombre": v.get("voz_nombre", ""), "descripcion": v.get("descripcion", ""),
            "color": v.get("color", "")}


def registro_de(carpeta: Path, clip: str) -> Path:
    return carpeta / "audio" / "tts" / f"{clip}.json"


def estado_clip(carpeta: Path, c: dict, voz: dict) -> dict:
    """falta, lista, texto (la cita cambió) u otra-voz (el personaje cambió de voz)."""
    mp3 = carpeta / "audio" / "tts" / f"{c['clip']}.mp3"
    out = {"clip": c["clip"], "personaje": c["personaje"], "texto": c["texto"],
           "linea": c.get("linea"), "caracteres": len(c["texto"]), "dur": c.get("dur") or 0}
    if not mp3.exists():
        return {**out, "estado": "falta", "motor": ""}
    out["bytes"] = mp3.stat().st_size
    reg = registro_de(carpeta, c["clip"])
    if not reg.exists():
        # Hecho a mano antes de que existiera el registro: se respeta.
        return {**out, "estado": "lista", "motor": "manual"}
    r = json.loads(reg.read_text(encoding="utf-8"))
    out["motor"] = r.get("motor", "manual")
    if r.get("texto") != c["texto"]:
        return {**out, "estado": "texto"}
    if r.get("motor") != "manual" and (r.get("motor"), r.get("voz")) != (voz["motor"], voz["voz"]):
        return {**out, "estado": "otra-voz"}
    return {**out, "estado": "lista"}


def estado(carpeta: Path) -> dict:
    clips, voces = lectura(carpeta)
    personajes: dict[str, dict] = {}
    filas = []
    for c in clips:
        p = c["personaje"]
        if p not in personajes:
            personajes[p] = {"nombre": p, **asignacion(voces, p), "clips": 0, "caracteres": 0}
        personajes[p]["clips"] += 1
        personajes[p]["caracteres"] += len(c["texto"])
        filas.append(estado_clip(carpeta, c, personajes[p]))
    return {"personajes": list(personajes.values()), "clips": filas,
            "pendientes": sum(1 for f in filas if f["estado"] != "lista")}


# ----------------------------------------------------------------------------
# sonoridad: todos los clips a −16 LUFS, como la narración en el render
# ----------------------------------------------------------------------------

def normalizar(entrada: Path, salida: Path) -> float:
    """Pasa entrada a mp3 mono a LUFS y devuelve su duración.

    loudnorm en dos pasadas, lineal: mide primero y después aplica una sola
    ganancia, que no deforma una frase corta como lo haría la compresión
    dinámica de una sola pasada."""
    if not shutil.which("ffmpeg"):
        raise ErrorVoz("Falta ffmpeg.")
    objetivo = f"I={LUFS}:TP=-1.5:LRA=11"
    medida = subprocess.run(
        ["ffmpeg", "-hide_banner", "-nostats", "-i", str(entrada), "-af",
         f"loudnorm={objetivo}:print_format=json", "-f", "null", "-"],
        capture_output=True, text=True)
    filtro = f"loudnorm={objetivo}"
    m = re.search(r"\{[^{}]*\"input_i\"[^{}]*\}", medida.stderr)
    if m:
        v = json.loads(m.group())
        if v.get("input_i") not in (None, "-inf") and float(v["input_i"]) > -70:
            filtro += (f":measured_I={v['input_i']}:measured_TP={v['input_tp']}"
                       f":measured_LRA={v['input_lra']}:measured_thresh={v['input_thresh']}"
                       f":offset={v['target_offset']}:linear=true")
    salida.parent.mkdir(parents=True, exist_ok=True)
    tmp = salida.with_suffix(".tmp.mp3")
    p = subprocess.run(
        ["ffmpeg", "-hide_banner", "-v", "error", "-y", "-i", str(entrada), "-af", filtro,
         "-ar", "44100", "-ac", "1", "-c:a", "libmp3lame", "-q:a", "2", str(tmp)],
        capture_output=True, text=True)
    if p.returncode != 0:
        tmp.unlink(missing_ok=True)
        raise ErrorVoz(f"ffmpeg no pudo convertir el audio: {p.stderr.strip()[-300:]}")
    tmp.replace(salida)
    return duracion(salida)


def duracion(ruta: Path) -> float:
    p = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration",
                        "-of", "default=nokey=1:noprint_wrappers=1", str(ruta)],
                       capture_output=True, text=True)
    try:
        return round(float(p.stdout.strip()), 2)
    except ValueError:
        return 0.0


def guardar_clip(carpeta: Path, c: dict, audio: Path, motor: str, voz: str, modelo: str = "") -> float:
    mp3 = carpeta / "audio" / "tts" / f"{c['clip']}.mp3"
    dur = normalizar(audio, mp3)
    registro_de(carpeta, c["clip"]).write_text(json.dumps({
        "motor": motor, "voz": voz, "modelo": modelo, "texto": c["texto"],
        "lufs": LUFS, "hecho": time.strftime("%Y-%m-%d %H:%M"),
    }, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
    return dur


# ----------------------------------------------------------------------------
# ElevenLabs
# ----------------------------------------------------------------------------

def claves() -> dict:
    ruta = VOCES_DIR / "claves.json"
    if ruta.exists():
        try:
            return json.loads(ruta.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            return {}
    return {}


def clave_eleven() -> tuple[str, str]:
    if os.environ.get("ELEVENLABS_API_KEY"):
        return os.environ["ELEVENLABS_API_KEY"].strip(), "entorno"
    k = (claves().get("elevenlabs") or "").strip()
    return k, "archivo" if k else ""


def eleven(metodo: str, ruta: str, cuerpo: dict | None = None, consulta: dict | None = None,
           binario: bool = False):
    clave, _ = clave_eleven()
    if not clave:
        raise ErrorVoz("No hay clave de ElevenLabs. Agrégala en la consola (Citas › ElevenLabs) "
                       "o en la variable ELEVENLABS_API_KEY.")
    url = ELEVEN_URL + ruta
    if consulta:
        url += "?" + urllib.parse.urlencode({k: v for k, v in consulta.items() if v is not None}, doseq=True)
    datos = json.dumps(cuerpo).encode() if cuerpo is not None else None
    req = urllib.request.Request(url, data=datos, method=metodo, headers={
        "xi-api-key": clave, "Content-Type": "application/json",
        "Accept": "audio/mpeg" if binario else "application/json",
    })
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            cuerpo_r = r.read()
    except urllib.error.HTTPError as e:
        detalle = e.read().decode(errors="replace")
        try:
            d = json.loads(detalle).get("detail", detalle)
            detalle = d.get("message", d) if isinstance(d, dict) else d
        except (json.JSONDecodeError, AttributeError):
            pass
        if e.code == 401:
            raise ErrorVoz(f"ElevenLabs no acepta la clave: {detalle}") from None
        raise ErrorVoz(f"ElevenLabs respondió {e.code}: {str(detalle)[:300]}") from None
    except urllib.error.URLError as e:
        raise ErrorVoz(f"No pude hablar con ElevenLabs: {e.reason}") from None
    return cuerpo_r if binario else json.loads(cuerpo_r or b"{}")


def eleven_modelo() -> str:
    return os.environ.get("ELEVENLABS_MODEL") or claves().get("elevenlabs_modelo") or ELEVEN_MODELO


def eleven_cuenta() -> dict:
    s = eleven("GET", "/v1/user/subscription")
    return {"nivel": s.get("tier"), "usados": s.get("character_count"),
            "limite": s.get("character_limit"), "renueva": s.get("next_character_count_reset_unix")}


def eleven_voces() -> list[dict]:
    out, token = [], None
    for _ in range(5):
        r = eleven("GET", "/v2/voices", consulta={"page_size": 100, "next_page_token": token,
                                                  "include_total_count": "false"})
        for v in r.get("voices", []):
            out.append({"id": v["voice_id"], "nombre": v.get("name", ""),
                        "categoria": v.get("category", ""), "descripcion": v.get("description") or "",
                        "muestra": v.get("preview_url") or "", "etiquetas": v.get("labels") or {}})
        token = r.get("next_page_token")
        if not r.get("has_more") or not token:
            break
    # Las propias (diseñadas o clonadas) primero: son las de los personajes.
    out.sort(key=lambda v: (v["categoria"] == "premade", v["nombre"].lower()))
    return out


def eleven_decir(voz: str, texto: str, personaje: str, destino: Path) -> None:
    audio = eleven("POST", f"/v1/text-to-speech/{urllib.parse.quote(voz)}", binario=True,
                   consulta={"output_format": "mp3_44100_128"},
                   cuerpo={"text": texto, "model_id": eleven_modelo(),
                           # La misma semilla para el mismo personaje: rehacer
                           # una cita no le cambia el carácter a la voz.
                           "seed": zlib.crc32(personaje.encode()) & 0xFFFFFFFF})
    if len(audio) < 512:
        raise ErrorVoz("ElevenLabs devolvió un audio vacío.")
    destino.write_bytes(audio)


# ----------------------------------------------------------------------------
# Qwen3-TTS, en el computador
# ----------------------------------------------------------------------------

def qwen_instalado() -> bool:
    import importlib.util
    return importlib.util.find_spec("qwen_tts") is not None


def qwen_dispositivo() -> tuple[str, object]:
    import torch
    pedido = os.environ.get("STUDIO_QWEN_DEVICE", "")
    if pedido:
        dev = pedido
    elif torch.cuda.is_available():
        dev = "cuda:0"
    elif getattr(torch.backends, "mps", None) and torch.backends.mps.is_available():
        dev = "mps"
    else:
        dev = "cpu"
    # bfloat16 en GPU (CUDA y el Mac: medio el tiempo de float32, y es como
    # Qwen evaluó el modelo); en CPU, float32.
    return dev, torch.float32 if dev == "cpu" else torch.bfloat16


def qwen_modelo(nombre: str):
    if not qwen_instalado():
        raise ErrorVoz("Qwen3-TTS no está instalado aquí. En el Mac: "
                       "preparar/.venv/bin/python -m pip install -r preparar/requirements-voz.txt "
                       "y después ./estudio voces … (corre en la GPU del Mac).")
    from qwen_tts import Qwen3TTSModel
    dev, dtype = qwen_dispositivo()
    avisar(paso=f"cargando {nombre.split('/')[-1]} en {dev} (la primera vez se descarga)")
    return Qwen3TTSModel.from_pretrained(nombre, device_map=dev, dtype=dtype)


def semilla(n: int) -> None:
    import random
    import torch
    random.seed(n)
    torch.manual_seed(n)


def escribir_wav(ruta: Path, onda, sr: int) -> None:
    import soundfile as sf
    sf.write(str(ruta), onda, sr)


def qwen_dir(voz: str) -> Path:
    if not re.fullmatch(r"[a-z0-9-]+", voz or ""):
        raise ErrorVoz(f"«{voz}» no es una voz local.")
    d = VOCES_DIR / "qwen" / voz
    if not (d / "referencia.wav").exists():
        raise ErrorVoz(f"No encuentro la voz local «{voz}» en {d}.")
    return d


def qwen_voces() -> list[dict]:
    out = []
    for d in sorted((VOCES_DIR / "qwen").glob("*/voz.json")):
        info = json.loads(d.read_text(encoding="utf-8"))
        out.append({"id": d.parent.name, "nombre": info.get("nombre", d.parent.name),
                    "descripcion": info.get("descripcion", ""), "categoria": "local",
                    "muestra": f"qwen/{d.parent.name}/referencia.wav"})
    return out


# ----------------------------------------------------------------------------
# diseñar una voz: tres candidatas a partir de la descripción
# ----------------------------------------------------------------------------

def texto_de_muestra(clips: list[dict], personaje: str, minimo: int, maximo: int) -> str:
    """Las propias líneas del personaje: con ellas la voz sale para lo que va a
    decir, y no para un texto genérico."""
    texto = " ".join(c["texto"] for c in clips if c["personaje"] == personaje)
    texto = re.sub(r"\s+", " ", texto).strip()
    if len(texto) > maximo:
        corte = texto[:maximo]
        fin = max(corte.rfind(". "), corte.rfind("? "), corte.rfind("! "))
        texto = corte[:fin + 1] if fin > minimo else corte[:corte.rfind(" ")]
    return texto if len(texto) >= minimo else ""


def dir_disenos(personaje: str) -> Path:
    return VOCES_DIR / "disenos" / slug(personaje)


def disenar(carpeta: Path, personaje: str, motor: str, descripcion: str = "") -> dict:
    clips, voces = lectura(carpeta)
    descripcion = (descripcion or voces.get(personaje, {}).get("descripcion", "")).strip()
    if not descripcion:
        raise ErrorVoz(f"Escribe primero cómo suena {personaje}: edad, oficio, ánimo, acento.")
    destino = dir_disenos(personaje)
    if destino.exists():
        shutil.rmtree(destino)
    destino.mkdir(parents=True)
    candidatas = []

    if motor == "elevenlabs":
        texto = texto_de_muestra(clips, personaje, 100, 1000)
        cuerpo = {"voice_description": f"Habla en español. {descripcion}",
                  "model_id": "eleven_multilingual_ttv_v2"}
        if texto:
            cuerpo["text"] = texto
        else:
            cuerpo["auto_generate_text"] = True
        avisar(paso="diseñando en ElevenLabs")
        r = eleven("POST", "/v1/text-to-voice/design", cuerpo=cuerpo,
                   consulta={"output_format": "mp3_44100_128"})
        texto = r.get("text") or texto
        for i, p in enumerate(r.get("previews", []), start=1):
            archivo = destino / f"{i}.mp3"
            archivo.write_bytes(base64.b64decode(p["audio_base_64"]))
            candidatas.append({"id": p["generated_voice_id"], "archivo": f"{i}.mp3",
                               "dur": round(p.get("duration_secs") or 0, 1)})
    elif motor == "qwen":
        # La referencia para clonar después: corta, 8 a 15 segundos.
        texto = texto_de_muestra(clips, personaje, 40, 240) or \
            "Hay cosas que solo se entienden despacio, cuando uno se toma el tiempo de mirarlas bien."
        modelo = qwen_modelo(QWEN_DISENO)
        # Otra tanda cada vez: volver a diseñar con la misma descripción da
        # voces nuevas. La elegida queda guardada como muestra, así que no
        # hace falta poder repetirla.
        base = int(time.time()) % 1_000_000 * 10
        for i in range(1, CANDIDATAS + 1):
            avisar(paso=f"candidata {i} de {CANDIDATAS}", hecho=i - 1, total=CANDIDATAS)
            semilla(base + i)
            ondas, sr = modelo.generate_voice_design(text=texto, language="Spanish",
                                                     instruct=descripcion, max_new_tokens=2048)
            escribir_wav(destino / f"{i}.wav", ondas[0], sr)
            candidatas.append({"id": str(i), "archivo": f"{i}.wav",
                               "dur": round(len(ondas[0]) / sr, 1)})
    else:
        raise ErrorVoz("Solo se diseñan voces con ElevenLabs o con Qwen.")

    info = {"personaje": personaje, "motor": motor, "descripcion": descripcion,
            "texto": texto, "candidatas": candidatas}
    (destino / "disenos.json").write_text(json.dumps(info, ensure_ascii=False, indent=1), encoding="utf-8")
    return {**info, "carpeta": f"disenos/{slug(personaje)}"}


def asignar(carpeta: Path, personaje: str, motor: str, voz: str, voz_nombre: str = "") -> None:
    """Escribe motor y voz del personaje en [voces] de lectura.toml."""
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from lector import escribir_toml
    ruta = carpeta / "lectura.toml"
    datos = tomllib.loads(ruta.read_text(encoding="utf-8"))
    v = datos.setdefault("voces", {}).setdefault(personaje, {})
    v.update(motor=motor, voz=voz)
    if voz_nombre:
        v["voz_nombre"] = voz_nombre
    else:
        v.pop("voz_nombre", None)
    escribir_toml(carpeta, datos)


def elegir(personaje: str, candidato: str) -> dict:
    """Guarda la candidata elegida como la voz del personaje y devuelve su id."""
    d = dir_disenos(personaje)
    try:
        info = json.loads((d / "disenos.json").read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise ErrorVoz(f"No hay diseños de {personaje}: diseña primero.") from None
    c = next((c for c in info["candidatas"] if c["id"] == candidato), None)
    if not c:
        raise ErrorVoz("Esa candidata ya no está; vuelve a diseñar.")
    nombre = f"{personaje} · ESTOC"

    if info["motor"] == "elevenlabs":
        r = eleven("POST", "/v1/text-to-voice", cuerpo={
            "voice_name": nombre, "voice_description": info["descripcion"],
            "generated_voice_id": c["id"], "labels": {"proyecto": "estoc", "personaje": personaje}})
        return {"motor": "elevenlabs", "voz": r["voice_id"], "vozNombre": r.get("name") or nombre}

    voz = f"{slug(personaje)}-{time.strftime('%Y%m%d-%H%M%S')}"
    final = VOCES_DIR / "qwen" / voz
    final.mkdir(parents=True)
    shutil.copy(d / c["archivo"], final / "referencia.wav")
    (final / "referencia.txt").write_text(info["texto"] + "\n", encoding="utf-8")
    (final / "voz.json").write_text(json.dumps({
        "nombre": nombre, "personaje": personaje, "descripcion": info["descripcion"],
        "diseno": QWEN_DISENO, "candidata": candidato, "creada": time.strftime("%Y-%m-%d %H:%M"),
    }, ensure_ascii=False, indent=1), encoding="utf-8")
    return {"motor": "qwen", "voz": voz, "vozNombre": nombre}


# ----------------------------------------------------------------------------
# generar los clips
# ----------------------------------------------------------------------------

def generar(carpeta: Path, solo: list[str] | None = None, forzar: bool = False) -> dict:
    clips, voces = lectura(carpeta)
    por_clip = {c["clip"]: c for c in clips}
    if solo:
        faltan = [s for s in solo if s not in por_clip]
        if faltan:
            raise ErrorVoz(f"No hay clips {', '.join(faltan)} en la lectura.")
    trabajo = []
    for c in clips:
        voz = asignacion(voces, c["personaje"])
        e = estado_clip(carpeta, c, voz)
        if solo is not None and c["clip"] not in solo:
            continue
        if solo is None and not forzar and e["estado"] == "lista":
            continue
        trabajo.append((c, voz))

    hechos, errores = [], []
    sin_voz = [(c, v) for c, v in trabajo if v["motor"] == "manual" or not v["voz"]]
    for c, v in sin_voz:
        motivo = ("se sube a mano" if v["motor"] == "manual"
                  else f"{c['personaje']} no tiene voz elegida")
        errores.append({"clip": c["clip"], "error": motivo})
    trabajo = [t for t in trabajo if t not in sin_voz]
    total = len(trabajo)
    avisar(paso=f"{total} clip(s) por hacer", hecho=0, total=total)

    tmp = Path(tempfile.mkdtemp(prefix="voz-"))
    try:
        k = 0
        for c, v in [t for t in trabajo if t[1]["motor"] == "elevenlabs"]:
            avisar(paso=f"{c['clip']} · ElevenLabs", hecho=k, total=total, clip=c["clip"])
            try:
                crudo = tmp / f"{c['clip']}.mp3"
                eleven_decir(v["voz"], c["texto"], c["personaje"], crudo)
                dur = guardar_clip(carpeta, c, crudo, "elevenlabs", v["voz"], eleven_modelo())
                hechos.append({"clip": c["clip"], "dur": dur})
            except ErrorVoz as e:
                errores.append({"clip": c["clip"], "error": str(e)})
                if "clave" in str(e) or "401" in str(e) or "quota" in str(e).lower():
                    break
            k += 1

        locales = [t for t in trabajo if t[1]["motor"] == "qwen"]
        if locales:
            try:
                modelo = qwen_modelo(QWEN_BASE)
            except ErrorVoz as e:
                errores += [{"clip": c["clip"], "error": str(e)} for c, _ in locales]
                locales = []
            prompts: dict[str, object] = {}
            for c, v in locales:
                avisar(paso=f"{c['clip']} · Qwen", hecho=k, total=total, clip=c["clip"])
                try:
                    if v["voz"] not in prompts:
                        d = qwen_dir(v["voz"])
                        prompts[v["voz"]] = modelo.create_voice_clone_prompt(
                            ref_audio=str(d / "referencia.wav"),
                            ref_text=(d / "referencia.txt").read_text(encoding="utf-8").strip())
                    semilla(zlib.crc32(c["personaje"].encode()) & 0x7FFFFFFF)
                    ondas, sr = modelo.generate_voice_clone(
                        text=c["texto"], language="Spanish",
                        voice_clone_prompt=prompts[v["voz"]], max_new_tokens=2048)
                    crudo = tmp / f"{c['clip']}.wav"
                    escribir_wav(crudo, ondas[0], sr)
                    dur = guardar_clip(carpeta, c, crudo, "qwen", v["voz"], QWEN_BASE)
                    hechos.append({"clip": c["clip"], "dur": dur})
                except ErrorVoz as e:
                    errores.append({"clip": c["clip"], "error": str(e)})
                k += 1
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

    avisar(paso="listo", hecho=total, total=total)
    return {"hechos": hechos, "errores": errores}


def subir(carpeta: Path, clip: str, archivo: Path) -> dict:
    clips, _ = lectura(carpeta)
    c = next((c for c in clips if c["clip"] == clip), None)
    if not c:
        raise ErrorVoz(f"No hay clip {clip} en la lectura.")
    if not archivo.exists():
        raise ErrorVoz(f"No encuentro {archivo}.")
    dur = guardar_clip(carpeta, c, archivo, "manual", "")
    return {"clip": clip, "dur": dur}


def motores() -> dict:
    clave, origen = clave_eleven()
    qwen = {"instalado": qwen_instalado(), "dispositivo": ""}
    if qwen["instalado"]:
        try:
            qwen["dispositivo"] = qwen_dispositivo()[0]
        except Exception as e:  # torch roto o sin GPU: se informa, no se cae
            qwen["error"] = str(e)
    return {"elevenlabs": {"clave": bool(clave), "origen": origen,
                           "final": clave[-4:] if clave else "", "modelo": eleven_modelo()},
            "qwen": qwen}


# ----------------------------------------------------------------------------

def main() -> int:
    global JSON
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--json", action="store_true", help="responder JSON (lo usa el estudio)")
    sub = ap.add_subparsers(dest="orden", required=True)

    sub.add_parser("estado", help="personajes, su voz y el estado de cada clip").add_argument("carpeta")
    sub.add_parser("motores", help="qué motores hay: clave de ElevenLabs, Qwen instalado")
    sub.add_parser("cuenta", help="caracteres usados y disponibles en ElevenLabs")
    v = sub.add_parser("voces", help="las voces que se pueden elegir")
    v.add_argument("--motor", choices=["elevenlabs", "qwen"], default="elevenlabs")

    d = sub.add_parser("disenar", help="tres voces candidatas para un personaje")
    d.add_argument("carpeta")
    d.add_argument("--personaje", required=True)
    d.add_argument("--motor", choices=["elevenlabs", "qwen"], required=True)
    d.add_argument("--descripcion", default="", help="si no, la de lectura.toml")

    e = sub.add_parser("elegir", help="quedarse con una candidata como la voz del personaje")
    e.add_argument("carpeta", nargs="?")
    e.add_argument("--personaje", required=True)
    e.add_argument("--candidato", required=True)

    g = sub.add_parser("generar", help="hacer los clips que faltan o cambiaron")
    g.add_argument("carpeta")
    g.add_argument("--clips", default="", help="solo estos, separados por comas (se rehacen)")
    g.add_argument("--forzar", action="store_true", help="rehacerlos todos")

    s = sub.add_parser("subir", help="usar un audio propio para un clip")
    s.add_argument("carpeta")
    s.add_argument("--clip", required=True)
    s.add_argument("archivo")

    a = ap.parse_args()
    JSON = a.json
    try:
        if a.orden == "estado":
            r = estado(Path(a.carpeta))
            filas = [f"  {c['clip']:<18} {c['estado']:<9} {c['texto'][:60]}" for c in r["clips"]]
            pers = [f"  {p['nombre']}: {p['motor']} {p['vozNombre'] or p['voz'] or '(sin voz)'}"
                    for p in r["personajes"]]
            responder(r, "\n".join(pers + [""] + filas + ["", f"  Pendientes: {r['pendientes']}"]))
        elif a.orden == "motores":
            responder(motores())
        elif a.orden == "cuenta":
            r = eleven_cuenta()
            responder(r, f"  ElevenLabs ({r['nivel']}): {r['usados']} de {r['limite']} caracteres usados")
        elif a.orden == "voces":
            r = eleven_voces() if a.motor == "elevenlabs" else qwen_voces()
            responder(r, "\n".join(f"  {x['id']:<24} {x['nombre']} ({x['categoria']})" for x in r))
        elif a.orden == "disenar":
            r = disenar(Path(a.carpeta), a.personaje, a.motor, a.descripcion)
            responder(r, "\n".join(
                [f"  Texto: {r['texto']}", ""] +
                [f"  {c['id']}: {VOCES_DIR / r['carpeta'] / c['archivo']}" for c in r["candidatas"]] +
                ["", f"  Escúchalas y elige: ./estudio voces elegir {a.carpeta} "
                     f"--personaje \"{a.personaje}\" --candidato ID"]))
        elif a.orden == "elegir":
            r = elegir(a.personaje, a.candidato)
            if a.carpeta:
                asignar(Path(a.carpeta), a.personaje, r["motor"], r["voz"], r["vozNombre"])
            responder(r, f"  {a.personaje}: {r['motor']} {r['voz']}" +
                      (" (escrito en lectura.toml)" if a.carpeta else ""))
        elif a.orden == "generar":
            solo = [x.strip() for x in a.clips.split(",") if x.strip()] or None
            r = generar(Path(a.carpeta), solo, a.forzar)
            responder(r, "\n".join([f"  Hechos: {len(r['hechos'])}"] +
                                   [f"  {e['clip']}: {e['error']}" for e in r["errores"]]))
        elif a.orden == "subir":
            r = subir(Path(a.carpeta), a.clip, Path(a.archivo).expanduser())
            responder(r, f"  {r['clip']}: {r['dur']} s")
    except ErrorVoz as e:
        if JSON:
            print(json.dumps({"error": str(e)}, ensure_ascii=False), flush=True)
        else:
            print(f"  {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
