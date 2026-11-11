#!/usr/bin/env python3
"""Preparación de una lectura para el estudio: del PDF del libro a sus insumos.

Cada lectura vive en su carpeta con un `lectura.toml` que dice qué libro, qué
páginas y qué citas. Este programa lee el PDF y deja listo lo que el estudio
necesita: las páginas recortadas (`paginas/`, `tira.json`), las figuras con y
sin pie (`figuras/`), el texto de cada cita para la voz sintética
(`audio/tts/`) y `marcas.json`. También escribe `guion.txt`, el pasaje
numerado con lo que hay que hacer en cada punto, para ensayar.

    python preparar/lector.py nueva --libro LIBRO.pdf --paginas 207-218 \\
        --numero 22 --titulo "Belleza cuántica II: exuberancia"
    python preparar/lector.py preparar lecturas/22-belleza-cuantica-ii-exuberancia

Requiere PyMuPDF (pip install -r preparar/requirements.txt). ffprobe, si está,
mide la duración de los clips de voz.
"""

from __future__ import annotations

import argparse
import bisect
import json
import os
import re
import shutil
import subprocess
import sys
import tomllib
import unicodedata
from collections import Counter
from dataclasses import dataclass
from pathlib import Path

KIT = Path(__file__).resolve().parent
ESTUDIO = KIT.parent

try:
    import pymupdf
except ImportError:
    try:  # versiones anteriores a 1.24.3 solo se importan como fitz
        import fitz as pymupdf
    except ImportError:  # se avisa al usarlo, no al importar
        pymupdf = None

PALETA = ["#ffab3d", "#35c66b", "#b07cff", "#4c8dff", "#ff5f56",
          "#2ec4c4", "#f06bc4", "#c8c84a", "#8fb3ff", "#ff9f80"]

# Una referencia en el texto: «la Figura 27», «(véase la Lámina FF)».
REFERENCIA = re.compile(r"\b(Figura|Fig\.|Lámina|Tabla)\s+(\d+|[A-Z]{1,3})\b")
# El comienzo de un pie: «FIGURA 27. El carbono…», «LÁMINA F. Platón…».
PIE = re.compile(r"^\s*(FIGURA|Figura|FIG\.|Fig\.|LÁMINA|Lámina|LAMINA|TABLA|Tabla)\s+"
                 r"(\d+|[A-Z]{1,3})\s*[.:]")
# Fin de frase: el punto (o ?, !, …) seguido de espacio o de fin de texto,
# admitiendo comillas o paréntesis de cierre en medio.
FIN_FRASE = re.compile(r"[.?!…][»”\"')\]]*(?=\s|$)")
CITA_ANGULAR = re.compile(r"«[^»]{12,}?»")

PALABRAS_POR_MINUTO = 150


# ----------------------------------------------------------------------------
# utilidades
# ----------------------------------------------------------------------------

def nfc(s: str) -> str:
    return unicodedata.normalize("NFC", s)


def slug(s: str) -> str:
    s = unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode()
    return re.sub(r"[^a-z0-9]+", "-", s.lower()).strip("-")


def patron_flexible(ancla: str) -> re.Pattern:
    """Regex que encuentra `ancla` sin importar saltos de línea ni espacios."""
    fichas = nfc(ancla).split()
    if not fichas:
        raise ValueError("ancla vacía")
    return re.compile(r"\s+".join(re.escape(f) for f in fichas))


def rango_paginas(texto: str) -> list[int]:
    """«207-218» o «61-72, 446» → lista de páginas del PDF (1 = primera)."""
    paginas: list[int] = []
    for trozo in str(texto).replace(" ", "").split(","):
        if not trozo:
            continue
        if "-" in trozo:
            a, b = (int(x) for x in trozo.split("-", 1))
            paginas.extend(range(a, b + 1))
        else:
            paginas.append(int(trozo))
    return paginas


def duracion_audio(ruta: Path) -> float:
    if not ruta.exists() or not shutil.which("ffprobe"):
        return 0.0
    p = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration",
         "-of", "default=nokey=1:noprint_wrappers=1", str(ruta)],
        capture_output=True, text=True)
    try:
        return round(float(p.stdout.strip()), 2)
    except ValueError:
        return 0.0


def reloj(segundos: float) -> str:
    return f"{int(segundos // 60)}:{int(segundos % 60):02d}"


class ErrorLectura(Exception):
    """Un problema de la lectura que se le explica a quien la prepara."""


def requiere_pymupdf() -> None:
    if pymupdf is None:
        sys.exit("Falta PyMuPDF. Instálalo con:  pip install -r requirements.txt")


# ----------------------------------------------------------------------------
# configuración
# ----------------------------------------------------------------------------

@dataclass
class Config:
    carpeta: Path
    datos: dict

    @property
    def lectura(self) -> dict:
        return self.datos.get("lectura", {})

    @property
    def pantalla(self) -> dict:
        return self.datos.get("pantalla", {})

    @property
    def libro(self) -> Path:
        ruta = Path(os.path.expanduser(self.lectura["libro"]))
        ruta = ruta if ruta.is_absolute() else (self.carpeta / ruta).resolve()
        # Dentro del contenedor de preparación el libro no está donde lo dice
        # el toml del anfitrión: está en la carpeta montada en $LIBROS.
        if not ruta.exists() and os.environ.get("LIBROS"):
            otra = Path(os.environ["LIBROS"]) / ruta.name
            if otra.exists():
                return otra
        return ruta

    @property
    def paginas(self) -> list[int]:
        return rango_paginas(self.lectura["paginas"])

    @property
    def dpi(self) -> int:
        return int(self.pantalla.get("dpi", 200))

    @property
    def titulo_video(self) -> str:
        L = self.lectura
        return f"{L.get('curso', 'ME')}0{L.get('grupo', 1)} Lectura {int(L['numero']):02d} {L['titulo']}"


def cargar_config(carpeta: Path) -> Config:
    ruta = carpeta / "lectura.toml"
    if not ruta.exists():
        raise ErrorLectura(f"No encuentro {ruta}. Crea la lectura con `./estudio nueva …`.")
    try:
        datos = tomllib.loads(ruta.read_text(encoding="utf-8"))
    except tomllib.TOMLDecodeError as e:
        raise ErrorLectura(f"{ruta}: {e}")
    cfg = Config(carpeta.resolve(), datos)
    for clave in ("libro", "paginas", "numero", "titulo"):
        if clave not in cfg.lectura:
            raise ErrorLectura(f"{ruta}: falta `{clave}` en [lectura]")
    if not cfg.libro.exists():
        raise ErrorLectura(f"No encuentro el libro: {cfg.libro}\n"
                           f"Corrige `libro` en {ruta} (ruta absoluta, con ~, o relativa a la carpeta).")
    return cfg


# ----------------------------------------------------------------------------
# lectura del PDF
# ----------------------------------------------------------------------------

@dataclass
class Linea:
    pag: int            # página del PDF (1 = primera)
    x0: float
    y0: float
    x1: float
    y1: float
    texto: str
    tam: float          # tamaño de letra mayor de la línea
    bloque: int         # bloque de PyMuPDF; casi siempre un párrafo
    tipo: str = "texto"  # texto | titulo | pie
    parrafo: int = 0
    y: float = 0.0      # posición en la tira, en píxeles
    h: float = 0.0
    html: str = ""      # el mismo texto con cursivas, negritas y subíndices


