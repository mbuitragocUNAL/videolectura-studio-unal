#!/usr/bin/env python3
"""La miniatura de YouTube de una lectura (1280×720).

Misma cara que la del capítulo 18: fondo oscuro con una nube tenue de puntos,
el escudo arriba a la izquierda, «LECTURA NN» en dorado, el título grande, la
barra de colores y una línea que dice de qué trata. A la derecha, si se le da,
una figura del libro fundida con el fondo. Las miniaturas se leen del tamaño
de una estampilla: pocas palabras, muy grandes.

    python3 preparar/miniatura.py lecturas/22-… --titulo "Belleza cuántica II" \\
        --destacado "exuberancia" --linea "Diamante · grafeno · nanotubos · buckyballs" \\
        --figura figuras/figura-30.png --salida estudio/output/miniatura.png
"""
from __future__ import annotations

import argparse
import random
import tomllib
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

MARCA = Path(__file__).resolve().parent / "marca"
ANCHO, ALTO, K = 1280, 720, 2  # se dibuja al doble y se reduce: bordes limpios

FONDO = (16, 19, 26)
TINTA = (236, 230, 216)
APAGADO = (138, 143, 156)
COLORES = [(93, 169, 233), (242, 184, 75), (167, 139, 250), (79, 193, 166), (232, 112, 106)]
DORADO = COLORES[1]


def fuente(peso: str, tam: float) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(str(MARCA / f"IBMPlexSans-{peso}.ttf"), int(tam * K))


def espaciado(draw, xy, texto, f, color, extra):
    """Versalitas espaciadas, como las etiquetas del capítulo 18."""
    x, y = xy
    for c in texto:
        draw.text((x, y), c, font=f, fill=color)
        x += draw.textlength(c, font=f) + extra * K
    return x


