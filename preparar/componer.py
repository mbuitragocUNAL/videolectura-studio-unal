"""El pasaje compuesto de nuevo, para el video.

El render mostraba las páginas del libro tal cual: con sus márgenes, con el
hueco entre una página y la siguiente, y con el texto más pequeño cada vez que
una figura fija a la derecha le quitaba espacio. El profesor pide otra cosa
(lineamientos 2.c y 2.f): el texto ocupando casi toda la pantalla, y del mismo
tamaño con o sin figura al lado; «si hay menos espacio para letras iguales,
saldrán más líneas de texto».

Así que el pasaje se vuelve a componer, con las fuentes del propio libro
(Times New Roman, embebida en el PDF) y a su mismo tamaño, en dos columnas:

  ancha     casi todo el ancho de la pantalla, para cuando no hay figura;
  angosta   la que deja libre la figura fija a la derecha.

Las dos con la misma letra, así que la angosta tiene más líneas. Cada columna
es una tira continua, sin cortes de página, y el render la desliza mientras
se lee. Para eso necesita saber dónde quedó cada línea: tipografia.json dice,
para cada párrafo (por el número de su primera línea en marcas.json), dónde
empieza cada línea compuesta, como posición en el texto que el estudio lee de
ese párrafo, y a qué altura de la tira quedó.
"""
from __future__ import annotations

import html as h
import json
import statistics
import unicodedata
from pathlib import Path

import pymupdf

ANCHO, ALTO = 1920, 1080   # el video
MARGEN = 60                # a izquierda y derecha: el texto ocupa ~94 % del ancho
HUECO_FIGURA = 44          # entre la columna angosta y la figura
PAGINA_PT = 14000          # alto de cada página de composición (límite de PDF: 14 400)


def _letras(texto: str) -> tuple[str, list[int]]:
    """Solo letras y cifras, en minúscula, con la posición de cada una en texto."""
    out, pos = [], []
    for i, c in enumerate(texto):
        # NFKC deshace las ligaduras de la composición («ﬁ» → «fi»).
        for d in unicodedata.normalize("NFKC", c):
            if d.isalnum():
                out.append(d.lower())
                pos.append(i)
    return "".join(out), pos


VOCALES = set("aeiouáéíóúüAEIOUÁÉÍÓÚÜ")
INSEPARABLES = {"bl", "br", "cl", "cr", "dr", "fl", "fr", "gl", "gr", "kl", "kr",
                "pl", "pr", "tr", "ch", "ll", "rr", "qu", "gu"}
GUION_BLANDO = "\u00ad"


def silabear(palabra: str) -> list[int]:
    """Dónde se puede partir una palabra española: índices de corte.

    Las reglas de siempre: una consonante entre vocales va con la siguiente
    (ca-sa), dos se reparten salvo que formen grupo (bl, tr, ch, ll: o-tro,
    ac-to), de tres o cuatro pasan dos a la siguiente si forman grupo
    (ins-truir, cons-truir). Nunca se separan vocales, aunque sea hiato: es lo
    seguro sin saber dónde va el acento. Y no se deja menos de dos letras
    antes del corte ni menos de tres después."""
    w = palabra.lower()
    n = len(w)
    # Muy cortas, con cifras, o extranjeras (buckyball, Kroto): sin partir.
    if n < 5 or not w.isalpha() or any(c in w for c in "kw"):
        return []
    vocal = [c in VOCALES for c in w]
    # «qu» y «gu» delante de e, i: la u no suena, va con la consonante.
    for i in range(1, n - 1):
        if w[i] == "u" and w[i - 1] in "qg" and w[i + 1] in "eéií":
            vocal[i] = False
    cortes = []
    i = 0
    while i < n:
        if not vocal[i]:
            i += 1
            continue
        j = i
        while j < n and vocal[j]:
            j += 1
        k = j
        while k < n and not vocal[k]:
            k += 1
        if k >= n:
            break
        cons = w[j:k]
        m = len(cons)
        if m == 1:
            corte = j
        elif m == 2:
            corte = j if cons in INSEPARABLES else j + 1
        elif m == 3:
            corte = j + 1 if cons[1:] in INSEPARABLES else j + 2
        else:
            corte = j + 2
        if 2 <= corte <= n - 3:
            cortes.append(corte)
        i = k
    return cortes