def lineas_de_pagina(pagina) -> list[Linea]:
    crudas: list[Linea] = []
    for b in pagina.get_text("dict")["blocks"]:
        if b["type"] != 0:
            continue
        for ln in b["lines"]:
            spans = [s for s in ln["spans"] if s["text"].strip()]
            if not spans:
                continue
            texto = nfc("".join(s["text"] for s in ln["spans"]).strip())
            crudas.append(Linea(pagina.number + 1, *ln["bbox"], texto,
                                max(s["size"] for s in spans), b["number"],
                                html=html_de_spans(ln["spans"])))

    # Viñetas y letras sueltas salen como líneas aparte a la misma altura que
    # su texto. Se juntan para que el guion muestre una sola línea.
    crudas.sort(key=lambda l: ((l.y0 + l.y1) / 2, l.x0))
    unidas: list[Linea] = []
    for l in crudas:
        previa = unidas[-1] if unidas else None
        if previa and abs((l.y0 + l.y1) / 2 - (previa.y0 + previa.y1) / 2) < 0.4 * (l.y1 - l.y0):
            if len(previa.texto) <= 2:       # la viñeta cede el bloque al texto
                previa.bloque = l.bloque
            previa.texto = f"{previa.texto} {l.texto}"
            previa.html = f"{previa.html} {l.html}"
            previa.x0, previa.x1 = min(previa.x0, l.x0), max(previa.x1, l.x1)
            previa.y0, previa.y1 = min(previa.y0, l.y0), max(previa.y1, l.y1)
            previa.tam = max(previa.tam, l.tam)
        else:
            unidas.append(l)
    return [l for l in unidas if not re.fullmatch(r"\d{1,4}", l.texto)]  # folios


def html_de_spans(spans: list[dict]) -> str:
    """La línea como HTML: cursiva y negrita como en el libro, y los números
    pequeños fuera de la línea base (el 60 de C60) como sub- o superíndice.
    Su texto, sin etiquetas, es el mismo de Linea.texto."""
    import html as h
    normales = [s for s in spans if s["text"].strip()]
    if not normales:
        return ""
    cuerpo = max(s["size"] for s in normales)
    base = max(normales, key=lambda s: (s["size"], len(s["text"])))["origin"][1]
    partes = []
    for s in spans:
        t = h.escape(nfc(s["text"]), quote=False)
        if not t:
            continue
        if s["text"].strip() and s["size"] < 0.8 * cuerpo:
            t = f"<sub>{t}</sub>" if s["origin"][1] > base + 0.5 else f"<sup>{t}</sup>"
        if s["flags"] & 2 and s["text"].strip():
            t = f"<i>{t}</i>"
        if s["flags"] & 16 and s["text"].strip():
            t = f"<b>{t}</b>"
        partes.append(t)
    return "".join(partes).strip()


def imagenes_de_pagina(pagina) -> list:
    rects = [pymupdf.Rect(i["bbox"]) for i in pagina.get_image_info()]
    return [r for r in rects if r.width > 30 and r.height > 30]


def clasificar(lineas: list[Linea]) -> float:
    """Marca títulos y pies de figura. Devuelve el tamaño del cuerpo de texto."""
    pesos = Counter()
    for l in lineas:
        pesos[round(l.tam, 1)] += len(l.texto)
    cuerpo = pesos.most_common(1)[0][0] if pesos else 12.0

    bloques_pie = set()
    vistos = set()
    for l in lineas:
        clave = (l.pag, l.bloque)
        if clave in vistos:
            continue
        vistos.add(clave)
        if PIE.match(l.texto):
            bloques_pie.add(clave)

    for l in lineas:
        if (l.pag, l.bloque) in bloques_pie:
            l.tipo = "pie"
        elif l.tam >= cuerpo * 1.15:
            l.tipo = "titulo"
    return cuerpo


def numerar_parrafos(lineas: list[Linea], x_texto: float, x_texto1: float) -> None:
    """Un párrafo nuevo empieza con cada bloque, salvo cuando el bloque solo
    continúa en la página siguiente el párrafo que venía.

    Continúa si la página anterior terminó con una línea llena (el párrafo no
    acabó ahí) y la primera línea de la nueva no tiene sangría: ni respecto al
    margen del libro ni respecto a la línea anterior. Lo segundo es por las
    listas, cuyo texto va entrado: el punto de una lista que sigue en la otra
    página arranca en el margen de la lista, no en el del libro, y antes se
    tomaba por un párrafo nuevo, que partía la frase en dos tomas."""
    n = 0
    for i, l in enumerate(lineas):
        previa = lineas[i - 1] if i else None
        if previa is None:
            n += 1
        elif (l.pag, l.bloque) != (previa.pag, previa.bloque):
            continua = (l.pag != previa.pag and l.tipo == previa.tipo == "texto"
                        and previa.x1 >= x_texto1 - 25
                        and (l.x0 <= x_texto + 8 or l.x0 <= previa.x0 + 4))
            if not continua:
                n += 1
        l.parrafo = n


class Texto:
    """Las líneas unidas con '\\n', para buscar anclas que cruzan renglones."""

    def __init__(self, lineas: list[Linea]):
        self.inicios: list[int] = []
        partes, pos = [], 0
        for l in lineas:
            self.inicios.append(pos)
            partes.append(l.texto)
            pos += len(l.texto) + 1
        self.cadena = "\n".join(partes)

    def linea_de(self, offset: int) -> int:
        return max(0, bisect.bisect_right(self.inicios, offset) - 1)

    def buscar(self, ancla: str, desde: int = 0) -> re.Match | None:
        return patron_flexible(ancla).search(self.cadena, desde)


# ----------------------------------------------------------------------------
# preparar
# ----------------------------------------------------------------------------

def localizar_figura(doc, tipo: str, ident: str, preferidas: list[int], forzada: int | None):
    """Busca el pie de la figura en el libro y la imagen que le corresponde.

    Devuelve (página, rect_imagen, rect_pie, texto_pie) o None.
    """
    palabra = {"figura": r"(?:figura|fig\.?)", "lamina": r"l[áa]mina", "tabla": r"tabla"}[tipo]
    patron = re.compile(rf"^\s*{palabra}\s+{re.escape(ident)}\s*[.:]", re.I)

    if forzada:
        orden = [forzada]
    else:
        resto = [p for p in range(1, len(doc) + 1) if p not in preferidas]
        orden = list(preferidas) + resto

    for numero in orden:
        pagina = doc[numero - 1]
        if not re.search(rf"\b{re.escape(ident)}\b", pagina.get_text("text")):
            continue
        for b in pagina.get_text("dict")["blocks"]:
            if b["type"] != 0:
                continue
            texto = nfc(" ".join("".join(s["text"] for s in ln["spans"]) for ln in b["lines"]))
            if not patron.match(texto):
                continue
            pie = pymupdf.Rect(b["bbox"])
            imagenes = imagenes_de_pagina(pagina)
            arriba = [r for r in imagenes if r.y1 <= pie.y0 + 4]
            abajo = [r for r in imagenes if r.y0 >= pie.y1 - 4]
            if arriba:
                img = max(arriba, key=lambda r: r.y1)
            elif abajo:
                img = min(abajo, key=lambda r: r.y0)
            elif imagenes:
                img = max(imagenes, key=lambda r: r.width * r.height)
            else:
                # Figura dibujada con vectores: la zona entre el texto de arriba y el pie.
                techo = 36.0
                for otro in pagina.get_text("blocks"):
                    if otro[3] <= pie.y0 - 2:
                        techo = max(techo, otro[3])
                img = pymupdf.Rect(pie.x0, techo + 4, pie.x1, pie.y0 - 4)
            return numero, img, pie, texto.strip()
    return None


