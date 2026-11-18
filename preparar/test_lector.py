"""Pruebas de lector.py que no necesitan el libro: un PDF armado aquí mismo."""
import unittest

import pymupdf

import lector


def imagen(pagina, rect):
    pix = pymupdf.Pixmap(pymupdf.csRGB, pymupdf.IRect(0, 0, 40, 30), False)
    pix.clear_with(120)
    pagina.insert_image(rect, pixmap=pix, keep_proportion=False)


def pie(pagina, y, texto):
    pagina.insert_textbox(pymupdf.Rect(72, y, 540, y + 40), texto, fontsize=10)


class LocalizarFigura(unittest.TestCase):
    def setUp(self):
        # Como las láminas de Wilczek, p. 464-465: la imagen de EE quedó al
        # final de una página y su pie al comienzo de la siguiente, encima de
        # la imagen de FF (que sí tiene su pie debajo).
        self.doc = pymupdf.open()
        p1 = self.doc.new_page(width=612, height=792)
        imagen(p1, pymupdf.Rect(140, 72, 470, 270))
        pie(p1, 290, "LÁMINA DD. Un rayo de luz parece tener diferentes colores.")
        imagen(p1, pymupdf.Rect(140, 416, 470, 660))            # la de EE, sin pie debajo
        p2 = self.doc.new_page(width=612, height=792)
        pie(p2, 74, "LÁMINA EE. El arte anamórfico no solo soporta los cambios de perspectiva.")
        imagen(p2, pymupdf.Rect(145, 168, 467, 467))            # la de FF
        pie(p2, 486, "LÁMINA FF. Al enrollar una hoja de grafeno obtenemos los nanotubos.")

    def test_la_imagen_partida_se_busca_en_la_pagina_anterior(self):
        pag, img, _, texto, pag_img = lector.localizar_figura(self.doc, "lamina", "EE", [], None)
        self.assertEqual((pag, pag_img), (2, 1))
        self.assertAlmostEqual(img.y0, 416, delta=1)
        self.assertTrue(texto.startswith("LÁMINA EE"))

    def test_la_imagen_con_su_pie_debajo_no_se_la_lleva_otro(self):
        pag, img, _, _, pag_img = lector.localizar_figura(self.doc, "lamina", "FF", [], None)
        self.assertEqual((pag, pag_img), (2, 2))
        self.assertAlmostEqual(img.y0, 168, delta=1)

    def test_la_de_siempre_sigue_igual(self):
        pag, img, _, _, pag_img = lector.localizar_figura(self.doc, "lamina", "DD", [], None)
        self.assertEqual((pag, pag_img), (1, 1))
        self.assertAlmostEqual(img.y0, 72, delta=1)

    def test_la_figura_partida_se_junta_imagen_arriba(self):
        pag, img, r_pie, _, pag_img = lector.localizar_figura(self.doc, "lamina", "EE", [], None)
        pix = lector.figura_partida(self.doc[pag_img - 1], img, self.doc[pag - 1], r_pie, 72)
        self.assertAlmostEqual(pix.height, img.height + 10 + r_pie.height, delta=2)


if __name__ == "__main__":
    unittest.main()