def partir(html: str) -> str:
    """El HTML con guiones blandos donde las palabras se pueden partir, solo en
    el texto, nunca dentro de una etiqueta."""
    import re
    def palabra(m):
        w = m.group(0)
        c = silabear(w)
        if not c:
            return w
        out, prev = [], 0
        for k in c:
            out.append(w[prev:k])
            prev = k
        out.append(w[prev:])
        return GUION_BLANDO.join(out)
    # Y sin ligaduras: la fuente del libro viene recortada a los caracteres
    # que usa, y «ﬁ» o «ﬂ» no están; compuestas, «definición» salía «de
    # nición». Un ZWNJ entre las dos letras impide la ligadura y no se ve.
    # Solo en el texto: dentro de una etiqueta rompería «margin-left».
    def texto(t):
        return re.sub(r"f(?=[filt])", "f\u200c", re.sub(r"[^\W\d_]+", palabra, t))
    partes = re.split(r"(<[^>]*>)", html)
    return "".join(p if p.startswith("<") else texto(p) for p in partes)


def _fuentes(doc, paginas: list[int], destino: Path) -> dict[str, str]:
    """Extrae del PDF las fuentes del pasaje: regular, cursiva, negrita."""
    destino.mkdir(parents=True, exist_ok=True)
    vistas: dict[str, str] = {}
    for p in paginas:
        for xref, ext, _tipo, nombre, *_ in doc[p - 1].get_fonts(full=True):
            base = nombre.split("+")[-1].lower()
            estilo = ("negrita-cursiva" if "bold" in base and "italic" in base
                      else "negrita" if "bold" in base
                      else "cursiva" if "italic" in base or "oblique" in base
                      else "regular")
            if estilo in vistas or ext not in ("ttf", "otf", "cff", "pfa", "pfb"):
                continue
            _n, ext2, _t, buf = doc.extract_font(xref)
            if not buf:
                continue
            archivo = f"{estilo}.{ext2 or ext}"
            (destino / archivo).write_bytes(buf)
            vistas[estilo] = archivo
    return vistas


def _css(fuentes: dict[str, str], cuerpo: float, interlinea: float) -> str:
    caras = []
    for estilo, archivo in fuentes.items():
        extra = {"cursiva": "font-style: italic;", "negrita": "font-weight: bold;",
                 "negrita-cursiva": "font-style: italic; font-weight: bold;"}.get(estilo, "")
        caras.append(f"@font-face {{font-family: libro; src: url({archivo}); {extra}}}")
    familia = "libro" if fuentes else "serif"
    return "\n".join(caras) + f"""
body {{margin: 0; padding: 0; font-family: {familia}; font-size: {cuerpo}pt; line-height: {interlinea}pt; text-align: justify;}}
p {{margin-top: 0; margin-bottom: 0;}}
h2 {{margin: {interlinea * 1.2:.1f}pt 0 {interlinea * 0.6:.1f}pt 0; text-align: center; font-weight: normal;}}
p.centro {{text-align: center;}}
p.cita {{margin-top: {interlinea * 0.4:.1f}pt; margin-bottom: {interlinea * 0.4:.1f}pt; margin-left: {SANGRIA_CITA:.1f}pt; margin-right: {SANGRIA_CITA:.1f}pt; text-align: left;}}
"""


VINETAS = ("•", "◦", "▪", "‣")


# Sangría, en puntos, que lleva a cada lado una cita marcada por el lector. Un
# valor fijo e igual para todas, como en el libro: así se reconoce la voz
# citada frente al texto del autor, sin depender de la geometría del PDF. Tiene
# que ser mayor que la sangría de primera línea de un párrafo normal (unos
# 30 pt en este libro), o la cita se vería menos adentrada que el texto.
SANGRIA_CITA = 42.0


