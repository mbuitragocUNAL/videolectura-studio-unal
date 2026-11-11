#!/usr/bin/env python3
"""Arma video/index.html, la composición HyperFrames de la guía.

    python3 guia/construir.py && (cd guia/video && npx hyperframes render -o ../guia.mp4)

Los tiempos salen de la narración: video/assets/narracion/duraciones.json
(narrar.py) da cuánto dura cada escena, y lo que aparece en pantalla se ancla a
la palabra que lo nombra (MARCAS, segundos dentro de la narración de cada
escena, medidos con Whisper). Si se vuelve a generar la narración, basta con
volver a correr esto.
"""
from __future__ import annotations

import json
from pathlib import Path

AQUI = Path(__file__).resolve().parent
VIDEO = AQUI / "video"
REPO = "github.com/mbuitragocUNAL/videolectura-studio-unal"

ENTRADA, SALIDA_ESCENA = 0.5, 0.9      # aire antes y después de cada narración
W, H = 1920, 1080
# El marco donde se ven las capturas y los videos.
FX, FY, FW, FH = 600, 200, 1244, 700

dur = json.loads((VIDEO / "assets" / "narracion" / "duraciones.json").read_text())
inicio, t = [], 0.0
for d in dur:
    inicio.append(round(t, 3))
    t += ENTRADA + d + SALIDA_ESCENA
TOTAL = round(t + 0.6, 2)


def en(k: int, s: float) -> float:
    """Segundo absoluto de la palabra que suena s segundos dentro de la escena k (1..8)."""
    return round(inicio[k - 1] + ENTRADA + s, 3)


def fin(k: int) -> float:
    return round(inicio[k - 1] + ENTRADA + dur[k - 1] + SALIDA_ESCENA, 3)


def enfoque(cx: float, cy: float, s: float) -> dict:
    """x, y, scale para que el punto (cx, cy) de una captura 1920×1080 quede al
    centro del marco, a escala s."""
    return {"x": round(FW / 2 - cx * s, 1), "y": round(FH / 2 - cy * s, 1), "scale": s}


ENTERA = enfoque(960, 540, FW / 1920)

escenas_html = []
anim = []          # líneas de GSAP


def escena(k: int, cuerpo: str, clase: str = "") -> None:
    a, b = inicio[k - 1], fin(k)
    escenas_html.append(
        f'<section id="e{k}" class="clip escena {clase}" data-start="{a}" data-duration="{round(b - a, 3)}" '
        f'data-track-index="1">{cuerpo}</section>')
    anim.append(f'tl.fromTo("#e{k}", {{opacity: 0}}, {{opacity: 1, duration: 0.45, ease: "power2.out"}}, {a});')
    anim.append(f'tl.to("#e{k}", {{opacity: 0, duration: 0.35, ease: "power2.in"}}, {round(b - 0.35, 3)});')


def aparece(sel: str, t: float, y: int = 18, dur: float = 0.5) -> None:
    anim.append(f'tl.fromTo("{sel}", {{opacity: 0, y: {y}}}, {{opacity: 1, y: 0, duration: {dur}, ease: "power3.out"}}, {t});')


def encabezado(kicker: str, titulo: str) -> str:
    return f'<div class="encabezado"><div class="kicker">{kicker}</div><h2>{titulo}</h2></div>'


def barra() -> str:
    return '<div class="barra">' + "".join(f'<i style="background:var(--c{i})"></i>' for i in range(5)) + "</div>"


# 1 · Portada ----------------------------------------------------------------
escena(1, f'''
  <div class="sello"><img src="assets/marca/escudo-unal-blanco.png" alt="">
    <div><div class="espaciado">UNIVERSIDAD NACIONAL DE COLOMBIA</div>
    <div class="apagado">Modelos Estocásticos y Simulación · ME01</div></div></div>
  <div class="portada">
    <div class="kicker" id="p-kicker">GUÍA</div>
    <h1 id="p-titulo">Videolectura Studio</h1>
    <div class="unal" id="p-unal">UNAL</div>
    <div id="p-barra">{barra()}</div>
    <p class="lema" id="p-lema">Graba tu video-lectura. El estudio se encarga de los lineamientos.</p>
  </div>''')