@dataclass
class Analisis:
    """Lo que se sabe del pasaje sin escribir nada todavía.

    Todo se cuenta en índices de `todas`, las líneas de las páginas pedidas
    completas; `lineas` es el tramo que se lee (de `desde` a `hasta`). La
    consola de preparación muestra `todas` para que se elija ese tramo.
    """
    todas: list[Linea]
    i_desde: int
    i_hasta: int
    cuerpo: float
    x_texto0: float
    x_texto1: float
    imagenes: dict
    figuras: list[dict]            # con rects de PyMuPDF en "_img" y "_pie"
    eventos: list[dict]            # {"figura", "modo", "i"}
    persistencias: list[dict]      # {"figura", "desde", "hasta"} en índices de todas
    citas: list[dict]              # resueltas, con "i", "iFin", "texto" o "error"
    candidatas: list[dict]
    avisos: list[str]

    @property
    def lineas(self) -> list[Linea]:
        return self.todas[self.i_desde:self.i_hasta + 1]


def analizar(cfg: Config, doc) -> Analisis:
    paginas = cfg.paginas
    if not paginas or max(paginas) > len(doc) or min(paginas) < 1:
        raise ErrorLectura(f"«{cfg.lectura['paginas']}» no cabe en un PDF de {len(doc)} páginas")
    avisos: list[str] = []

    # --- líneas de las páginas pedidas ---------------------------------------
    todas: list[Linea] = []
    imagenes: dict[int, list] = {}
    for p in paginas:
        pagina = doc[p - 1]
        todas.extend(lineas_de_pagina(pagina))
        imagenes[p] = imagenes_de_pagina(pagina)
    if not todas:
        raise ErrorLectura("Esas páginas no tienen texto extraíble (¿un PDF escaneado sin OCR?).")
    cuerpo = clasificar(todas)

    texto_total = Texto(todas)
    i_desde, i_hasta = 0, len(todas) - 1
    if cfg.lectura.get("desde"):
        m = texto_total.buscar(cfg.lectura["desde"])
        if m:
            i_desde = texto_total.linea_de(m.start())
        else:
            avisos.append(f"No encuentro el comienzo «{cfg.lectura['desde']}»; empiezo en la primera línea.")
    if cfg.lectura.get("hasta"):
        m = texto_total.buscar(cfg.lectura["hasta"], texto_total.inicios[i_desde])
        if m:
            i_hasta = texto_total.linea_de(m.end() - 1)
        else:
            avisos.append(f"No encuentro el final «{cfg.lectura['hasta']}»; termino en la última línea.")
    lineas = todas[i_desde:i_hasta + 1]

    cuerpo_texto = [l for l in lineas if l.tipo == "texto"]
    x_texto0 = min(l.x0 for l in cuerpo_texto) if cuerpo_texto else 72.0
    x_texto1 = max(l.x1 for l in cuerpo_texto) if cuerpo_texto else 540.0
    numerar_parrafos(todas, x_texto0, x_texto1)

    # Texto continuo de lo que se lee, para menciones, frases y citas.
    leido = Texto(lineas)

    # --- figuras y láminas ---------------------------------------------------
    ajustes = {nfc(f["nombre"]): f for f in cfg.datos.get("figura", []) if f.get("nombre")}
    opciones = cfg.datos.get("figuras", {})
    detectar = opciones.get("detectar", True)
    persistente_global = opciones.get("persistente", True)

    menciones: dict[str, list[tuple[int, int]]] = {}   # nombre → [(línea, fin de frase)]
    for m in REFERENCIA.finditer(leido.cadena):
        i = leido.linea_de(m.start())
        if lineas[i].tipo != "texto":
            continue
        palabra, ident = m.group(1), m.group(2)
        nombre = {"Fig.": "Figura"}.get(palabra, palabra) + " " + ident
        fin = FIN_FRASE.search(leido.cadena, m.end())
        j = leido.linea_de(fin.start()) if fin else i
        menciones.setdefault(nombre, [])
        if not menciones[nombre] or menciones[nombre][-1][1] != j:   # una por frase
            menciones[nombre].append((i, j))

    nombres = list(menciones) if detectar else []
    nombres += [n for n in ajustes if n not in nombres]

    figuras, eventos, persistencias = [], [], []
    for nombre in nombres:
        ajuste = ajustes.get(nombre, {})
        palabra, _, ident = nombre.partition(" ")
        tipo = slug(palabra)
        lista = menciones.get(nombre, [])
        ficha = {
            "id": slug(nombre), "nombre": nombre, "pagina": None, "encontrada": False,
            "pie": "", "omitida": bool(ajuste.get("omitir")),
            "persistente": bool(ajuste.get("persistente", persistente_global)),
            "paginaForzada": ajuste.get("pagina"),
            "menciones": [i_desde + i for i, _ in lista],
            "enLaTira": False,
        }
        figuras.append(ficha)
        if tipo not in ("figura", "lamina", "tabla") or not ident:
            avisos.append(f"{nombre}: no sé buscar «{palabra}»; usa Figura, Lámina o Tabla.")
            continue
        hallada = localizar_figura(doc, tipo, ident, paginas, ajuste.get("pagina"))
        if not hallada:
            avisos.append(f"{nombre}: no encontré su pie en el libro. Indica la página donde está.")
            continue
        pag, r_img, r_pie, texto_pie = hallada
        ficha.update(pagina=pag, encontrada=True, pie=texto_pie, enLaTira=pag in paginas,
                     _img=r_img, _pie=r_pie,
                     img=[round(v, 1) for v in r_img], imgPie=[round(v, 1) for v in (r_img | r_pie)])
        if ficha["omitida"]:
            continue

        for k_m, (_, j) in enumerate(lista):
            eventos.append({"figura": ficha["id"], "modo": "pie" if k_m == 0 else "breve",
                            "i": i_desde + j})

        # 2.f: la figura queda fija a la derecha mientras se leen los párrafos
        # que hablan de ella.
        if ficha["persistente"] and lista:
            parrafos = sorted({lineas[i].parrafo for i, _ in lista})
            tramos: list[list[int]] = []
            for par in parrafos:
                if tramos and par <= tramos[-1][1] + 1:
                    tramos[-1][1] = par
                else:
                    tramos.append([par, par])
            for a, b in tramos:
                idx = [i_desde + i for i, l in enumerate(lineas) if a <= l.parrafo <= b]
                persistencias.append({"figura": ficha["id"], "desde": idx[0], "hasta": idx[-1]})
    # Tramos fijados a mano: párrafos que hablan de una figura sin nombrarla
    # («A la izquierda tenemos…»). Se anclan en texto, como `desde` y `hasta`.
    por_nombre = {f["nombre"]: f for f in figuras}
    texto_todas = Texto(todas)
    for k_f, fija in enumerate(cfg.datos.get("fija", []), start=1):
        ficha = por_nombre.get(nfc(fija.get("figura", "")))
        if not ficha or not ficha["encontrada"] or ficha["omitida"]:
            avisos.append(f"Tramo fijo {k_f}: no hay una figura «{fija.get('figura', '')}» que mostrar.")
            continue
        m = texto_todas.buscar(fija["desde"]) if fija.get("desde") else None
        if not m:
            avisos.append(f"Tramo fijo de {ficha['nombre']}: no encuentro «{fija.get('desde', '')}».")
            continue
        a_i = texto_todas.linea_de(m.start())
        b_i = a_i
        if fija.get("hasta"):
            m2 = texto_todas.buscar(fija["hasta"], m.start())
            if m2:
                b_i = texto_todas.linea_de(m2.end() - 1)
        a_i, b_i = max(a_i, i_desde), min(b_i, i_hasta)
        if a_i <= b_i:
            persistencias.append({"figura": ficha["id"], "desde": a_i, "hasta": b_i, "manual": True})

    eventos.sort(key=lambda e: e["i"])
    persistencias.sort(key=lambda p: p["desde"])

    # --- citas ---------------------------------------------------------------
    citas, cursor = [], 0
    for numero, c in enumerate(cfg.datos.get("cita", []), start=1):
        personaje = (c.get("personaje") or "").strip()
        ficha = {"k": numero, "personaje": personaje, "config": c}
        citas.append(ficha)
        ancla_ini = c.get("texto") or c.get("desde")
        if not personaje:
            ficha["error"] = "falta el personaje"
            continue
        if not ancla_ini:
            ficha["error"] = "falta el texto de la cita"
            continue
        m = leido.buscar(ancla_ini, cursor) or leido.buscar(ancla_ini)
        if not m:
            ficha["error"] = f"no encuentro «{ancla_ini[:60]}» en el pasaje"
            continue
        ini, fin = m.start(), m.end()
        if c.get("hasta") and not c.get("texto"):
            m2 = leido.buscar(c["hasta"], ini)
            if not m2:
                ficha["error"] = f"no encuentro el final «{c['hasta'][:60]}»"
                continue
            fin = m2.end()
        cursor = fin
        dicho = c.get("decir") or re.sub(r"\s+", " ", leido.cadena[ini:fin]).strip()
        dicho = dicho.strip("«»“”\"' ")
        if c.get("quitar"):
            dicho = re.sub(c["quitar"], "", dicho).strip()
        n_ini, n_fin = leido.linea_de(ini), leido.linea_de(fin - 1)
        ficha.update(texto=dicho, i=i_desde + n_ini, iFin=i_desde + n_fin,
                     enLinea=bool(c.get("en_linea", n_fin == n_ini)))
    for f in citas:
        if f.get("error"):
            avisos.append(f"Cita {f['k']} ({f['personaje'] or 'sin personaje'}): {f['error']}.")

    # --- frases entre comillas que podrían ser citas -------------------------
    ocupadas = {n for f in citas if "i" in f for n in range(f["i"], f["iFin"] + 1)}
    candidatas, vistas = [], set()
    for m in CITA_ANGULAR.finditer(leido.cadena):
        i = i_desde + leido.linea_de(m.start())
        if i in ocupadas or todas[i].tipo != "texto" or len(m.group().split()) < 4 or i in vistas:
            continue
        vistas.add(i)
        candidatas.append({"i": i, "texto": re.sub(r"\s+", " ", m.group())})

    return Analisis(todas, i_desde, i_hasta, cuerpo, x_texto0, x_texto1, imagenes,
                    figuras, eventos, persistencias, citas, candidatas, avisos)


