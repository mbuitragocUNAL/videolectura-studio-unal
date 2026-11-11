# Guía en video: Videolectura Studio UNAL

Unos tres minutos y medio, para el resto del curso. Cada escena: lo que dice
la narración (Qwen3-TTS, voz local) y lo que se ve. Las imágenes del estudio
son capturas de la consola real; los fragmentos de video son de la lectura 22.

## 1. Portada

**Narración:** Esto es Videolectura Studio, una herramienta hecha por
estudiantes de la Universidad Nacional para grabar las video-lecturas del
curso. Tú lees; el estudio se encarga de que el video cumpla los lineamientos.

**Imagen:** escudo, «VIDEOLECTURA STUDIO · UNAL», «Graba tu video-lectura. El
estudio se encarga de los lineamientos.»

## 2. El problema

**Narración:** Una video-lectura tiene muchas reglas. La página debe llenar la
pantalla. Cada figura se muestra sola con su pie la primera vez que se nombra,
y una flecha marca dónde sigue la lectura. Mientras se habla de una figura,
queda fija a la derecha sin que el texto cambie de tamaño. Las citas van con voz
sintética, siempre la misma para cada personaje. Hacer todo eso a mano en un
editor toma horas. Aquí cada regla es automática.

**Imagen:** las reglas una a una, con su número de lineamiento (2.c, 2.d, 2.f, 2.i).

## 3. Instalar y abrir

**Narración:** Solo necesitas Docker y Google Chrome. Clonas el repositorio,
corres «estudio abrir», y el estudio queda en localhost, puerto ocho mil cien.

**Imagen:** terminal con `git clone …`, `./estudio abrir`, `http://localhost:8100`.

## 4. Preparar la lectura

**Narración:** En Preparar subes el PDF del libro, eliges el capítulo desde su
índice y marcas con un clic dónde empieza y dónde termina tu parte. El estudio
encuentra las figuras y las láminas, cuántas veces se nombran, y en qué
párrafos deben quedar fijas. Las citas textuales se agregan con un clic, y cada
personaje recibe su voz: con ElevenLabs, con un modelo local, o con un audio
tuyo. Al final eliges una música de fondo y guardas.

**Imagen:** capturas de Preparar: libro y páginas, figuras, citas y voces, música.

## 5. Grabar

**Narración:** En la consola cada párrafo es una toma. Eliges el párrafo,
pulsas R para grabar, lees, y S para parar. Si te equivocas, lo grabas otra
vez: la toma nueva reemplaza a la anterior. Cuando el párrafo tiene una cita,
pulsas N y suena por los audífonos con la voz del personaje.

**Imagen:** la consola de grabación, con las teclas R, S, N, Av Pág.

## 6. El render

**Narración:** Al renderizar, el estudio escucha cada toma y sabe qué línea
estás leyendo. El texto, compuesto de nuevo con la letra del libro, corre con
tu voz. Cuando nombras una figura, aparece sola con su pie justo cuando lo
lees, y al volver una flecha marca dónde sigues. Si la figura queda fija a la
derecha, el texto conserva su tamaño. Además recorta los silencios largos y
deja todas las tomas al mismo volumen.

**Imagen:** fragmentos de la lectura 22: el texto corriendo, una figura con su
pie, la flecha, el texto angosto junto a la figura.

## 7. La entrega

**Narración:** Al final tienes un video en mil ochenta pe, listo para YouTube,
con el título en el formato del curso, la descripción y la miniatura.

**Imagen:** la miniatura, el título `ME01 Lectura 22 …`, «1080p · 18:00».

## 8. Cierre

**Narración:** Videolectura Studio está en GitHub. Pruébalo con tu próxima
lectura, y si algo falla, abre un issue. No es una herramienta oficial de la
universidad: la hicimos para el curso.

**Imagen:** `github.com/mbuitragocUNAL/videolectura-studio-unal`, «Hecho por
estudiantes · no es una herramienta oficial de la UNAL».