for i, sel in enumerate(["#p-kicker", "#p-titulo", "#p-unal", "#p-barra", "#p-lema"]):
    aparece(sel, round(inicio[0] + 0.15 + 0.22 * i, 3))

# 2 · Las reglas -------------------------------------------------------------
reglas = [("2.c", "La página llena la pantalla", 3.1, 0),
          ("2.d", "Cada figura, sola con su pie, la primera vez", 5.9, 1),
          ("2.d", "Una flecha marca dónde sigue la lectura", 9.7, 2),
          ("2.f", "La figura fija a la derecha, el texto del mismo tamaño", 14.1, 3),
          ("2.i", "Citas con voz sintética, una voz por personaje", 17.5, 4)]
filas = "".join(f'<li id="r{i}"><span class="cod" style="background:var(--c{c})">{cod}</span>{txt}</li>'
                for i, (cod, txt, _, c) in enumerate(reglas))
escena(2, encabezado("LINEAMIENTOS 3.2.3", "Lo que pide una video-lectura") + f'''
  <ul class="reglas">{filas}</ul>
  <div class="veredicto"><span id="v-mano">A mano, en un editor: horas.</span>
  <span id="v-aqui">Aquí: <b>automático</b>.</span></div>''')
for i, (_, _, s, _) in enumerate(reglas):
    aparece(f"#r{i}", en(2, s), y=0)
    anim.append(f'tl.from("#r{i}", {{x: -30, duration: 0.5, ease: "power3.out"}}, {en(2, s)});')
aparece("#v-mano", en(2, 21.5))
aparece("#v-aqui", en(2, 24.5))

# 3 · Instalar -----------------------------------------------------------------
lineas = [("$ ", f"git clone https://{REPO}", 3.0),
          ("$ ", "cd videolectura-studio-unal", 4.3),
          ("$ ", "./estudio abrir", 5.2),
          ("", "  Estudio en http://localhost:8100", 7.0)]
term = "".join(f'<div class="tl" id="t{i}"><span class="pr">{p}</span><span class="tx" id="tx{i}">{x}</span></div>'
               for i, (p, x, _) in enumerate(lineas))
escena(3, encabezado("INSTALAR", "Solo Docker y Google Chrome") + f'''
  <div class="chips"><span class="chip" id="ch-docker">Docker</span><span class="chip" id="ch-chrome">Google Chrome</span></div>
  <div class="terminal"><div class="puntos"><i></i><i></i><i></i></div>{term}</div>''')
aparece("#ch-docker", en(3, 0.6))
aparece("#ch-chrome", en(3, 1.4))
for i, (_, x, s) in enumerate(lineas):
    n = len(x) + 1
    anim.append(f'tl.set("#t{i}", {{opacity: 1}}, {en(3, s)});')
    anim.append(f'tl.fromTo("#tx{i}", {{width: "0ch"}}, {{width: "{n}ch", duration: {round(min(1.4, n * 0.035), 2)}, ease: "steps({n})"}}, {en(3, s)});')


# Capturas con enfoques ---------------------------------------------------------
def marco(id_: str, imagenes: list[tuple[str, str]]) -> str:
    imgs = "".join(f'<img id="{i}" class="captura" src="assets/capturas/{f}.png" alt="">' for i, f in imagenes)
    return f'<div class="marco" id="{id_}">{imgs}</div>'


def pasos(id_: str, items: list[str]) -> str:
    return f'<ol class="pasos" id="{id_}">' + "".join(
        f'<li id="{id_}-{i}"><span>{i + 1}</span>{t}</li>' for i, t in enumerate(items)) + "</ol>"


def mover(sel: str, f: dict, t: float, d: float = 1.1) -> None:
    anim.append(f'tl.to("{sel}", {{x: {f["x"]}, y: {f["y"]}, scale: {f["scale"]}, duration: {d}, ease: "power2.inOut"}}, {t});')