def duracion_estimada(cfg: Config, a: Analisis, durs: dict[str, float] | None = None) -> dict:
    lineas = a.lineas
    palabras = sum(len(l.texto.split()) for l in lineas if l.tipo != "pie")
    con_pie = {e["figura"] for e in a.eventos if e["modo"] == "pie"}
    palabras += sum(len(f["pie"].split()) for f in a.figuras if f["id"] in con_pie)
    validas = [c for c in a.citas if "texto" in c]
    palabras_citas = sum(len(c["texto"].split()) for c in validas)
    durs = durs or {}
    seg_citas = sum(durs.get(c["k"]) or len(c["texto"].split()) / PALABRAS_POR_MINUTO * 60 for c in validas)
    seg_figuras = len(a.eventos) * float(cfg.pantalla.get("segundos_figura", 3))
    return {str(ppm): round((palabras - palabras_citas) / ppm * 60 + seg_citas + seg_figuras)
            for ppm in (140, 150, 160)}


def analisis_json(cfg: Config, a: Analisis) -> dict:
    """La vista del análisis que usa la consola de preparación."""
    def publica(f: dict) -> dict:
        return {k: v for k, v in f.items() if not k.startswith("_")}

    return {
        "titulo": cfg.titulo_video,
        "libro": cfg.libro.name,
        "desde": a.i_desde, "hasta": a.i_hasta,
        "todas": [{"i": i, "pag": l.pag, "t": l.texto, "tipo": l.tipo, "p": l.parrafo,
                   "x0": round(l.x0, 1), "y0": round(l.y0, 1), "x1": round(l.x1, 1), "y1": round(l.y1, 1)}
                  for i, l in enumerate(a.todas)],
        "secciones": [{"i": i, "t": l.texto, "pag": l.pag}
                      for i, l in enumerate(a.todas) if l.tipo == "titulo"],
        "figuras": [publica(f) for f in a.figuras],
        "eventos": a.eventos,
        "persistencias": a.persistencias,
        "citas": [{k: v for k, v in c.items() if k != "config"} for c in a.citas],
        "candidatas": a.candidatas,
        "duracion": duracion_estimada(cfg, a),
        "avisos": a.avisos,
    }