def ajustar(draw, texto, peso, tam, ancho):
    """El tamaño más grande, hasta tam, con que texto cabe en ancho."""
    while tam > 20 and draw.textlength(texto, font=fuente(peso, tam)) > ancho * K:
        tam -= 2
    return fuente(peso, tam)


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("carpeta")
    ap.add_argument("--titulo", required=True, help="primera línea del título")
    ap.add_argument("--destacado", default="", help="segunda línea, más grande")
    ap.add_argument("--linea", default="", help="de qué trata, en una línea")
    ap.add_argument("--figura", default="", help="imagen para la derecha, relativa a la carpeta")
    ap.add_argument("--captura", default="",
                    help="en vez de --figura: una captura de pantalla, enmarcada a la derecha")
    ap.add_argument("--antetitulo", default="", help="en dorado sobre el título; por defecto «LECTURA NN»")
    ap.add_argument("--salida", default="estudio/output/miniatura.png")
    a = ap.parse_args()

    # Una carpeta de lectura da el número, el curso y el grupo; cualquier otra
    # (la guía, por ejemplo) usa el curso ME01 y necesita --antetitulo.
    carpeta = Path(a.carpeta)
    toml = carpeta / "lectura.toml"
    lectura = tomllib.loads(toml.read_text(encoding="utf-8"))["lectura"] if toml.exists() else {}
    numero, curso, grupo = int(lectura.get("numero", 0)), lectura.get("curso", "ME"), int(lectura.get("grupo", 1))
    antetitulo = a.antetitulo or f"LECTURA {numero:02d}"
    nombre_curso = {"ME": "Modelos Estocásticos y Simulación", "LP": "Lenguajes de Programación"}.get(curso, curso)

    img = Image.new("RGB", (ANCHO * K, ALTO * K), FONDO)

    # La nube de puntos: tres manchas gaussianas, apenas visibles.
    nube = Image.new("RGBA", img.size, (0, 0, 0, 0))
    dn = ImageDraw.Draw(nube)
    rng = random.Random(numero)
    centros = [(0.12, 0.25, 0.16), (0.85, 0.7, 0.18), (0.5, 0.95, 0.12)]
    for _ in range(560):
        cx, cy, s = rng.choices(centros, weights=[0.4, 0.4, 0.2])[0]
        x, y = rng.gauss(cx, s) * ANCHO * K, rng.gauss(cy, s * 1.6) * ALTO * K
        r = rng.uniform(1.2, 3.0) * K
        c = rng.choice(COLORES)
        dn.ellipse((x - r, y - r, x + r, y + r), fill=(*c, int(255 * rng.uniform(0.18, 0.55))))
    img.paste(nube, (0, 0), nube)
    draw = ImageDraw.Draw(img)

    # La figura a la derecha, fundida con el fondo por un borde suave.
    ancho_texto = 1180
    if a.figura:
        fig = Image.open(carpeta / a.figura).convert("RGB")
        # Sin el margen blanco del recorte: solo la imagen.
        oscura = fig.convert("L").point(lambda v: 255 if v < 200 else 0)
        if (caja := oscura.getbbox()) is not None:
            fig = fig.crop((caja[0] + 4, caja[1] + 4, caja[2] - 4, caja[3] - 4))
        lado = int(470 * K)
        fig.thumbnail((lado, lado))
        mascara = Image.new("L", fig.size, 0)
        dm = ImageDraw.Draw(mascara)
        m = int(28 * K)
        dm.ellipse((m, m, fig.width - m, fig.height - m), fill=255)
        mascara = mascara.filter(ImageFilter.GaussianBlur(26 * K))
        x0 = ANCHO * K - fig.width - int(40 * K)
        y0 = (ALTO * K - fig.height) // 2 + int(40 * K)
        img.paste(fig, (x0, y0), mascara)
        ancho_texto = (x0 / K) - 70 - 40
    elif a.captura:
        # Una pantalla del video, en un marco con esquinas redondeadas y
        # sombra, inclinada apenas hacia el centro.
        cap = Image.open(carpeta / a.captura).convert("RGB")
        ancho = int(560 * K)
        cap = cap.resize((ancho, int(cap.height * ancho / cap.width)), Image.LANCZOS)
        radio, borde = int(16 * K), int(3 * K)
        marco = Image.new("RGBA", (cap.width + 2 * borde, cap.height + 2 * borde), (0, 0, 0, 0))
        ImageDraw.Draw(marco).rounded_rectangle((0, 0, marco.width - 1, marco.height - 1), radio + borde,
                                                fill=(43, 50, 59, 255))
        mascara = Image.new("L", cap.size, 0)
        ImageDraw.Draw(mascara).rounded_rectangle((0, 0, cap.width - 1, cap.height - 1), radio, fill=255)
        marco.paste(cap, (borde, borde), mascara)
        marco = marco.rotate(-3, resample=Image.BICUBIC, expand=True)
        sombra = Image.new("RGBA", (marco.width + 120 * K, marco.height + 120 * K), (0, 0, 0, 0))
        sombra.paste((0, 0, 0, 150), (60 * K, 80 * K, 60 * K + marco.width, 80 * K + marco.height), marco)
        sombra = sombra.filter(ImageFilter.GaussianBlur(28 * K))
        x0 = ANCHO * K - marco.width - int(30 * K)
        y0 = (ALTO * K - marco.height) // 2 + int(55 * K)
        img.paste(sombra, (x0 - 60 * K, y0 - 60 * K), sombra)
        img.paste(marco, (x0, y0), marco)
        ancho_texto = (x0 / K) - 70 - 30

    # Encabezado: escudo, universidad, curso.
    tx = 44 * K
    ruta_escudo = MARCA / "escudo-unal-blanco.png"
    if ruta_escudo.exists():   # no viene en el repositorio: preparar/marca/README.md
        escudo = Image.open(ruta_escudo).convert("RGBA")
        h = int(84 * K)
        escudo = escudo.resize((int(escudo.width * h / escudo.height), h), Image.LANCZOS)
        img.paste(escudo, (int(44 * K), int(42 * K)), escudo)
        tx = 44 * K + escudo.width + 26 * K
    espaciado(draw, (tx, 52 * K), "UNIVERSIDAD NACIONAL DE COLOMBIA", fuente("Medium", 21), APAGADO, 5)
    draw.text((tx, 84 * K), f"{nombre_curso} · {curso}0{grupo}", font=fuente("Regular", 26), fill=APAGADO)

    # El bloque del título.
    x, y = 70 * K, 190 * K
    espaciado(draw, (x, y), antetitulo, fuente("Medium", 40), DORADO, 9)
    y += 72 * K
    f1 = ajustar(draw, a.titulo, "SemiBold", 76, ancho_texto)
    draw.text((x, y), a.titulo, font=f1, fill=TINTA)
    y += f1.size * 1.12
    if a.destacado:
        f2 = ajustar(draw, a.destacado, "SemiBold", 128, ancho_texto)
        draw.text((x, y - 10 * K), a.destacado, font=f2, fill=TINTA)
        y += f2.size * 1.08
    # La barra de colores.
    y += 6 * K
    for i, c in enumerate(COLORES):
        bx = x + i * 104 * K
        draw.rectangle((bx, y, bx + 98 * K, y + 9 * K), fill=c)
    y += 34 * K
    if a.linea:
        draw.text((x, y), a.linea, font=ajustar(draw, a.linea, "Regular", 36, ancho_texto), fill=APAGADO)

    salida = carpeta / a.salida
    salida.parent.mkdir(parents=True, exist_ok=True)
    img.resize((ANCHO, ALTO), Image.LANCZOS).save(salida, optimize=True)
    print(salida)


if __name__ == "__main__":
    main()