def _unir(ls) -> str:
    """El HTML de varias líneas seguidas: un guion al final de línea une sin espacio."""
    cuerpo = ""
    for l in ls:
        frag = l.html or h.escape(l.texto, quote=False)
        if cuerpo and not (cuerpo.endswith("-") and not cuerpo.endswith(" -")):
            cuerpo += " "
        cuerpo += frag.strip()
    return cuerpo


def _bloque(ls, x0_texto, x1_texto, centro, cuerpo_pt) -> str:
    """Un bloque de texto con la sangría, el margen y el centrado que tenía en la página."""
    primera, resto = ls[0], ls[1:]

    # Una cita que el lector marcó se compone como en el libro: sangrada a
    # ambos lados y en su propia letra (más pequeña), que es la señal visual de
    # que la voz no es la del autor sino la de quien se cita. El texto fluye y
    # se parte por el ancho del bloque como en el libro, con la sangría intacta
    # en cada línea de continuación — no se mete <br> por cada línea física del
    # PDF, que partía una misma oración en dos.
    if any(getattr(l, "cita_estilo", False) for l in ls):
        # El tamaño propio de la cita, un punto por debajo del cuerpo cuando el
        # PDF no lo marca más pequeño (en este libro las citas miden lo mismo
        # que el cuerpo en los metadatos, aunque en la página se vean menores).
        tam = statistics.median(l.tam for l in ls)
        if tam >= cuerpo_pt - 0.3:
            tam = cuerpo_pt * 0.9
        # El texto fluye y se parte por el ancho, salvo entre dos renglones de
        # cita distintos: una línea del PDF que cierra con punto (o ! ?) y va
        # seguida de otra es un renglón propio del poema, no una continuación
        # por ancho, así que lleva un salto. Lo demás se une y se rejustifica.
        cuerpo = ""
        for i, l in enumerate(ls):
            frag = (l.html or h.escape(l.texto, quote=False)).strip()
            if i == 0:
                cuerpo = frag
                continue
            # La línea anterior, sin cierres de comillas/paréntesis, para ver
            # si cerró una oración: «…curvarse.» termina renglón; «…espacio-»
            # continúa por ancho; «…recto (en el espacio-» también.
            prev = ls[i - 1].texto.rstrip().rstrip("»”\")")
            if cuerpo.endswith("-") and not cuerpo.endswith(" -"):
                cuerpo += frag                       # palabra partida por guion
            elif prev.endswith((".", "!", "?")):
                cuerpo += "<br>" + frag               # renglón de cita propio
            else:
                cuerpo += " " + frag                  # continúa la misma oración
        # Todo en el style inline: en la columna real la regla .cita del CSS no
        # bastó para sangrar (el margen quedaba en cero), así que el margen va
        # aquí, donde PyMuPDF Story sí lo aplica.
        return (f'<p class="cita" style="margin-left: {SANGRIA_CITA:.1f}pt; '
                f'margin-right: {SANGRIA_CITA:.1f}pt; font-size: {tam:.1f}pt">{cuerpo}</p>')

    cuerpo = _unir(ls)
    if not resto and abs((primera.x0 + primera.x1) / 2 - centro) < 14 and primera.x0 > x0_texto + 24:
        return f'<p class="centro">{cuerpo}</p>'
    cuerpo_x = min(l.x0 for l in resto) if resto else x0_texto
    margen = max(0.0, cuerpo_x - x0_texto)
    sangria = primera.x0 - cuerpo_x
    return f'<p style="margin-left: {margen:.1f}pt; text-indent: {sangria:.1f}pt">{cuerpo}</p>'