def escribir(cfg: Config, doc, a: Analisis) -> dict:
    """Páginas, figuras, textos de voz, marcas.json, tira.json y guion.txt."""
    k = cfg.dpi / 72.0
    hueco = float(cfg.pantalla.get("hueco", 22))           # puntos sobre y bajo cada página
    ancho_figura = float(cfg.pantalla.get("ancho_figura", 0.38))
    segundos_figura = float(cfg.pantalla.get("segundos_figura", 3))
    lineas = a.lineas
    base = a.i_desde

    # --- tira de páginas recortadas ------------------------------------------
    dir_paginas = cfg.carpeta / "paginas"
    if dir_paginas.exists():
        shutil.rmtree(dir_paginas)
    dir_paginas.mkdir(parents=True)

    tira, desplazamiento, ancho_px = [], 0, 0
    origen: dict[int, tuple[int, float]] = {}   # página → (dónde empieza en la tira, recorte de arriba)
    for p in cfg.paginas:
        propias = [l for l in lineas if l.pag == p]
        if not propias:
            continue
        pagina = doc[p - 1]
        arriba = min(l.y0 for l in propias)
        abajo = max(l.y1 for l in propias)
        recortada_arriba = p == lineas[0].pag and a.i_desde > 0
        recortada_abajo = p == lineas[-1].pag and a.i_hasta < len(a.todas) - 1
        for r in a.imagenes[p]:
            if (recortada_arriba and r.y1 < arriba) or (recortada_abajo and r.y0 > abajo):
                continue
            arriba, abajo = min(arriba, r.y0), max(abajo, r.y1)
        arriba = max(0.0, arriba - hueco)
        abajo = min(pagina.rect.height, abajo + hueco)

        pix = pagina.get_pixmap(dpi=cfg.dpi, clip=pymupdf.Rect(0, arriba, pagina.rect.width, abajo))
        nombre = f"t-{p:03d}.png"
        pix.save(dir_paginas / nombre)
        ancho_px = max(ancho_px, pix.width)
        for l in propias:
            l.y = round(desplazamiento + (l.y0 - arriba) * k, 1)
            l.h = round((l.y1 - l.y0) * k, 1)
        origen[p] = (desplazamiento, arriba)
        tira.append({"pagina": p, "archivo": nombre, "alto": pix.height, "ancho": pix.width})
        desplazamiento += pix.height
    alto_tira = desplazamiento

    def fr(y: float) -> float:
        return round(y / alto_tira, 5)

    def n(i: int) -> int:          # índice de `todas` → número de línea del pasaje
        return i - base + 1

    def linea(i: int) -> Linea:
        return a.todas[i]

    # --- figuras -------------------------------------------------------------
    dir_figuras = cfg.carpeta / "figuras"
    if dir_figuras.exists():
        shutil.rmtree(dir_figuras)
    dir_figuras.mkdir()
    figuras = []
    for f in a.figuras:
        if not f["encontrada"] or f["omitida"]:
            continue
        pagina = doc[f["pagina"] - 1]
        margen = 6
        con_pie = (f["_img"] | f["_pie"]) + (-margen, -margen, margen, margen)
        sin_pie = f["_img"] + (-2, -2, 2, 2)
        dpi = max(cfg.dpi, 200)
        pagina.get_pixmap(dpi=dpi, clip=con_pie & pagina.rect).save(dir_figuras / f"{f['id']}-pie.png")
        pagina.get_pixmap(dpi=dpi, clip=sin_pie & pagina.rect).save(dir_figuras / f"{f['id']}.png")
        ficha = {
            "id": f["id"], "nombre": f["nombre"], "pagina": f["pagina"],
            "img": f"figuras/{f['id']}.png", "imgPie": f"figuras/{f['id']}-pie.png",
            "pie": f["pie"], "enLaTira": f["enLaTira"], "menciones": [n(i) for i in f["menciones"]],
        }
        # Dónde está impresa en la tira, con su pie: mientras la figura está
        # fija a la derecha, el render la tapa en la página para no mostrarla
        # dos veces.
        if f["pagina"] in origen:
            y_tira, arriba = origen[f["pagina"]]
            r = con_pie & pagina.rect
            ficha["caja"] = [round(r.x0 * k), round(y_tira + (r.y0 - arriba) * k),
                             round(r.x1 * k), round(y_tira + (r.y1 - arriba) * k)]
        figuras.append(ficha)
    eventos = [{"figura": e["figura"], "modo": e["modo"], "n": n(e["i"]),
                "y": linea(e["i"]).y + linea(e["i"]).h,
                "f": fr(linea(e["i"]).y + linea(e["i"]).h)} for e in a.eventos]
    persistencias = []
    for p_ in a.persistencias:
        ya, yb = linea(p_["desde"]).y, linea(p_["hasta"]).y + linea(p_["hasta"]).h
        persistencias.append({"figura": p_["figura"], "nDesde": n(p_["desde"]), "nHasta": n(p_["hasta"]),
                              "desde": ya, "hasta": yb, "fDesde": fr(ya), "fHasta": fr(yb)})

    # --- citas para voz sintética (2.i) --------------------------------------
    voces_cfg = cfg.datos.get("voces", {})
    opciones = cfg.datos.get("citas", {})
    pausa_turno = float(opciones.get("pausa_turno", 0.35))
    pausa_larga = float(opciones.get("pausa_larga", 1.5))
    dir_tts = cfg.carpeta / "audio" / "tts"
    dir_tts.mkdir(parents=True, exist_ok=True)
    for viejo in dir_tts.glob("*.txt"):
        viejo.unlink()

    colores: dict[str, str] = {}
    clips = []
    for c in a.citas:
        if "texto" not in c:
            continue
        personaje = c["personaje"]
        if personaje not in colores:
            colores[personaje] = voces_cfg.get(personaje, {}).get("color") or PALETA[len(colores) % len(PALETA)]
        clip = f"{c['k']:02d}-{slug(personaje)}"
        (dir_tts / f"{clip}.txt").write_text(c["texto"] + "\n", encoding="utf-8")
        li, lf = linea(c["i"]), linea(c["iFin"])
        y_fin = None if c["enLinea"] else lf.y + lf.h
        clips.append({
            "clip": clip, "personaje": personaje, "color": colores[personaje],
            "linea": n(c["i"]), "lineaFin": n(c["iFin"]),
            # Posiciones como fracción de la tira, que es lo que usa el estudio.
            # fFin es nulo para una cita en medio de una frase.
            "f": fr(li.y), "fFin": None if y_fin is None else fr(y_fin),
            "dur": duracion_audio(dir_tts / f"{clip}.mp3"),
            "texto": c["texto"],
        })

    # Citas pegadas forman un diálogo: se encadenan con una sola tecla.
    grupos: list[dict] = []
    for i, c in enumerate(clips):
        previo = clips[i - 1] if i else None
        salto = c["linea"] - previo["lineaFin"] if previo else 99
        if previo and salto <= 2:
            grupos[-1]["clips"].append(c["clip"])
            grupos[-1]["pausas"].append(pausa_turno if salto <= 1 else pausa_larga)
        else:
            grupos.append({"clips": [c["clip"]], "pausas": []})

    # --- marcas.json y tira.json ---------------------------------------------
    # marcas.json lleva lo que lee el estudio (paginas, altoTira, clips,
    # grupos, secciones, lamina, lineas con su f) y, además, la geometría y las
    # figuras.
    secciones = [{"titulo": l.texto, "linea": i + 1, "f": fr(l.y)}
                 for i, l in enumerate(lineas) if l.tipo == "titulo"]
    # El estudio parte el pasaje en párrafos por la sangría del texto, como la
    # dejaba pdftotext: un renglón que empieza con 3 a 6 espacios abre párrafo.
    # Aquí se sabe el párrafo de verdad, así que se le pone esa sangría a su
    # primera línea y a ninguna otra. El pie de una figura impresa en la página
    # no es un párrafo que se grabe: se lee cuando la figura se muestra sola
    # (2.d), así que queda pegado al párrafo anterior.
    salida_lineas = []
    for i, l in enumerate(lineas):
        abre = i == 0 or (l.parrafo != lineas[i - 1].parrafo and l.tipo != "pie")
        salida_lineas.append({"n": i + 1, "f": fr(l.y), "t": ("     " if abre else "") + l.texto,
                              "tipo": l.tipo, "p": l.parrafo, "y": l.y, "h": l.h})
    ultima = lineas[-1]
    fin_pasaje = min(alto_tira, int(ultima.y + ultima.h + hueco * k / 2))

    lamina = next((e for e in eventos if e["modo"] == "pie" and e["figura"].startswith("lamina")), None)
    marcas = {
        "paginas": len(tira),
        "altoTira": alto_tira,
        "finPasaje": fin_pasaje,
        "clips": clips,
        "grupos": grupos,
        "secciones": secciones,
        "lamina": {"linea": lamina["n"], "f": lamina["f"], "figura": lamina["figura"]} if lamina else None,
        "lineas": salida_lineas,
        "lectura": {
            "titulo": cfg.lectura["titulo"], "numero": int(cfg.lectura["numero"]),
            "video": cfg.titulo_video, "paginas": str(cfg.lectura["paginas"]),
            "libro": cfg.libro.name,
        },
        "pantalla": {"anchoFigura": ancho_figura, "segundosFigura": segundos_figura},
        "musica": musica_de(cfg),
        "texto": {"x0": round(a.x_texto0 * k, 1), "x1": round(a.x_texto1 * k, 1)},
        "figuras": figuras,
        "eventos": eventos,
        "persistencias": persistencias,
    }
    (cfg.carpeta / "marcas.json").write_text(json.dumps(marcas, ensure_ascii=False, indent=1), encoding="utf-8")
    (cfg.carpeta / "tira.json").write_text(json.dumps({
        "gap": int(round(2 * hueco * k)), "ancho": ancho_px, "paginas": tira,
    }, ensure_ascii=False, indent=1), encoding="utf-8")
    escribir_guion(cfg, lineas, clips, figuras, eventos, persistencias)
    escribir_guia_voces(cfg, clips, voces_cfg)

    # El pasaje compuesto de nuevo para el video, en la letra del libro (2.c, 2.f).
    sys.path.insert(0, str(KIT))
    from componer import componer
    abre = [i == 0 or (l.parrafo != lineas[i - 1].parrafo and l.tipo != "pie") for i, l in enumerate(lineas)]
    componer(cfg, doc, a, lineas, abre, ancho_figura)
    return marcas