def poner(sel: str, f: dict, t: float) -> None:
    anim.append(f'tl.set("{sel}", {{x: {f["x"]}, y: {f["y"]}, scale: {f["scale"]}}}, {t});')


def mostrar(sel: str, t: float, d: float = 0.4) -> None:
    anim.append(f'tl.to("{sel}", {{opacity: 1, duration: {d}}}, {t});')


def ocultar(sel: str, t: float, d: float = 0.4) -> None:
    anim.append(f'tl.to("{sel}", {{opacity: 0, duration: {d}}}, {t});')


def activar(lista: str, i: int, t: float, n: int) -> None:
    for j in range(n):
        anim.append(f'tl.to("#{lista}-{j}", {{opacity: {1 if j == i else 0.38}, duration: 0.3}}, {t});')


# 4 · Preparar -------------------------------------------------------------------
escena(4, encabezado("PASO 1", "Preparar la lectura") +
       pasos("pp", ["Libro y capítulo", "Dónde empieza y termina", "Figuras y láminas",
                    "Citas y voces", "Música de fondo"]) +
       marco("m4", [("c4a", "preparar-libro"), ("c4b", "preparar-figuras"), ("c4c", "preparar-citas")]))
a4 = inicio[3]
for i in ("#c4a", "#c4b", "#c4c"):
    poner(i, ENTERA, a4)
anim.append(f'tl.set("#c4b, #c4c", {{opacity: 0}}, {a4});')
activar("pp", 0, a4, 5)
mover("#c4a", enfoque(410, 250, 1.5), en(4, 0.6))
activar("pp", 1, en(4, 5.0), 5)
mover("#c4a", enfoque(400, 720, 1.45), en(4, 5.0))
activar("pp", 2, en(4, 9.4), 5)
poner("#c4b", enfoque(450, 380, 1.35), en(4, 9.0))
mostrar("#c4b", en(4, 9.4))
activar("pp", 3, en(4, 16.0), 5)
poner("#c4c", enfoque(390, 430, 1.55), en(4, 15.6))
mostrar("#c4c", en(4, 16.0))
activar("pp", 4, en(4, 24.8), 5)
mover("#c4c", enfoque(390, 905, 1.55), en(4, 24.8))

# 5 · Grabar ---------------------------------------------------------------------
teclas = [("R", "grabar", 4.3), ("S", "parar", 7.0), ("Av Pág", "el siguiente párrafo", 9.6),
          ("N", "la cita, por los audífonos", 15.2)]
tk = "".join(f'<li id="k{i}"><kbd>{k}</kbd>{t}</li>' for i, (k, t, _) in enumerate(teclas))
escena(5, encabezado("PASO 2", "Grabar: un párrafo, una toma") + f'<ul class="teclas">{tk}</ul>'
       + '<div class="nota" id="k-nota">¿Te equivocaste? La toma nueva reemplaza a la anterior.</div>'
       + marco("m5", [("c5", "consola")]))
poner("#c5", ENTERA, inicio[4])
mover("#c5", enfoque(700, 560, 1.0), en(5, 1.0))
mover("#c5", enfoque(1735, 540, 1.6), en(5, 4.0))
for i, (_, _, s) in enumerate(teclas):
    aparece(f"#k{i}", en(5, s), y=0)
aparece("#k-nota", en(5, 10.7))
mover("#c5", enfoque(1735, 610, 1.6), en(5, 14.8))

# 6 · Render -----------------------------------------------------------------------
# Los fragmentos de la lectura 22: (archivo, inicio en la escena, fin, desde dónde del archivo)
cortes = [("texto", 0.0, 8.04, 0.0), ("figura", 8.04, 14.6, 0.0),
          ("flecha", 14.6, 17.4, 1.6), ("fija", 17.4, dur[5] + ENTRADA + SALIDA_ESCENA, 0.0)]