def _parrafos(lineas, abre, x0_texto, x1_texto, cuerpo_pt):
    """Los párrafos como los ve el estudio (marcas.json), con su HTML.

    Cada párrafo empieza donde marcas.json pone la sangría; los pies de figura
    no se leen como texto y no entran. La sangría, el margen de las listas y
    lo centrado salen de dónde estaba cada línea en la página. `cuerpo_pt` es
    el tamaño del texto normal, para las citas marcadas por el lector."""
    grupos, actual = [], None
    for i, l in enumerate(lineas):
        if abre[i]:
            actual = {"n": i + 1, "lineas": []}
            grupos.append(actual)
        if l.tipo != "pie" and actual is not None and l.texto.strip():
            actual["lineas"].append(l)
    centro = (x0_texto + x1_texto) / 2
    out = []
    for g in grupos:
        ls = g["lineas"]
        if not ls:
            continue
        # El texto como lo lee el estudio: las líneas unidas por un espacio.
        dicho = " ".join(l.texto.strip() for l in ls)
        tam = statistics.median(l.tam for l in ls)
        if ls[0].tipo == "titulo":
            html = f'<h2 style="font-size: {tam:.1f}pt">{_unir(ls)}</h2>'
        else:
            # Un párrafo se parte en bloques en cada viñeta (•) de una lista y
            # en cada cambio entre texto normal y una cita marcada, para que la
            # cita sea su propio bloque con su estilo.
            items, actual = [], []
            for l in ls:
                nueva = l.texto.lstrip().startswith(VINETAS)
                cambia_cita = actual and getattr(l, "cita_estilo", False) != getattr(actual[-1], "cita_estilo", False)
                if actual and (nueva or cambia_cita):
                    items.append(actual)
                    actual = []
                actual.append(l)
            items.append(actual)
            html = "".join(_bloque(item, x0_texto, x1_texto, centro, cuerpo_pt) for item in items)
        out.append({"n": g["n"], "dicho": dicho, "html": partir(html)})
    return out


def _componer(parrafos, css, archivo_fuentes: Path, ancho_pt: float, escala: float, salida: Path):
    """Compone todos los párrafos en una columna de ancho_pt y la guarda como
    una sola imagen alta. Devuelve su alto en píxeles y las líneas."""
    html = "\n".join(p["html"] for p in parrafos)
    story = pymupdf.Story(html=html, user_css=css, archive=pymupdf.Archive(str(archivo_fuentes)))
    pdf = salida.with_suffix(".pdf")
    escritor = pymupdf.DocumentWriter(str(pdf))
    rect = pymupdf.Rect(0, 0, ancho_pt, PAGINA_PT)
    llenos = []
    mas = 1
    while mas:
        disp = escritor.begin_page(rect)
        mas, lleno = story.place(rect)
        story.draw(disp)
        escritor.end_page()
        llenos.append(lleno[3])
    escritor.close()

    doc = pymupdf.open(pdf)
    escalar = pymupdf.Matrix(escala, escala)
    piezas, lineas, y_px = [], [], 0
    for k, pagina in enumerate(doc):
        alto_pt = llenos[k] + 2
        pix = pagina.get_pixmap(matrix=escalar, clip=pymupdf.Rect(0, 0, ancho_pt, alto_pt), alpha=False)
        for b in pagina.get_text("dict")["blocks"]:
            for ln in b.get("lines", []):
                texto = "".join(s["text"] for s in ln["spans"])
                if texto.strip():
                    x0, y0, _x1, y1 = ln["bbox"]
                    lineas.append({"t": texto, "x": x0, "y": round(y_px + y0 * escala),
                                   "h": round((y1 - y0) * escala)})
        piezas.append((pix, y_px))
        y_px += pix.height
    doc.close()
    pdf.unlink()

    # Las páginas una debajo de otra, y debajo una pantalla en blanco para que
    # la última línea también pueda subir a la altura de lectura.
    ancho_px = piezas[0][0].width
    total = y_px + ALTO
    tira = pymupdf.Pixmap(pymupdf.csRGB, pymupdf.IRect(0, 0, ancho_px, total), False)
    tira.clear_with(255)
    for pix, y in piezas:
        pix.set_origin(0, y)
        tira.copy(pix, pix.irect)
    tira.save(str(salida))
    # Una línea justificada a veces sale en trozos; se vuelven a juntar.
    lineas.sort(key=lambda l: (l["y"], l["x"]))
    juntas: list[dict] = []
    for l in lineas:
        if juntas and abs(juntas[-1]["y"] - l["y"]) <= 3:
            juntas[-1]["t"] += " " + l["t"]
        else:
            juntas.append({"t": l["t"], "y": l["y"], "h": l["h"]})
    return total, ancho_px, juntas