def musica_de(cfg: Config) -> dict | None:
    """[musica] de lectura.toml (2.j): la pista vive en la carpeta musica/ del
    estudio, compartida por todas las lecturas; aquí solo va su nombre."""
    m = cfg.datos.get("musica", {})
    if not m.get("archivo"):
        return None
    return {"archivo": Path(str(m["archivo"])).name, "nivel": float(m.get("nivel", 14)),
            "atenuar": bool(m.get("atenuar", True))}


def preparar(carpeta: Path, silencioso: bool = False) -> dict:
    requiere_pymupdf()
    try:
        cfg = cargar_config(carpeta)
        doc = pymupdf.open(cfg.libro)
        a = analizar(cfg, doc)
    except ErrorLectura as e:
        sys.exit(str(e))
    marcas = escribir(cfg, doc, a)
    if not silencioso:
        resumen(cfg, a, marcas)
    return marcas


def escribir_guion(cfg, lineas, clips, figuras, eventos, persistencias) -> None:
    """guion.txt: el pasaje numerado con lo que hay que hacer en cada punto."""
    nombre = {f["id"]: f["nombre"] for f in figuras}
    antes: dict[int, list[str]] = {}
    despues: dict[int, list[str]] = {}
    for c in clips:
        antes.setdefault(c["linea"], []).append(f">> CITA {c['clip'][:2]} · {c['personaje']} (voz sintética)")
    for p in persistencias:
        antes.setdefault(p["nDesde"], []).append(f"[[ {nombre[p['figura']]} fija a la derecha desde aquí ]]")
        despues.setdefault(p["nHasta"], []).append(f"[[ fin de {nombre[p['figura']]} fija ]]")
    for e in eventos:
        if e["modo"] == "pie":
            texto = f"<< {nombre[e['figura']]}: mostrarla con su pie, leer el pie, 2-3 s, volver con flecha >>"
        else:
            texto = f"<< {nombre[e['figura']]}: mostrarla sin pie 2-3 s >>"
        despues.setdefault(e["n"], []).append(texto)

    salida = [f"{cfg.titulo_video}", f"{cfg.libro.name}, páginas {cfg.lectura['paginas']} del PDF", ""]
    for i, l in enumerate(lineas, start=1):
        salida.extend(f"       {a}" for a in antes.get(i, []))
        marca = {"titulo": "## ", "pie": "(pie) "}.get(l.tipo, "")
        salida.append(f"{i:4d}   {marca}{l.texto}")
        salida.extend(f"       {d}" for d in despues.get(i, []))
    (cfg.carpeta / "guion.txt").write_text("\n".join(salida) + "\n", encoding="utf-8")


def escribir_guia_voces(cfg, clips, voces_cfg) -> None:
    dir_tts = cfg.carpeta / "audio" / "tts"
    filas = ["# Voces sintéticas", "",
             "Lineamiento 2.i: cada cita textual se lee con voz sintética, la misma voz para",
             "un mismo personaje en toda la lectura. En la consola (Preparar › Citas) se",
             "diseña la voz de cada personaje y se generan todos los clips, con ElevenLabs o",
             "con Qwen3-TTS en el computador; o, desde la terminal:",
             "",
             "    ./estudio voces generar " + (f"lecturas/{cfg.carpeta.name}" if cfg.carpeta.parent.name == "lecturas" else str(cfg.carpeta)),
             "",
             "Un clip hecho en otra parte se guarda aquí como `NN-personaje.mp3` (o se sube",
             "en la consola) y después `./estudio preparar` lee su duración.", ""]
    personajes = list(dict.fromkeys(c["personaje"] for c in clips))
    for p in personajes:
        propios = [c for c in clips if c["personaje"] == p]
        desc = voces_cfg.get(p, {}).get("descripcion", "")
        filas.append(f"## {p} — {len(propios)} cita(s), {sum(len(c['texto']) for c in propios)} caracteres")
        if desc:
            filas += ["", f"Voz: {desc}"]
        filas.append("")
        for c in propios:
            estado = "listo" if (dir_tts / f"{c['clip']}.mp3").exists() else "falta"
            filas.append(f"- `{c['clip']}.mp3` ({estado}): {c['texto']}")
        filas.append("")
    total = sum(len(c["texto"]) for c in clips)
    filas.append(f"Total: {len(clips)} clips, {total} caracteres.")
    (dir_tts / "VOCES.md").write_text("\n".join(filas) + "\n", encoding="utf-8")


def resumen(cfg: Config, a: Analisis, marcas: dict) -> None:
    lineas = a.lineas
    print(f"\n  {cfg.titulo_video}")
    print(f"  {cfg.libro.name}, páginas {cfg.lectura['paginas']} del PDF")
    print(f"  {len(lineas)} líneas, letra del cuerpo {a.cuerpo:g} pt\n")

    titulos = [l for l in lineas if l.tipo == "titulo"]
    if titulos:
        print("  Secciones:")
        for l in titulos:
            print(f"    p. {l.pag}  {l.texto}")
        print()

    if marcas["figuras"]:
        print("  Figuras y láminas:")
        for f in marcas["figuras"]:
            donde = "en las páginas leídas" if f["enLaTira"] else f"fuera del pasaje, p. {f['pagina']}"
            print(f"    {f['nombre']:<11} mencionada en las líneas {f['menciones'] or '—'}  ({donde})")
        print(f"    {len(marcas['eventos'])} momentos para mostrarlas, "
              f"{len(marcas['persistencias'])} tramos con la figura fija\n")

    if marcas["clips"]:
        print("  Citas:")
        for c in marcas["clips"]:
            dur = f"{c['dur']:.1f}s" if c["dur"] else "sin mp3"
            print(f"    {c['clip']:<18} línea {c['linea']:>3}  {dur:>7}  {c['texto'][:50]}")
        print()

    if a.candidatas:
        print("  Posibles citas sin configurar (decide tú si van con voz sintética):")
        for c in a.candidatas:
            print(f"    línea {c['i'] - a.i_desde + 1:>3}  {c['texto'][:70]}")
        print()

    durs = {c["k"]: d["dur"] for c, d in zip([c for c in a.citas if "texto" in c], marcas["clips"])}
    for ppm, seg in duracion_estimada(cfg, a, durs).items():
        aviso = "" if 5 * 60 <= seg <= 25 * 60 else "  ← fuera de 5–25 min (2.m)"
        print(f"  Duración estimada leyendo a {ppm} palabras/min: {reloj(seg)}{aviso}")

    print("\n  Escrito: marcas.json, tira.json, guion.txt, paginas/, figuras/, audio/tts/")
    for aviso in a.avisos:
        print(f"  AVISO: {aviso}")
    print()


