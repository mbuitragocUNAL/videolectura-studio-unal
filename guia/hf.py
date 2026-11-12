"""Piezas para armar los videos de cada paso con HyperFrames.

Cada video vive en guia/pasos/<n>-<nombre>/: su guion.md (la narración, escena
por escena), su video.py (lo que se ve en cada escena, con estas piezas) y,
después de narrar.py, video/assets/narracion/ con el audio de cada escena y
cuándo se dice cada palabra. Lo que aparece en pantalla se ancla a una
palabra de la narración: `v.en(3, "grabar")` es el instante en que la escena 3
dice «grabar». Si se vuelve a narrar, el video se vuelve a armar solo.

    preparar/.venv/bin/python guia/narrar.py generar --voz 2 --carpeta guia/pasos/1-instalar
    python3 guia/pasos/1-instalar/video.py
    (cd guia/pasos/1-instalar/video && npx hyperframes render -o ../video.mp4)
"""
from __future__ import annotations

import json
import re
import shutil
import unicodedata
from pathlib import Path

GUIA = Path(__file__).resolve().parent
COMPARTIDO = GUIA / "compartido"      # capturas, fragmentos (fuera de git)
PLANTILLA = GUIA / "video"            # el proyecto HyperFrames de la guía general
REPO = "github.com/mbuitragocUNAL/videolectura-studio-unal"
W, H = 1920, 1080
ENTRADA, SALIDA = 0.5, 0.9            # aire antes y después de cada narración
FX, FY, FW, FH = 600, 200, 1244, 700  # el marco de capturas y fragmentos


def _norm(s: str) -> str:
    s = unicodedata.normalize("NFKD", s.lower()).encode("ascii", "ignore").decode()
    return re.sub(r"[^a-z0-9]", "", s)


