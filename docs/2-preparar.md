# 2 · Preparar la lectura

[Video de este paso](https://youtu.be/jfDIFUssoig) · [← Instalar](1-instalar.md) · [Siguiente: voces y música →](3-voces-y-musica.md)

Preparar es decirle al estudio **qué parte del libro vas a leer**. Todo se elige
mirando las páginas: a la derecha se ven con las líneas que encontró el
análisis encima, y cada cambio se vuelve a analizar en uno o dos segundos.

En **Lecturas › Nueva lectura** (o «Preparar» en una que ya existe):

## Los pasos del panel

1. **Libro.** Elige uno del estante o pulsa **Subir un PDF**. Los libros quedan en
   `libros/`, fuera de git: cada quien tiene su copia.
2. **Páginas.** El índice del PDF lista los capítulos: un clic elige el capítulo
   y su título. También se pueden escribir las páginas (`207-218`, o `61-72, 446`
   si una lámina está al final del libro). Son páginas **del PDF**, no las impresas.
3. **Dónde empieza y termina.** Pulsa una línea en las páginas: «la lectura
   empieza aquí» o «termina aquí». Las secciones del capítulo tienen atajos
   (empezar en una, terminar antes de otra). Lo que queda fuera se ve lavado.
4. **Figuras y láminas.** El estudio busca cada «Figura N» y «Lámina X» que el
   texto nombra, con su recorte, su página y cuántas veces se nombra. En cada una puedes:
   - quitarla, o corregir la página donde está;
   - apagar «fija donde la nombran» (2.f);
   - **fijarla en otro párrafo**: para los párrafos que hablan de una figura sin
     nombrarla («A la izquierda tenemos…»). Pulsa «Fijar en otro párrafo» y luego el párrafo.

   Debajo eliges cuántos segundos se queda una figura sin pie (2–3, lineamiento
   2.e) y qué parte de la pantalla ocupa la figura fija.
5. **Citas con voz sintética.** Las sugerencias son frases entre comillas
   angulares («…») de cuatro palabras o más; un clic las agrega. Para cualquier
   otro fragmento, cambia a la vista **Texto**, selecciona el texto y agrégalo.
   Cada cita lleva su **personaje**. Las voces se eligen aquí mismo: ver
   [Voces y música](3-voces-y-musica.md).
6. **Datos del video.** Curso, grupo, número de lectura y título. Abajo se ve el
   título de YouTube como queda (2.k: `ME01 Lectura 22 Belleza cuántica II: exuberancia`)
   y la **duración estimada** a tres velocidades de lectura, contra los 5–25 minutos.
7. **Música de fondo.** Opcional: ver [Voces y música](3-voces-y-musica.md).

Pulsa **Guardar y preparar**. El estudio escribe `lectura.toml`, recorta las
páginas, extrae las figuras con y sin pie, y compone el texto para el video.
Luego, **Grabar** abre la lectura en la consola.

## El texto compuesto

Al preparar, el estudio vuelve a componer tu pasaje con la letra del propio
libro (la extrae del PDF), al mismo tamaño, en dos columnas: una ancha para toda
la pantalla y una angosta para cuando hay una figura fija a la derecha. Así el
video cumple 2.c (el texto llena la pantalla) y 2.f (el texto no cambia de
tamaño junto a una figura). No tienes que hacer nada: solo saber que el video
no muestra las páginas tal cual, sino el mismo texto sin márgenes ni cortes.

## `lectura.toml`

Es la fuente de todo: lo que eliges con clics se guarda ahí como texto del
libro (por ejemplo `desde = "El análisis de la materia"`), así que sigue valiendo
aunque cambies el rango de páginas. Se puede editar a mano y volver a preparar
con `./estudio preparar lecturas/22-…`.

## Antes de grabar, revisa

- [ ] El rango empieza y termina donde debe (el título de la sección incluido).
- [ ] Todas las figuras y láminas que el texto nombra aparecen, en su página.
- [ ] Las que tienen **texto dentro de la imagen** (rótulos, fórmulas): tendrás que leerlo después del pie (2.d).
- [ ] Cada cita tiene su personaje, y los párrafos que hablan de una figura sin nombrarla la tienen fija.
- [ ] La duración estimada queda entre 5 y 25 minutos.