# ----------------------------------------------------------------------------
# lectura.toml desde la consola de preparación
# ----------------------------------------------------------------------------

def toml_valor(v) -> str:
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return repr(v)
    s = str(v).replace("\\", "\\\\").replace('"', '\\"').replace("\n", " ")
    return f'"{s}"'


COMENTARIOS = {
    ("lectura", "curso"): "ME (Modelos estocásticos) o LP (Lenguajes de programación)",
    ("lectura", "grupo"): "dígito del grupo en el SIA",
    ("lectura", "paginas"): "páginas del PDF, no las impresas. Ej.: \"61-72\" o \"61-72, 446\"",
    ("lectura", "desde"): "texto con el que empieza la lectura",
    ("lectura", "hasta"): "texto con el que termina",
    ("pantalla", "ancho_figura"): "parte de la pantalla para la figura fija a la derecha (2.f)",
    ("pantalla", "segundos_figura"): "cuánto se deja una figura sin pie (2.e)",
    ("figuras", "persistente"): "2.f: fija la figura a la derecha en los párrafos que hablan de ella",
    ("musica", "archivo"): "2.j: pista en la carpeta musica/ del estudio",
    ("musica", "nivel"): "dB por debajo de la voz (14 = detrás de la voz sin taparla)",
    ("musica", "atenuar"): "bajarla un poco más mientras se lee",
    ("voces", "motor"): "elevenlabs, qwen (en el computador) o manual (audio propio)",
    ("voces", "voz"): "id de la voz: de ElevenLabs, o la carpeta en voces/qwen/",
}


def escribir_toml(carpeta: Path, datos: dict) -> None:
    """lectura.toml a partir del mismo diccionario que produce tomllib."""
    L = datos.get("lectura", {})
    salida = [f"# Lectura {int(L.get('numero', 0)):02d}: {L.get('titulo', '')}",
              "# Escrito por la consola de preparación del estudio. Se puede editar a mano;",
              f"# después:  ./estudio preparar {'lecturas/' + carpeta.name if carpeta.parent.name == 'lecturas' else carpeta}",
              ""]

    def tabla(nombre: str, contenido: dict, cabecera: str | None = None) -> None:
        if not contenido:
            return
        salida.append(f"[{cabecera or nombre}]")
        for clave, valor in contenido.items():
            if valor is None or valor == "" or isinstance(valor, (dict, list)):
                continue
            nota = COMENTARIOS.get((nombre, clave))
            linea = f"{clave} = {toml_valor(valor)}"
            salida.append(f"{linea:<34} # {nota}" if nota else linea)
        salida.append("")

    tabla("lectura", L)
    tabla("pantalla", datos.get("pantalla", {}))
    tabla("figuras", datos.get("figuras", {}))
    tabla("citas", datos.get("citas", {}))
    tabla("musica", datos.get("musica", {}))
    for f in datos.get("figura", []):
        if f.get("nombre"):
            tabla("figura", f, "[figura]")
    for f in datos.get("fija", []):
        tabla("fija", f, "[fija]")
    for c in datos.get("cita", []):
        tabla("cita", c, "[cita]")
    for nombre, voz in datos.get("voces", {}).items():
        clave = nombre if re.fullmatch(r"[A-Za-z0-9_-]+", nombre) else toml_valor(nombre)
        tabla("voces", voz, f"voces.{clave}")
    (carpeta / "lectura.toml").write_text("\n".join(salida), encoding="utf-8")


def ancla_unica(todas: list[Linea], i: int, al_final: bool = False) -> str:
    """El texto más corto, desde el comienzo de la línea i (o hasta su final),
    que aparece una sola vez en las páginas. Así `desde` y `hasta` siguen
    valiendo aunque se cambie el rango de páginas."""
    t = Texto(todas)
    palabras = todas[i].texto.split()
    j = i
    while True:
        for n in range(min(4, len(palabras)), len(palabras) + 1):
            candidata = " ".join(palabras[-n:] if al_final else palabras[:n])
            if len(list(patron_flexible(candidata).finditer(t.cadena))) == 1:
                return candidata
        # La línea entera se repite: se le suma la vecina.
        j = j - 1 if al_final else j + 1
        if not 0 <= j < len(todas):
            return " ".join(palabras)
        palabras = (todas[j].texto.split() + palabras) if al_final else (palabras + todas[j].texto.split())


def config_desde_peticion(peticion: dict) -> Config:
    """La consola manda la configuración como el diccionario del toml, más
    `desde_i` y `hasta_i` (índices de líneas) cuando el tramo se eligió con
    clics: se convierten aquí en texto, que es lo que se guarda."""
    datos = json.loads(json.dumps(peticion.get("config", {})))
    carpeta = Path(peticion.get("carpeta") or ".").resolve()
    cfg = Config(carpeta, datos)
    L = datos.setdefault("lectura", {})
    for clave in ("libro", "paginas"):
        if not L.get(clave):
            raise ErrorLectura(f"falta `{clave}`")
    L.setdefault("numero", 0)
    L.setdefault("titulo", "")
    if not cfg.libro.exists():
        raise ErrorLectura(f"No encuentro el libro: {cfg.libro}")

    elegidos = {k: L.pop(k) for k in ("desde_i", "hasta_i") if k in L}
    fijas = [f for f in datos.get("fija", []) if "desde_i" in f or "hasta_i" in f]
    if elegidos or fijas:
        doc = pymupdf.open(cfg.libro)
        todas = []
        for p in cfg.paginas:
            todas.extend(lineas_de_pagina(doc[p - 1]))
        for f in fijas:
            if "desde_i" in f:
                f["desde"] = ancla_unica(todas, int(f.pop("desde_i")))
            if "hasta_i" in f:
                f["hasta"] = ancla_unica(todas, int(f.pop("hasta_i")), al_final=True)
        if "desde_i" in elegidos:
            i = int(elegidos["desde_i"])
            L["desde"] = "" if i <= 0 else ancla_unica(todas, i)
        if "hasta_i" in elegidos:
            i = int(elegidos["hasta_i"])
            L["hasta"] = "" if i >= len(todas) - 1 else ancla_unica(todas, i, al_final=True)
    return cfg


def libro_info(ruta: Path) -> dict:
    doc = pymupdf.open(ruta)
    meta = doc.metadata or {}
    return {
        "nombre": ruta.name, "paginas": len(doc),
        "titulo": meta.get("title") or ruta.stem, "autor": meta.get("author") or "",
        # Capítulos del índice del PDF: [nivel, título, página].
        "indice": [{"nivel": n, "titulo": nfc(t), "pagina": p} for n, t, p in doc.get_toc() if p > 0],
    }


def responder(obj) -> None:
    sys.stdout.write(json.dumps(obj, ensure_ascii=False))
    sys.stdout.write("\n")