def _ubicar(parrafos, lineas):
    """Para cada línea compuesta, de qué párrafo es y en qué posición de su
    texto (el que lee el estudio) empieza. Se comparan solo letras y cifras:
    la composición cambia espacios y une palabras partidas, no letras."""
    flujos = []
    for p in parrafos:
        letras, pos = _letras(p["dicho"])
        flujos.append((p["n"], letras, pos))
    out: dict[str, list] = {}
    k, i = 0, 0
    perdidas = 0
    for ln in lineas:
        letras, _ = _letras(ln["t"])
        if not letras:
            continue
        while k < len(flujos) and i >= len(flujos[k][1]):
            k, i = k + 1, 0
        if k >= len(flujos):
            break
        n, flujo, pos = flujos[k]
        if not flujo.startswith(letras[:12], i):
            # Algo no coincide (un signo que la composición cambió): se busca
            # el comienzo de la línea un poco más adelante.
            j = flujo.find(letras[:12], i)
            if j < 0 or j - i > 40:
                perdidas += 1
                continue
            i = j
        out.setdefault(str(n), []).append([pos[i], ln["y"], ln["h"]])
        i += len(letras)
    return out, perdidas


def componer(cfg, doc, a, lineas, abre, ancho_figura: float) -> dict:
    """Escribe tipografia/ (las dos columnas y las fuentes) y tipografia.json."""
    carpeta = cfg.carpeta / "tipografia"
    paginas = sorted({l.pag for l in lineas})
    fuentes = _fuentes(doc, paginas, carpeta)

    cuerpo_lineas = [l for l in lineas if l.tipo == "texto"]
    cuerpo = statistics.median(l.tam for l in cuerpo_lineas) if cuerpo_lineas else 11.0
    pasos = [b.y0 - a_.y0 for a_, b in zip(cuerpo_lineas, cuerpo_lineas[1:])
             if a_.pag == b.pag and a_.parrafo == b.parrafo and 0 < b.y0 - a_.y0 < cuerpo * 2]
    interlinea = statistics.median(pasos) if pasos else cuerpo * 1.2

    ancho_libro = a.x_texto1 - a.x_texto0
    escala = (ANCHO - 2 * MARGEN) / ancho_libro            # píxeles por punto
    angosto_px = ANCHO * (1 - ancho_figura) - HUECO_FIGURA - MARGEN

    parrafos = _parrafos(lineas, abre, a.x_texto0, a.x_texto1, cuerpo)
    css = _css(fuentes, round(cuerpo, 2), round(interlinea, 2))
    out = {"video": [ANCHO, ALTO], "escala": round(escala, 4), "cuerpoPt": round(cuerpo, 2),
           "lectura": 0.38, "figura": {"x0": round(ANCHO * (1 - ancho_figura)), "margen": MARGEN},
           "columnas": {}}
    for nombre, ancho_px in (("ancha", ANCHO - 2 * MARGEN), ("angosta", angosto_px)):
        archivo = carpeta / f"{nombre}.png"
        alto, real, lineas_c = _componer(parrafos, css, carpeta, ancho_px / escala, escala, archivo)
        mapa, perdidas = _ubicar(parrafos, lineas_c)
        out["columnas"][nombre] = {"archivo": f"tipografia/{nombre}.png", "x": MARGEN, "ancho": real,
                                   "alto": alto, "lineas": len(lineas_c), "sinUbicar": perdidas,
                                   "parrafos": mapa}
    (cfg.carpeta / "tipografia.json").write_text(json.dumps(out, ensure_ascii=False), encoding="utf-8")
    return out