class Video:
    def __init__(self, carpeta: Path, paso: int, nombre: str):
        self.carpeta, self.paso, self.nombre = Path(carpeta), paso, nombre
        self.dir = self.carpeta / "video"
        narr = self.dir / "assets" / "narracion"
        self.dur = json.loads((narr / "duraciones.json").read_text())
        self.palabras = [json.loads((narr / f"palabras-{k}.json").read_text()) for k in range(1, len(self.dur) + 1)]
        self.inicio, t = [], 0.0
        for d in self.dur:
            self.inicio.append(round(t, 3))
            t += ENTRADA + d + SALIDA
        self.total = round(t + 0.6, 2)
        self.html, self.anim, self.medios, self.usados = [], [], [], set()
        self._cursor = {}

    # ---- tiempo ----------------------------------------------------------
    def en(self, k: int, palabra: str | float) -> float:
        """El instante en que la escena k dice `palabra` (la siguiente vez
        después de la anterior que se pidió en esa escena), o, si es un
        número, ese segundo de la narración."""
        if isinstance(palabra, (int, float)):
            return round(self.inicio[k - 1] + ENTRADA + palabra, 3)
        lista = self.palabras[k - 1]
        quiero = [_norm(p) for p in palabra.split()]
        desde = self._cursor.get(k, 0)
        for orden in (range(desde, len(lista)), range(0, len(lista))):
            for i in orden:
                if all(i + j < len(lista) and _norm(lista[i + j]["w"]).startswith(q)
                       for j, q in enumerate(quiero)):
                    self._cursor[k] = i + 1
                    return round(self.inicio[k - 1] + ENTRADA + lista[i]["a"], 3)
        print(f"  aviso: la escena {k} no dice «{palabra}»; uso su mitad")
        return round(self.inicio[k - 1] + ENTRADA + self.dur[k - 1] / 2, 3)

    def fin(self, k: int) -> float:
        return round(self.inicio[k - 1] + ENTRADA + self.dur[k - 1] + SALIDA, 3)

    # ---- animación ---------------------------------------------------------
    def aparece(self, sel: str, t: float, y: int = 18) -> None:
        self.anim.append(f'tl.fromTo("{sel}", {{opacity: 0, y: {y}}}, {{opacity: 1, y: 0, duration: 0.5, ease: "power3.out"}}, {t});')

    def _escena(self, k: int, cuerpo: str) -> None:
        a, b = self.inicio[k - 1], self.fin(k)
        self.html.append(f'<section id="e{k}" class="clip escena" data-start="{a}" data-duration="{round(b - a, 3)}" '
                         f'data-track-index="1">{cuerpo}</section>')
        self.anim.append(f'tl.fromTo("#e{k}", {{opacity: 0}}, {{opacity: 1, duration: 0.45, ease: "power2.out"}}, {a});')
        self.anim.append(f'tl.to("#e{k}", {{opacity: 0, duration: 0.35, ease: "power2.in"}}, {round(b - 0.35, 3)});')

    @staticmethod
    def _encabezado(kicker: str, titulo: str) -> str:
        return f'<div class="encabezado"><div class="kicker">{kicker}</div><h2>{titulo}</h2></div>'

    @staticmethod
    def _barra() -> str:
        return '<div class="barra">' + "".join(f'<i style="background:var(--c{i})"></i>' for i in range(5)) + "</div>"

    def _usar(self, ruta: str) -> str:
        self.usados.add(ruta)
        return f"assets/{ruta}"

    # ---- escenas -------------------------------------------------------------
    def portada(self, k: int, titulo: str, lema: str) -> None:
        self._escena(k, f'''
  <div class="sello"><img src="{self._usar("marca/escudo-unal-blanco.png")}" alt="">
    <div><div class="espaciado">VIDEOLECTURA STUDIO · UNAL</div>
    <div class="apagado">Modelos Estocásticos y Simulación · ME01</div></div></div>
  <div class="portada">
    <div class="kicker" id="p{k}-a">PASO {self.paso} DE 6</div>
    <h1 id="p{k}-b">{titulo}</h1>
    <div id="p{k}-c">{self._barra()}</div>
    <p class="lema" id="p{k}-d">{lema}</p>
  </div>''')
        for i, s in enumerate("abcd"):
            self.aparece(f"#p{k}-{s}", round(self.inicio[k - 1] + 0.15 + 0.22 * i, 3))

    def lista(self, k: int, kicker: str, titulo: str, items: list[tuple[str, str, str | float]],
              pie: tuple[str, str | float] | None = None) -> None:
        """items: (insignia, texto, palabra). La insignia puede ser vacía."""
        filas = "".join(
            f'<li id="l{k}-{i}">' + (f'<span class="cod" style="background:var(--c{i % 5})">{b}</span>' if b else
                                      f'<span class="punto" style="background:var(--c{i % 5})"></span>') + f'{t}</li>'
            for i, (b, t, _) in enumerate(items))
        extra = f'<div class="veredicto" id="l{k}-pie">{pie[0]}</div>' if pie else ""
        self._escena(k, self._encabezado(kicker, titulo) + f'<ul class="reglas">{filas}</ul>{extra}')
        for i, (_, _, p) in enumerate(items):
            t = self.en(k, p)
            self.anim.append(f'tl.fromTo("#l{k}-{i}", {{opacity: 0, x: -30}}, {{opacity: 1, x: 0, duration: 0.5, ease: "power3.out"}}, {t});')
        if pie:
            self.aparece(f"#l{k}-pie", self.en(k, pie[1]))

    def terminal(self, k: int, kicker: str, titulo: str, lineas: list[tuple[str, str, str | float]],
                 chips: list[tuple[str, str | float]] = ()) -> None:
        """lineas: (prompt, texto, palabra)."""
        term = "".join(f'<div class="tl" id="t{k}-{i}"><span class="pr">{p}</span>'
                       f'<span class="tx" id="tx{k}-{i}">{x}</span></div>' for i, (p, x, _) in enumerate(lineas))
        ch = "".join(f'<span class="chip" id="ch{k}-{i}">{c}</span>' for i, (c, _) in enumerate(chips))
        top = 380 if chips else 280
        self._escena(k, self._encabezado(kicker, titulo) +
                     (f'<div class="chips">{ch}</div>' if chips else "") +
                     f'<div class="terminal" style="top:{top}px"><div class="puntos"><i></i><i></i><i></i></div>{term}</div>')
        for i, (_, p) in enumerate(chips):
            self.aparece(f"#ch{k}-{i}", self.en(k, p))
        for i, (_, x, p) in enumerate(lineas):
            t, n = self.en(k, p), len(x) + 1
            self.anim.append(f'tl.set("#t{k}-{i}", {{opacity: 1}}, {t});')
            self.anim.append(f'tl.fromTo("#tx{k}-{i}", {{width: "0ch"}}, {{width: "{n}ch", duration: {round(min(1.4, n * 0.035), 2)}, ease: "steps({n})"}}, {t});')

    @staticmethod
    def enfoque(cx: float, cy: float, s: float) -> dict:
        return {"x": round(FW / 2 - cx * s, 1), "y": round(FH / 2 - cy * s, 1), "scale": s}

    def captura(self, k: int, kicker: str, titulo: str, imagenes: list[tuple[str, list[tuple]]],
                lado: tuple[str, list[tuple[str, str, str | float]]] | None = None,
                nota: tuple[str, str | float] | None = None) -> None:
        """imagenes: (captura, [(palabra|0, cx, cy, escala), ...]) en orden; la
        primera palabra de cada imagen es cuando entra. lado: ("pasos"|"teclas"
        |"rotulos", [(tecla o "", texto, palabra), ...])."""
        ids = [f"c{k}-{i}" for i in range(len(imagenes))]
        imgs = "".join(f'<img id="{i}" class="captura" src="{self._usar("capturas/" + f + ".png")}" alt="">'
                       for i, (f, _) in zip(ids, imagenes))
        lateral = ""
        if lado:
            tipo, items = lado
            if tipo == "pasos":
                lateral = '<ol class="pasos">' + "".join(
                    f'<li id="s{k}-{i}"><span>{i + 1}</span>{t}</li>' for i, (_, t, _) in enumerate(items)) + "</ol>"
            elif tipo == "teclas":
                lateral = '<ul class="teclas">' + "".join(
                    f'<li id="s{k}-{i}"><kbd>{c}</kbd>{t}</li>' for i, (c, t, _) in enumerate(items)) + "</ul>"
            else:
                lateral = '<ul class="rotulos">' + "".join(
                    f'<li id="s{k}-{i}" style="--c:var(--c{i % 5})">{t}</li>' for i, (_, t, _) in enumerate(items)) + "</ul>"
        nota_html = f'<div class="nota" id="n{k}">{nota[0]}</div>' if nota else ""
        self._escena(k, self._encabezado(kicker, titulo) + lateral + nota_html + f'<div class="marco">{imgs}</div>')
        a = self.inicio[k - 1]
        entera = self.enfoque(960, 540, FW / 1920)
        for j, (i, (_, focos)) in enumerate(zip(ids, imagenes)):
            self.anim.append(f'tl.set("#{i}", {{x: {entera["x"]}, y: {entera["y"]}, scale: {entera["scale"]}, opacity: {1 if j == 0 else 0}}}, {a});')
            for n, (p, cx, cy, s) in enumerate(focos):
                t = self.en(k, p) if p else round(a + 0.6, 3)
                f = self.enfoque(cx, cy, s)
                if n == 0 and j > 0:
                    self.anim.append(f'tl.set("#{i}", {{x: {f["x"]}, y: {f["y"]}, scale: {s}}}, {round(t - 0.45, 3)});')
                    self.anim.append(f'tl.to("#{i}", {{opacity: 1, duration: 0.45}}, {round(t - 0.45, 3)});')
                else:
                    self.anim.append(f'tl.to("#{i}", {{x: {f["x"]}, y: {f["y"]}, scale: {s}, duration: 1.1, ease: "power2.inOut"}}, {t});')
        if lado:
            tipo, items = lado
            if tipo == "pasos":
                for i, (_, _, p) in enumerate(items):
                    if p is None:      # este paso no se activa en esta escena
                        continue
                    t = self.en(k, p) if p else round(a + 0.3, 3)
                    for j in range(len(items)):
                        self.anim.append(f'tl.to("#s{k}-{j}", {{opacity: {1 if j == i else 0.38}, duration: 0.3}}, {t});')
            else:
                for i, (_, _, p) in enumerate(items):
                    self.aparece(f"#s{k}-{i}", self.en(k, p), y=0)
        if nota:
            self.aparece(f"#n{k}", self.en(k, nota[1]))

    def fragmentos(self, k: int, kicker: str, titulo: str, cortes: list[tuple[str, str | float, float]],
                   rotulos: list[tuple[str, str | float]]) -> None:
        """cortes: (fragmento, palabra donde empieza, desde qué segundo del archivo)."""
        rt = "".join(f'<li id="g{k}-{i}" style="--c:var(--c{i % 5})">{t}</li>' for i, (t, _) in enumerate(rotulos))
        self._escena(k, self._encabezado(kicker, titulo) + f'<ul class="rotulos">{rt}</ul><div class="marco"></div>')
        for i, (_, p) in enumerate(rotulos):
            self.aparece(f"#g{k}-{i}", self.en(k, p), y=0)
        inicios = [self.inicio[k - 1] if n == 0 else self.en(k, p) for n, (_, p, _) in enumerate(cortes)]
        for n, (archivo, _, desde) in enumerate(cortes):
            a = inicios[n]
            b = inicios[n + 1] if n + 1 < len(cortes) else self.fin(k)
            self.medios.append(f'<video id="v{k}-{n}" class="clip fragmento" src="{self._usar("clips/" + archivo + ".mp4")}" '
                               f'muted playsinline data-start="{a}" data-duration="{round(b - a, 3)}" '
                               f'data-media-start="{desde}" data-track-index="2"></video>')

    def imagen(self, k: int, kicker: str, titulo: str, ruta: str, texto: list[tuple[str, str | float]]) -> None:
        """Una imagen grande a la izquierda y textos a la derecha (la entrega)."""
        tx = "".join(f'<div class="bloque" id="b{k}-{i}">{t}</div>' for i, (t, _) in enumerate(texto))
        self._escena(k, self._encabezado(kicker, titulo) +
                     f'<img class="miniatura" id="im{k}" src="{self._usar(ruta)}" alt=""><div class="entrega">{tx}</div>')
        self.aparece(f"#im{k}", round(self.inicio[k - 1] + 0.4, 3))
        for i, (_, p) in enumerate(texto):
            self.aparece(f"#b{k}-{i}", self.en(k, p))

    def cierre(self, k: int, siguiente: str, doc: str) -> None:
        self._escena(k, f'''
  <div class="cierre">
    <img src="{self._usar("marca/escudo-unal-blanco.png")}" class="escudo-grande" alt="">
    <div class="kicker" id="z{k}-a">{siguiente}</div>
    <div class="repo" id="z{k}-b">{REPO}</div>
    <div class="doc" id="z{k}-c">{doc}</div>
    {self._barra()}
    <p class="lema" id="z{k}-d">Hecho por estudiantes · abierto a quien quiera mejorarlo</p>
  </div>''')
        for i, s in enumerate("abcd"):
            self.aparece(f"#z{k}-{s}", round(self.inicio[k - 1] + 0.3 + 0.5 * i, 3))

    # ---- salida --------------------------------------------------------------
    def escribir(self) -> None:
        self.dir.mkdir(parents=True, exist_ok=True)
        for f in ("hyperframes.json", "package.json"):
            if not (self.dir / f).exists():
                shutil.copy(PLANTILLA / f, self.dir / f)
        (self.dir / "meta.json").write_text(json.dumps({"id": f"paso-{self.paso}", "name": self.nombre}), encoding="utf-8")
        # Los recursos que el video usa, copiados a su carpeta.
        for ruta in sorted(self.usados | {"musica/bossa-antigua.mp3", "marca/IBMPlexSans-Regular.ttf",
                                           "marca/IBMPlexSans-Medium.ttf", "marca/IBMPlexSans-SemiBold.ttf"}):
            origen = COMPARTIDO / ruta
            if not origen.exists():
                origen = PLANTILLA / "assets" / ruta
            destino = self.dir / "assets" / ruta
            destino.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy(origen, destino)
        audios = [f'<audio id="narracion-{k + 1}" src="assets/narracion/escena-{k + 1}.wav" '
                  f'data-start="{round(self.inicio[k] + ENTRADA, 3)}" data-duration="{self.dur[k]}" data-track-index="3"></audio>'
                  for k in range(len(self.dur))]
        audios.append(f'<audio id="musica" src="assets/musica/bossa-antigua.mp3" data-start="0" data-duration="{self.total}" '
                      f'data-volume="0.55" data-fade-in="1.5" data-fade-out="3" loop data-track-index="4"></audio>')
        html = (PLANTILLA_HTML
                .replace("%TOTAL%", str(self.total))
                .replace("%ESCENAS%", "\n".join(self.html))
                .replace("%MEDIOS%", "\n".join(self.medios + audios))
                .replace("%ANIM%", "\n".join(self.anim)))
        (self.dir / "index.html").write_text(html, encoding="utf-8")
        print(f"{self.dir / 'index.html'}: {len(self.dur)} escenas, {self.total} s")