def ordenes_json(orden: str, args) -> None:
    """Órdenes que usa el estudio: leen JSON de stdin y responden JSON."""
    requiere_pymupdf()
    try:
        if orden == "libro":
            responder(libro_info(Path(args.pdf)))
        elif orden == "config":
            carpeta = Path(args.carpeta).resolve()
            ruta = carpeta / "lectura.toml"
            if not ruta.exists():
                raise ErrorLectura("Esa lectura todavía no tiene lectura.toml.")
            try:
                datos = tomllib.loads(ruta.read_text(encoding="utf-8"))
            except tomllib.TOMLDecodeError as e:
                raise ErrorLectura(f"lectura.toml: {e}")
            cfg = Config(carpeta, datos)
            libro = cfg.libro if cfg.lectura.get("libro") else None
            responder({"config": datos,
                       "libroRuta": str(libro) if libro and libro.exists() else "",
                       "libroNombre": libro.name if libro else ""})
        elif orden == "analizar":
            cfg = config_desde_peticion(json.load(sys.stdin))
            doc = pymupdf.open(cfg.libro)
            responder(analisis_json(cfg, analizar(cfg, doc)))
        elif orden == "guardar":
            peticion = json.load(sys.stdin)
            carpeta = Path(args.carpeta).resolve()
            carpeta.mkdir(parents=True, exist_ok=True)
            peticion["carpeta"] = str(carpeta)
            cfg = config_desde_peticion(peticion)
            escribir_toml(carpeta, cfg.datos)
            cfg = cargar_config(carpeta)
            doc = pymupdf.open(cfg.libro)
            a = analizar(cfg, doc)
            marcas = escribir(cfg, doc, a)
            responder({"carpeta": str(carpeta), "titulo": cfg.titulo_video,
                       "lineas": len(marcas["lineas"]), "figuras": len(marcas["figuras"]),
                       "citas": len(marcas["clips"]), "avisos": a.avisos})
    except ErrorLectura as e:
        responder({"error": str(e)})
        sys.exit(2)
    except SystemExit:
        raise
    except Exception as e:  # el estudio muestra el mensaje; el rastro va a stderr
        import traceback
        traceback.print_exc()
        responder({"error": f"{type(e).__name__}: {e}"})
        sys.exit(1)


# ----------------------------------------------------------------------------
# nueva
# ----------------------------------------------------------------------------

PLANTILLA = '''\
# Lectura {numero:02d}: {titulo}
# Después de cambiar algo aquí:  ./estudio preparar {carpeta_rel}

[lectura]
curso = "{curso}"          # ME (Modelos estocásticos) o LP (Lenguajes de programación)
grupo = {grupo}             # dígito del grupo en el SIA
numero = {numero}
titulo = "{titulo}"
libro = "{libro}"
paginas = "{paginas}"     # páginas del PDF, no las impresas en el libro. Ej.: "61-72" o "61-72, 446"
{desde_linea}
{hasta_linea}

[pantalla]
dpi = 200                 # resolución de las páginas
ancho_figura = 0.38       # parte de la pantalla para la figura fija a la derecha (2.f)
segundos_figura = 3       # cuánto se deja una figura sin pie (2.e)

[figuras]
detectar = true           # busca «Figura N» y «Lámina X» en el texto y su imagen en el libro
persistente = true        # 2.f: fija la figura a la derecha en los párrafos que hablan de ella

# Para corregir una figura detectada, o agregar una que el texto no nombra:
# [[figura]]
# nombre = "Lámina FF"
# pagina = 465            # página del PDF donde está, si la detección falla
# persistente = false
# omitir = true

# Citas textuales con voz sintética (2.i). Una entrada por intervención, en orden.
# `texto` es la cita tal como aparece en el libro (basta con que coincidan las
# palabras; los saltos de línea no importan). Para citas largas usa `desde` y
# `hasta` con el comienzo y el final. `decir` reemplaza lo que dice la voz.
#
# [[cita]]
# personaje = "Pauli"
# texto = "«No más de dos electrones…»"
#
# [voces.Pauli]
# color = "#ffab3d"
# descripcion = "hombre de unos 40 años, austríaco, seguro de sí"
'''


def nueva(args) -> None:
    carpeta = ESTUDIO / "lecturas" / f"{args.numero:02d}-{slug(args.titulo)}"
    if (carpeta / "lectura.toml").exists() and not args.forzar:
        sys.exit(f"Ya existe {carpeta / 'lectura.toml'}. Edítalo, o usa --forzar para reescribirlo.")
    carpeta.mkdir(parents=True, exist_ok=True)
    libro = Path(os.path.expanduser(args.libro)).resolve()
    texto = PLANTILLA.format(
        numero=args.numero, titulo=args.titulo, curso=args.curso, grupo=args.grupo,
        libro=str(libro).replace(str(Path.home()), "~", 1), paginas=args.paginas,
        carpeta_rel=carpeta.relative_to(ESTUDIO),
        desde_linea=f'desde = "{args.desde}"' if args.desde else
        '# desde = "texto donde empieza"   # si la lectura no arranca al comienzo de la primera página',
        hasta_linea=f'hasta = "{args.hasta}"' if args.hasta else
        '# hasta = "texto donde termina"   # si no termina al final de la última',
    )
    (carpeta / "lectura.toml").write_text(texto, encoding="utf-8")
    print(f"  Creada {carpeta.relative_to(ESTUDIO)}/lectura.toml")
    preparar(carpeta)


# ----------------------------------------------------------------------------

def main() -> None:
    ap = argparse.ArgumentParser(description="Prepara una lectura para el estudio (lineamientos 3.2.3).")
    sub = ap.add_subparsers(dest="orden", required=True)

    n = sub.add_parser("nueva", help="crear la carpeta y el lectura.toml de una lectura")
    n.add_argument("--libro", required=True, help="ruta del PDF del libro")
    n.add_argument("--paginas", required=True, help='páginas del PDF, p. ej. "207-218"')
    n.add_argument("--numero", required=True, type=int, help="número de la lectura (correo de ganador)")
    n.add_argument("--titulo", required=True, help="título de la sección, como en el libro")
    n.add_argument("--curso", default="ME", choices=["ME", "LP"])
    n.add_argument("--grupo", default=1, type=int)
    n.add_argument("--desde", default="", help="texto donde empieza, si no es el comienzo de la página")
    n.add_argument("--hasta", default="", help="texto donde termina, si no es el final de la página")
    n.add_argument("--forzar", action="store_true", help="reescribir un lectura.toml que ya existe")

    p = sub.add_parser("preparar", help="regenerar páginas, figuras, citas y marcas.json")
    p.add_argument("carpeta")

    # Las usa el estudio: responden JSON.
    sub.add_parser("libro", help="(JSON) páginas e índice de un PDF").add_argument("pdf")
    sub.add_parser("config", help="(JSON) el lectura.toml de una carpeta").add_argument("carpeta")
    sub.add_parser("analizar", help="(JSON) analizar la configuración que llega por stdin")
    sub.add_parser("guardar", help="(JSON) escribir lectura.toml desde stdin y preparar").add_argument("carpeta")


    args = ap.parse_args()
    if args.orden == "nueva":
        nueva(args)
    elif args.orden == "preparar":
        preparar(Path(args.carpeta))
    else:
        ordenes_json(args.orden, args)


if __name__ == "__main__":
    main()