rotulos = [("El texto corre con tu voz", 5.3, 0), ("La figura, sola con su pie (2.d)", 11.9, 1),
           ("Flecha de regreso (2.d)", 15.0, 2), ("Figura fija · mismo tamaño (2.f)", 17.6, 3),
           ("Sin silencios largos · volumen parejo", 21.5, 4)]
rt = "".join(f'<li id="g{i}" style="--c:var(--c{c})">{txt}</li>' for i, (txt, _, c) in enumerate(rotulos))
escena(6, encabezado("PASO 3", "El render") + f'<ul class="rotulos">{rt}</ul><div class="marco" id="m6"></div>')
for i, (_, s, _) in enumerate(rotulos):
    aparece(f"#g{i}", en(6, s), y=0)
videos = []
for nombre, a, b, desde in cortes:
    ta = round(inicio[5] + a, 3)
    videos.append(f'<video id="v-{nombre}" class="clip fragmento" src="assets/clips/{nombre}.mp4" muted playsinline '
                  f'data-start="{ta}" data-duration="{round(b - a, 3)}" data-media-start="{desde}" '
                  f'data-track-index="2"></video>')

# 7 · Entrega ---------------------------------------------------------------------
escena(7, encabezado("PASO 4", "La entrega") + '''
  <img class="miniatura" id="mini" src="assets/marca/miniatura-lectura22.png" alt="">
  <div class="entrega">
    <div class="titulo-yt" id="yt">ME01 Lectura 22 Belleza cuántica II: exuberancia</div>
    <div class="chips"><span class="chip" id="ent1">1080p</span><span class="chip" id="ent2">título del curso (2.k)</span>
    <span class="chip" id="ent3">descripción</span><span class="chip" id="ent4">miniatura</span></div>
  </div>''')
aparece("#mini", en(7, 0.3))
aparece("#yt", en(7, 3.0))
for i, s in enumerate([1.8, 3.6, 5.6, 6.6]):
    aparece(f"#ent{i + 1}", en(7, s))

# 8 · Cierre ----------------------------------------------------------------------
escena(8, f'''
  <div class="cierre">
    <img src="assets/marca/escudo-unal-blanco.png" class="escudo-grande" alt="">
    <div class="kicker" id="c-k">VIDEOLECTURA STUDIO · UNAL</div>
    <div class="repo" id="c-repo">{REPO}</div>
    {barra()}
    <p class="lema" id="c-nota">Hecho por estudiantes · no es una herramienta oficial de la UNAL</p>
    <p class="creditos" id="c-cred">Narración: Qwen3-TTS · Música: «Bossa Antigua», Kevin MacLeod (incompetech.com), CC BY 4.0</p>
  </div>''')
aparece("#c-k", en(8, 0.2))
aparece("#c-repo", en(8, 0.8))
aparece("#c-nota", en(8, 6.5))
aparece("#c-cred", en(8, 8.0))

audios = [f'<audio id="narracion-{k + 1}" src="assets/narracion/escena-{k + 1}.wav" data-start="{round(inicio[k] + ENTRADA, 3)}" '
          f'data-duration="{dur[k]}" data-track-index="3"></audio>' for k in range(len(dur))]
audios.append(f'<audio id="musica" src="assets/musica/bossa-antigua.mp3" data-start="0" data-duration="{TOTAL}" '
              f'data-volume="0.55" data-fade-in="1.5" data-fade-out="3" loop data-track-index="4"></audio>')