PLANTILLA_HTML = '''<!doctype html>
<html lang="es" data-resolution="landscape">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=1920, height=1080" />
<script src="https://cdn.jsdelivr.net/npm/gsap@3.15.0/dist/gsap.min.js"></script>
<style>
@font-face { font-family: Plex; src: url(assets/marca/IBMPlexSans-Regular.ttf); font-weight: 400; }
@font-face { font-family: Plex; src: url(assets/marca/IBMPlexSans-Medium.ttf); font-weight: 500; }
@font-face { font-family: Plex; src: url(assets/marca/IBMPlexSans-SemiBold.ttf); font-weight: 600; }
:root { --fondo: #10131A; --tinta: #ECE6D8; --apagado: #8A8F9C; --dorado: #F2B84B;
  --c0: #5DA9E9; --c1: #F2B84B; --c2: #A78BFA; --c3: #4FC1A6; --c4: #E8706A; }
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body { width: 1920px; height: 1080px; overflow: hidden; background: var(--fondo); }
#root { position: relative; width: 100%; height: 100%; font-family: Plex, sans-serif; color: var(--tinta); }
#nube { position: absolute; inset: 0; }
.escena { position: absolute; inset: 0; }
.sello { position: absolute; left: 80px; top: 64px; display: flex; gap: 28px; align-items: center; }
.sello img { height: 92px; }
.espaciado { letter-spacing: 0.32em; font-size: 22px; font-weight: 500; color: var(--apagado); }
.apagado { color: var(--apagado); font-size: 28px; margin-top: 6px; }
.kicker { letter-spacing: 0.28em; font-size: 30px; font-weight: 500; color: var(--dorado); }
.portada { position: absolute; left: 112px; top: 320px; }
.portada h1 { font-size: 132px; font-weight: 600; letter-spacing: -0.02em; line-height: 1.05; margin-top: 18px; }
.barra { display: flex; gap: 8px; margin: 34px 0 30px; }
.barra i { width: 112px; height: 10px; display: block; }
.lema { font-size: 40px; color: var(--apagado); max-width: 1500px; }
.encabezado { position: absolute; left: 80px; top: 70px; }
.encabezado h2 { font-size: 64px; font-weight: 600; margin-top: 10px; letter-spacing: -0.01em; }
.reglas { position: absolute; left: 112px; top: 290px; list-style: none; display: grid; gap: 30px; max-width: 1700px; }
.reglas li { font-size: 42px; display: flex; align-items: center; gap: 30px; opacity: 0; }
.cod { font-size: 26px; font-weight: 600; color: var(--fondo); padding: 6px 16px; border-radius: 8px; min-width: 92px; text-align: center; flex: none; }
.punto { width: 18px; height: 18px; border-radius: 50%; flex: none; }
.veredicto { position: absolute; left: 112px; top: 900px; font-size: 38px; color: var(--dorado); opacity: 0; }
.chips { position: absolute; left: 80px; top: 250px; display: flex; gap: 16px; flex-wrap: wrap; }
.chip { font-size: 30px; padding: 10px 24px; border: 2px solid #2b323b; border-radius: 999px; background: #161a22; opacity: 0; }
.terminal { position: absolute; left: 80px; width: 1760px; background: #0a0c10; border: 2px solid #2b323b;
  border-radius: 18px; padding: 34px 44px 44px; font-family: Menlo, monospace; font-size: 38px; line-height: 1.75; }
.puntos { display: flex; gap: 12px; margin-bottom: 18px; }
.puntos i { width: 18px; height: 18px; border-radius: 50%; background: #2b323b; display: block; }
.tl { opacity: 0; white-space: pre; display: flex; }
.pr { color: var(--c3); }
.tx { display: inline-block; overflow: hidden; white-space: pre; vertical-align: bottom; }
.marco { position: absolute; left: 600px; top: 200px; width: 1244px; height: 700px; overflow: hidden;
  border-radius: 16px; border: 2px solid #2b323b; background: #0d0f12; box-shadow: 0 30px 80px rgba(0,0,0,.45); }
.captura { position: absolute; left: 0; top: 0; width: 1920px; height: 1080px; transform-origin: 0 0; }
.pasos, .teclas, .rotulos { position: absolute; left: 80px; top: 250px; width: 470px; list-style: none; display: grid; gap: 26px; }
.pasos li { font-size: 32px; display: flex; gap: 20px; align-items: center; }
.pasos span { width: 50px; height: 50px; border-radius: 50%; background: var(--dorado); color: var(--fondo);
  display: grid; place-items: center; font-weight: 600; font-size: 26px; flex: none; }
.teclas li { font-size: 32px; display: flex; gap: 22px; align-items: center; opacity: 0; }
kbd { font-family: Plex, sans-serif; font-weight: 600; font-size: 28px; min-width: 64px; padding: 8px 16px; text-align: center;
  border: 2px solid var(--dorado); color: var(--dorado); border-radius: 10px; background: #161a22; flex: none; }
.nota { position: absolute; left: 80px; top: 780px; width: 470px; font-size: 28px; color: var(--apagado); opacity: 0; }
.rotulos li { font-size: 30px; padding-left: 22px; border-left: 6px solid var(--c); opacity: 0; line-height: 1.25; }
.fragmento { position: absolute; left: 602px; top: 202px; width: 1240px; height: 696px; object-fit: cover; border-radius: 14px; z-index: 5; }
.miniatura { position: absolute; left: 80px; top: 250px; width: 960px; border-radius: 14px; border: 2px solid #2b323b; opacity: 0; }
.entrega { position: absolute; left: 1100px; top: 260px; width: 740px; display: grid; gap: 26px; }
.bloque { font-size: 32px; line-height: 1.35; padding: 20px 26px; background: #161a22; border: 2px solid #2b323b; border-radius: 14px; opacity: 0; }
.cierre { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; }
.cierre .barra { justify-content: center; }
.escudo-grande { height: 130px; margin-bottom: 36px; }
.repo { font-family: Menlo, monospace; font-size: 50px; margin-top: 26px; }
.doc { font-family: Menlo, monospace; font-size: 34px; color: var(--apagado); margin-top: 14px; }
</style>
</head>
<body>
<div id="root" data-composition-id="main" data-start="0" data-duration="%TOTAL%" data-width="1920" data-height="1080">
<canvas id="nube" width="1920" height="1080"></canvas>
%ESCENAS%
%MEDIOS%
</div>
<script>
(function () {
  const c = document.getElementById("nube").getContext("2d");
  let s = 22;
  const r = () => (s = (s * 1103515245 + 12345) % 2147483648) / 2147483648;
  const g = () => (r() + r() + r() - 1.5) / 1.5;
  const centros = [[0.12, 0.2, 0.16], [0.86, 0.75, 0.18], [0.5, 1.0, 0.12]];
  const colores = ["#5DA9E9", "#F2B84B", "#A78BFA", "#4FC1A6", "#E8706A"];
  for (let i = 0; i < 520; i++) {
    const [cx, cy, sp] = centros[i % 5 < 2 ? 0 : i % 5 < 4 ? 1 : 2];
    const x = (cx + g() * sp) * 1920, y = (cy + g() * sp * 1.6) * 1080;
    c.globalAlpha = 0.12 + r() * 0.3;
    c.fillStyle = colores[Math.floor(r() * 5)];
    c.beginPath(); c.arc(x, y, 1.2 + r() * 2.2, 0, Math.PI * 2); c.fill();
  }
})();
const tl = gsap.timeline({ paused: true });
%ANIM%
window.__timelines = window.__timelines || {};
window.__timelines["main"] = tl;
tl.seek(0);
</script>
</body>
</html>
'''