html = f'''<!doctype html>
<html lang="es" data-resolution="landscape">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width={W}, height={H}" />
<script src="https://cdn.jsdelivr.net/npm/gsap@3.15.0/dist/gsap.min.js"></script>
<style>
@font-face {{ font-family: Plex; src: url(assets/marca/IBMPlexSans-Regular.ttf); font-weight: 400; }}
@font-face {{ font-family: Plex; src: url(assets/marca/IBMPlexSans-Medium.ttf); font-weight: 500; }}
@font-face {{ font-family: Plex; src: url(assets/marca/IBMPlexSans-SemiBold.ttf); font-weight: 600; }}
:root {{
  --fondo: #10131A; --tinta: #ECE6D8; --apagado: #8A8F9C; --dorado: #F2B84B;
  --c0: #5DA9E9; --c1: #F2B84B; --c2: #A78BFA; --c3: #4FC1A6; --c4: #E8706A;
}}
* {{ margin: 0; padding: 0; box-sizing: border-box; }}
html, body {{ width: {W}px; height: {H}px; overflow: hidden; background: var(--fondo); }}
#root {{ position: relative; width: 100%; height: 100%; font-family: Plex, sans-serif; color: var(--tinta); }}
#nube {{ position: absolute; inset: 0; }}
.escena {{ position: absolute; inset: 0; }}
.sello {{ position: absolute; left: 80px; top: 64px; display: flex; gap: 28px; align-items: center; }}
.sello img {{ height: 92px; }}
.espaciado {{ letter-spacing: 0.32em; font-size: 22px; font-weight: 500; color: var(--apagado); }}
.apagado {{ color: var(--apagado); font-size: 28px; margin-top: 6px; }}
.kicker {{ letter-spacing: 0.28em; font-size: 30px; font-weight: 500; color: var(--dorado); }}
.portada {{ position: absolute; left: 112px; top: 300px; }}
.portada h1 {{ font-size: 168px; font-weight: 600; letter-spacing: -0.02em; line-height: 1.05; margin-top: 18px; }}
.unal {{ font-size: 64px; font-weight: 600; letter-spacing: 0.3em; color: var(--dorado); margin-top: 4px; }}
.barra {{ display: flex; gap: 8px; margin: 34px 0 30px; }}
.barra i {{ width: 112px; height: 10px; display: block; }}
.lema {{ font-size: 40px; color: var(--apagado); }}
.encabezado {{ position: absolute; left: 80px; top: 70px; }}
.encabezado h2 {{ font-size: 64px; font-weight: 600; margin-top: 10px; letter-spacing: -0.01em; }}
.reglas {{ position: absolute; left: 112px; top: 300px; list-style: none; display: grid; gap: 30px; }}
.reglas li {{ font-size: 44px; display: flex; align-items: center; gap: 30px; }}
.cod {{ font-size: 28px; font-weight: 600; color: var(--fondo); padding: 6px 16px; border-radius: 8px; min-width: 92px; text-align: center; }}
.veredicto {{ position: absolute; left: 112px; top: 870px; font-size: 42px; display: flex; gap: 60px; }}
#v-mano {{ color: var(--apagado); text-decoration: line-through; }}
#v-aqui b {{ color: var(--dorado); font-weight: 600; }}
.chips {{ display: flex; gap: 16px; flex-wrap: wrap; }}
.escena > .chips {{ position: absolute; left: 80px; top: 250px; }}
.chip {{ font-size: 30px; padding: 10px 24px; border: 2px solid #2b323b; border-radius: 999px; background: #161a22; }}
.terminal {{ position: absolute; left: 80px; top: 380px; width: 1760px; background: #0a0c10; border: 2px solid #2b323b;
  border-radius: 18px; padding: 34px 44px 44px; font-family: Menlo, monospace; font-size: 40px; line-height: 1.75; }}
.puntos {{ display: flex; gap: 12px; margin-bottom: 18px; }}
.puntos i {{ width: 18px; height: 18px; border-radius: 50%; background: #2b323b; display: block; }}
.tl {{ opacity: 0; white-space: pre; display: flex; }}
.pr {{ color: var(--c3); }}
.tx {{ display: inline-block; overflow: hidden; white-space: pre; vertical-align: bottom; }}
#t3 .tx {{ color: var(--dorado); }}
.marco {{ position: absolute; left: {FX}px; top: {FY}px; width: {FW}px; height: {FH}px; overflow: hidden;
  border-radius: 16px; border: 2px solid #2b323b; background: #0d0f12; box-shadow: 0 30px 80px rgba(0,0,0,.45); }}
.captura {{ position: absolute; left: 0; top: 0; width: 1920px; height: 1080px; transform-origin: 0 0; }}
.pasos, .teclas, .rotulos {{ position: absolute; left: 80px; top: 250px; width: 470px; list-style: none; display: grid; gap: 26px; }}
.pasos li {{ font-size: 34px; display: flex; gap: 20px; align-items: center; }}
.pasos span {{ width: 50px; height: 50px; border-radius: 50%; background: var(--dorado); color: var(--fondo);
  display: grid; place-items: center; font-weight: 600; font-size: 26px; flex: none; }}
.teclas li {{ font-size: 34px; display: flex; gap: 22px; align-items: center; opacity: 0; }}
kbd {{ font-family: Plex, sans-serif; font-weight: 600; font-size: 30px; min-width: 64px; padding: 8px 16px; text-align: center;
  border: 2px solid var(--dorado); color: var(--dorado); border-radius: 10px; background: #161a22; flex: none; }}
.nota {{ position: absolute; left: 80px; top: 760px; width: 470px; font-size: 30px; color: var(--apagado); opacity: 0; }}
.rotulos li {{ font-size: 32px; padding-left: 22px; border-left: 6px solid var(--c); opacity: 0; line-height: 1.25; }}
.fragmento {{ position: absolute; left: {FX + 2}px; top: {FY + 2}px; width: {FW - 4}px; height: {FH - 4}px;
  object-fit: cover; border-radius: 14px; z-index: 5; }}
.miniatura {{ position: absolute; left: 80px; top: 250px; width: 960px; border-radius: 14px; border: 2px solid #2b323b; }}
.entrega {{ position: absolute; left: 1100px; top: 270px; width: 740px; display: grid; gap: 36px; }}
.titulo-yt {{ font-family: Menlo, monospace; font-size: 34px; line-height: 1.4; padding: 26px; background: #0a0c10;
  border: 2px solid #2b323b; border-radius: 14px; }}
.cierre {{ position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; }}
.cierre .barra {{ justify-content: center; }}
.escudo-grande {{ height: 150px; margin-bottom: 40px; }}
.repo {{ font-family: Menlo, monospace; font-size: 54px; margin-top: 26px; }}
.creditos {{ font-size: 24px; color: #5d6370; margin-top: 50px; }}
</style>
</head>
<body>
<div id="root" data-composition-id="main" data-start="0" data-duration="{TOTAL}" data-width="{W}" data-height="{H}">
<canvas id="nube" width="{W}" height="{H}"></canvas>
{chr(10).join(escenas_html)}
{chr(10).join(videos)}
{chr(10).join(audios)}
</div>
<script>
// La nube de puntos de la miniatura, con un generador fijo: el mismo dibujo en cada cuadro.
(function () {{
  const c = document.getElementById("nube").getContext("2d");
  let s = 22;
  const r = () => (s = (s * 1103515245 + 12345) % 2147483648) / 2147483648;
  const g = () => (r() + r() + r() - 1.5) / 1.5;
  const centros = [[0.12, 0.2, 0.16], [0.86, 0.75, 0.18], [0.5, 1.0, 0.12]];
  const colores = ["#5DA9E9", "#F2B84B", "#A78BFA", "#4FC1A6", "#E8706A"];
  for (let i = 0; i < 520; i++) {{
    const [cx, cy, sp] = centros[i % 5 < 2 ? 0 : i % 5 < 4 ? 1 : 2];
    const x = (cx + g() * sp) * {W}, y = (cy + g() * sp * 1.6) * {H};
    c.globalAlpha = 0.12 + r() * 0.3;
    c.fillStyle = colores[Math.floor(r() * 5)];
    c.beginPath(); c.arc(x, y, 1.2 + r() * 2.2, 0, Math.PI * 2); c.fill();
  }}
}})();
const tl = gsap.timeline({{ paused: true }});
{chr(10).join(anim)}
window.__timelines = window.__timelines || {{}};
window.__timelines["main"] = tl;
tl.seek(0);
</script>
</body>
</html>
'''
(VIDEO / "index.html").write_text(html, encoding="utf-8")
print(f"index.html: {len(dur)} escenas, {TOTAL} s")
