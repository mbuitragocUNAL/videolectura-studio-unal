# Manual técnico · Videolectura Studio UNAL

Las decisiones de diseño, con su porqué. Para empezar a usarlo, ver el
[README](../README.md).

Un estudio local para grabar video-lecturas académicas: teleprompter, ventana de
presentación que se graba a sí misma, montaje por tomas y render con ffmpeg.

Un binario de Go sirve la API y la interfaz; la interfaz es Nuxt 4 compilada
dentro de la imagen. En producción **no corre Node**.

```bash
./estudio abrir lecturas/22-belleza-cuantica-ii-exuberancia
# → http://localhost:8100
```

Se publica solo en `127.0.0.1` a propósito: `getUserMedia` y `getDisplayMedia`
exigen contexto seguro, y `localhost` lo es. Llegar desde otra máquina por HTTP
plano rompería la grabación en silencio.

---

## Las cuatro superficies

Son **zooms sobre una misma partitura**, no pantallas distintas. El eje es la
posición en la lectura (0..1 de la tira), no el tiempo, porque es la única
coordenada en la que coinciden líneas, citas, secciones y tomas.

| Scope | Para qué |
|---|---|
| **Lectura** | Teleprompter y transporte. Es la superficie que se usa hablando. |
| **Montaje** | Las tomas como material: recortar, nombrar, descartar. |
| **Render** | Lanza ffmpeg y sigue su progreso. |
| **`/presentador`** | La ventana que se graba. Sin una sola pieza de interfaz. |
| **`/lecturas`**, **`/preparar`** | Elegir, crear y preparar lecturas mirando el libro. |

## Cómo se graba

**Una toma es un párrafo.** La página se queda quieta mientras lo lees, y el
montaje empalma las tomas después **en el orden del pasaje, no en el que las
grabaste**: si repites un párrafo, la toma nueva ocupa su sitio sola y solo
tienes que borrar la mala. Acompasar la voz a una tira en movimiento es
lo que hacía esto difícil: obligaba a acertar a la primera durante media hora
seguida, y cualquier tropiezo desalineaba todo lo que venía detrás.

1. «Abrir presentación» y acomodar esa ventana **maximizada**: se graba con la
   forma que tenga, y la entrega es 1080p 16:9. Ponte los audífonos: las citas
   suenan por ellos y el micrófono no las capta, así que quedan como silencio
   limpio del largo exacto.
2. La consola lista **los 79 párrafos** con una marca en los que ya están
   grabados. Eliges uno —con `Av Pág` / `Re Pág`, o pulsando en él— y la página
   se coloca sola. El elegido va iluminado: eso es lo que lees, y donde paras.
3. `R`, o el botón de grabar de ese párrafo. Lees, `S`. La presentación captura
   su propia pestaña, así que no hay nada que recortar y el escritorio no puede
   colarse.
4. `Av Pág` para el siguiente, y otra vez. Cada párrafo dura unos veinte
   segundos, así que repetir uno cuesta veinte segundos.

**Un párrafo, una toma.** Si lo lees otra vez, la toma nueva sustituye a la
anterior: el vídeo no dice la frase dos veces. La vieja se queda en el disco
—el contador dice cuántas hay y la papelera del párrafo borra la última, así
que se puede volver atrás— y el montaje ofrece borrarlas todas de golpe. Una
toma recortada hasta no dejar nada tampoco entra: eso no es una toma muda, es
ninguna toma.

El párrafo elegido se guarda **con la toma**, no se deduce del scroll. Arriba
del todo la página ya no puede subir más, así que la posición se queda a cuatro
décimas de pantalla del borde y los dos primeros párrafos quedaban detrás:
imposibles de elegir.

`N` suena **la cita de ese párrafo**, y solo esa. Un párrafo es un turno, así
que la cita que suena es la del turno que elegiste, y volver a pulsar la vuelve
a sonar: no hay forma de perderla de vista. (Antes encadenaba el diálogo
entero, que tenía sentido cuando una toma cubría toda la conversación; con una
toma por párrafo metía cuatro clips en una grabación que quería uno, y dejaba
la cadena pasada del turno que buscabas.)

**Las citas van por los audífonos, no por los altavoces.** La grabación captura
únicamente el micrófono —la pantalla se captura con `audio: false`— y el clip
no se graba nunca: el render lo mezcla desde el mp3 en el instante en que
pulsaste `N`. Por los altavoces el micrófono recogería una segunda copia con
eco encima de la limpia.

**La página no se mueve sola nunca.** El scroll continuo sigue ahí —`espacio`
lo arranca a mano— pero nada lo enciende por su cuenta: antes la cita
arrastraba el scroll con su audio, lo que tenía sentido cuando una toma cubría
un diálogo entero y, con una toma por párrafo, movía justo la frase que se
estaba diciendo.

En la consola, el panel **Citas** lista las diecinueve con su personaje, su
párrafo y si ya están grabadas. Pulsar una lleva al párrafo donde vive, que es
lo mismo que elegirla: `N` suena la del párrafo en el que estés.

Cada toma se sube sola con sus citas, su envolvente y la traza del scroll.

| Tecla | |
|---|---|
| `R` / `S` | grabar / detener |
| `N` | sonar la cita de este párrafo |
| `P` | volver a sonarla |
| `Esc` | cortarla |
| `Av Pág` / `Re Pág` | el siguiente párrafo / el anterior |
| `espacio` | correr / pausar el scroll |
| `↑` `↓` | velocidad ±1 px/s |
| `←` `→` | una línea atrás / adelante (con `shift`, diez) |
| | La línea que lees va al 40% de la pantalla, no arriba del todo. |
| `L` / `A` | lámina / flecha |

La regla de arriba de la partitura es una barra de arrastre: al pasar por
encima dice en qué línea caería el cursor, y arrastrando lo dejas ahí. Es la
forma de colocar el inicio de una toma sin tantear con el scroll.

La partitura **hace zoom**: la rueda acerca sobre el puntero, `shift` y
arrastrar la desplaza, y «ver todo» vuelve a la tira entera. Con setenta y
nueve tomas en un carril de mil píxeles, cada una mide trece: hace falta poder
acercarse para señalar una.

## Cómo se monta

El montaje edita **una toma a la vez** y se apoya en tres cosas que el estudio
ya sabe, en vez de pedirte que las adivines mirando una onda.

**Se ve entero.** Arriba del scope está el montaje completo —una toma por
bloque, en el orden del pasaje— y se reproduce de corrido saltando los cortes.
Hay un salto breve al cambiar de toma mientras se abre el archivo siguiente; el
render las empalma de verdad. Pulsando un bloque se edita esa toma.

**Se oye.** El reproductor salta lo cortado, así que lo que suena es el montaje
y no la grabación. Un empalme se juzga oyéndolo; el render es una forma
demasiado lenta de preguntar.

**Se acerca.** La ventana llega hasta un cuarto de segundo. Media hora repartida
en mil píxeles son dos segundos por píxel, y a dos segundos por píxel no se
recorta un tropiezo. Arriba queda la toma entera con la ventana marcada.

**Se lee.** La traza nombra la línea del pasaje bajo el cursor, así que un corte
es «de la línea 67 a la 71» y no un par de timestamps que nadie puede
comprobar. Debajo del vídeo va esa línea con la anterior y la siguiente.

### Propuestas

La toma ya sabe dónde salió mal, así que el estudio lo propone —nunca lo
aplica— y tú lo aceptas, lo ajustas o lo ignoras:

- **Relecturas.** Cuando te equivocas, empujas la tira hacia atrás y repites la
  frase. Es lo único que la traza hace que no sea monótono: un escalón hacia
  atrás. Entre el momento en que la tira pasó por ahí la primera vez y el
  momento del salto está la pasada mala, que es justo lo que sobra.
- **Silencios.** Tramos por debajo del ruido de fondo durante más de 1,2 s, con
  un cuarto de segundo de aire a cada lado para que el empalme no corte la
  respiración. **Una cita nunca se propone**: suena por los audífonos, así que
  en el micrófono es silencio deliberado del largo exacto, y cortarlo borraría
  el hueco donde la cita tiene que entrar.

Un corte que se tragaría una cita se rechaza, porque esa cita no tiene otro
sitio donde vivir — salvo que la misma cita vuelva a sonar después, que es
exactamente lo que pasa cuando leíste el pasaje dos veces.

| Tecla | |
|---|---|
| `espacio` | reproducir / pausar |
| `←` `→` | mover el cursor 1 s (con `shift`, 0,1 s) |
| `I` / `O` | marcar entrada y salida |
| `X` | cortar la selección |
| `Z` | deshacer el último corte |
| `E` / `F` | acercar a la selección / ver la toma entera |
| `+` `-` | acercar y alejar |
| `Tab` / `Enter` | recorrer las propuestas / aceptar la que está marcada |

La rueda acerca sobre el puntero; arrastrar el borde de un corte lo mueve, y la
`×` sobre él lo quita.

## Figuras y láminas

El lineamiento 3.2.3 trata distinto la primera vez que el autor nombra una
figura o lámina y las siguientes, y `lector.py` marca cada mención en la frase
donde ocurre:

- **Primera mención (2.d):** la figura entera, con su pie y nada más, y el pie
  se lee en voz alta. Es una toma propia: en la lista de párrafos aparece una
  fila con el nombre de la figura justo después del párrafo que la nombra, con
  su botón de grabar. `L` la muestra en la presentación; se lee el pie y se
  para. Si no se graba, el render la deja igual tres segundos en silencio, para
  que no falte.
- **Las siguientes (2.e):** la figura otra vez, **sin pie**, dos o tres segundos
  (`segundos_figura` en el toml). No se graba nada: la lista lo anota y el render
  la pone sola.
- **Al volver**, la página que sigue lleva la flecha de retorno en el párrafo
  donde se retoma la lectura. La pone el render, después de cualquier figura.

Una toma de figura ocupa un sitio **al lado** del párrafo, no el mismo: grabarla
no sustituye la lectura del párrafo. Un párrafo que nombra dos figuras tiene
una fila por cada una. El render las muestra completas sobre blanco en vez de
recortarlas al cuadro, porque no son 16:9.

Los pies impresos en la página no son párrafos para grabar: se leen cuando la
figura se muestra sola, así que la lista no los incluye.

La lectura 07 no trae menciones en su `marcas.json`, solo la lámina F, y sigue
funcionando como antes.

## La imagen la dibuja el render, no la grabación

El vídeo de cada toma **no se usa**. El navegador solo entrega un fotograma
cuando la pestaña cambia, y la página se queda quieta a propósito mientras se
lee un párrafo: las capturas salían a **un fotograma por segundo**. Veintiún
minutos de voz llegaban con catorce de imagen, y las dos se separaban desde el
primer empalme.

El render dibuja el texto él mismo, y lo hace correr con la voz.

**El pasaje se compone de nuevo** (`preparar/componer.py`, al preparar). El
profesor rechazó una lectura de este mismo capítulo por dos cosas que salían de
mostrar las páginas tal cual: márgenes enormes, y el texto más pequeño cada vez
que una figura fija le quitaba espacio («si hay menos espacio para letras
iguales, saldrán más líneas de texto»). Así que el pasaje se vuelve a componer
con las fuentes del propio libro (extraídas del PDF), a su mismo tamaño, en dos
columnas: una ancha que ocupa el 94 % de la pantalla y una angosta para cuando
hay una figura a la derecha. La letra es la misma en las dos; la angosta solo
tiene más líneas. Conserva cursivas, negritas, subíndices (C₆₀), sangrías,
listas, títulos y líneas centradas, y parte las palabras en sílabas para que la
columna angosta no quede con huecos. Son dos tiras continuas: no hay corte de
página.

**La tira corre con la lectura.** El render sabe, por la transcripción de
Whisper, cuándo empieza cada línea compuesta, y desliza la tira para que la
línea que se está leyendo quede siempre a la misma altura (38 % desde arriba).
Un párrafo sigue al otro sin corte; la columna solo cambia (de ancha a angosta)
en un corte que ya existía: la figura con su pie, o una toma nueva. La flecha de
regreso (2.d) va pegada a su línea y corre con ella. Si una lectura no tiene
`tipografia.json` (la 07), el render vuelve a mostrar las páginas.

**Imagen y sonido juntos.** Cada pedazo dura un número entero de fotogramas y
lleva el audio en PCM hasta el empalme final, donde se codifica a AAC una sola
vez. Antes, cada pedazo terminaba su imagen un poco después de su sonido; a lo
largo de setenta pedazos la imagen se atrasaba cinco segundos, y las figuras
aparecían cuando ya se estaba leyendo su pie.

De la grabación se usa solo el micrófono. Por eso las citas van por los
audífonos: el clip no se graba nunca, lo mezcla ffmpeg desde el mp3.

**Las pausas.** Una toma se graba entre dos teclas, así que trae un segundo
de silencio antes de la primera palabra y otro después de la última; puestas
una tras otra, cada cambio de párrafo quedaba en dos segundos de nada. El
render deja 0,18 s antes de la primera palabra y 0,32 s después de la última
(medio segundo entre párrafos), y acorta a 0,7 s cualquier pausa dentro de una
toma que pase de 0,9 s. Mide la voz en el audio mismo, así que un clic de
teclado no cuenta como lectura. Nunca toca el silencio donde suena una cita.
Son cortes del render, no de la toma: la consola sigue mostrando solo los
cortes propios. `-pausa 0` los apaga; `-pausa 1.2` deja pausas más largas.

**La música de fondo (2.j)**, opcional. Las pistas van en `musica/`
(compartida por todas las lecturas, fuera de git: la música es de quien la
descargó) y la lectura elige una en **Preparar › Música de fondo**, que la
escribe en `[musica]` de `lectura.toml`. El render la pone bajo todo el video:
repetida hasta el final, con fundido de 2 s al entrar y de 4 s al salir, a
`nivel` dB por debajo de la voz (14 por defecto: se oye detrás de la voz sin
taparla; con 20 no se oía en los parlantes de un portátil) y, con `atenuar`,
unos 3 dB más abajo mientras se lee.

## La traza del scroll

Cada toma guarda muestras `(t, f)` a 20 Hz: dónde estaba la tira en cada
instante. Las citas fijan la correspondencia tiempo↔posición en diecinueve
puntos; la traza la fija en todos. Es lo que permite que el montaje diga **«este
corte quita de la línea 67 a la 71»** en vez de estimar un timestamp, y lo que
hace exacta la comprobación de que un corte no se traga una cita.

## Levantarlo

```bash
./estudio abrir lecturas/NN-titulo   # → http://localhost:8100
```

`abrir` monta esa carpeta como la lectura y su `estudio/` como el lugar de las
tomas. `docker compose up -d` a secas abre la lectura 07.

Eso es todo lo que hace falta para grabar: un contenedor con el binario, la
interfaz compilada y ffmpeg. Tarda unos segundos en pasar a `healthy`.

```bash
docker compose ps                # estado
docker compose logs -f studio    # qué está haciendo
./estudio cerrar                 # bajarlo (las tomas quedan en la carpeta de la lectura)
```

### Trabajar en la interfaz

```bash
docker compose --profile dev up -d   # → http://localhost:3300 con recarga
```

Levanta un segundo contenedor con el servidor de Nuxt montando `frontend/` del
host. Manda `/api` y `/assets` al contenedor del estudio por la red de Docker,
así que los dos tienen que estar arriba. Los `node_modules` viven en un volumen
para que los binarios de Linux del contenedor no pisen los de macOS.

Cuando termines, `docker compose build studio` hornea la interfaz dentro de la
imagen y `:8100` queda actualizado.

```bash
go test ./...                    # lógica de cortes, grafo de ffmpeg, traza
cd frontend && pnpm typecheck && pnpm test
```

`pnpm-workspace.yaml` declara `allowBuilds` para esbuild y vue-demi: sin eso
pnpm 11 aborta la instalación y Vite no arranca.

## Preparar otra lectura

```bash
./estudio abrir          # → http://localhost:8100/lecturas
```

En **Lecturas** (el ícono de biblioteca en la cabecera de la consola) están
todas las de `lecturas/`, con sus tomas. Desde ahí se graba una, se vuelve a
preparar o se crea otra. **Nueva lectura** abre la página de preparación, que
se hace mirando el libro:

1. **Libro.** Uno del estante o «Subir un PDF». Quedan en `libros/`, fuera de
   git: el libro no se comparte, cada quien tiene su copia.
2. **Páginas.** El índice del PDF lista los capítulos con sus páginas; un clic
   elige el capítulo y su título. También se pueden escribir (`61-72, 446`).
3. **Dónde empieza y termina.** Las páginas se ven a la derecha con las líneas
   que encontró el análisis encima. Clic en una línea: «la lectura empieza
   aquí» o «termina aquí». Las secciones tienen atajos para empezar en una o
   terminar antes de otra. Lo que queda fuera se ve lavado.
4. **Figuras y láminas.** Cada una con su recorte, su página y cuántas veces se
   nombra. Se puede quitar, corregir la página donde está, desactivar que quede
   fija donde la nombran, o fijarla en otro párrafo (clic en «Fijar en otro
   párrafo» y luego en el párrafo, o desde el menú de la línea). Eso último es
   para los párrafos que hablan de la figura sin nombrarla.
5. **Citas.** Las sugerencias son frases entre comillas angulares; en la vista
   «Texto» se selecciona cualquier fragmento y se agrega. Cada cita lleva su
   personaje, y cada personaje su color y la descripción de su voz.
6. **Datos del video.** Curso, grupo, número y título, con el título de YouTube
   como queda (2.k) y la duración estimada contra los 5 a 25 minutos.

Cada cambio se vuelve a analizar en uno o dos segundos y se dibuja sobre las
páginas antes de escribir nada. **Guardar y preparar** escribe el
`lectura.toml` y prepara la carpeta; **Grabar** la abre en la consola.

**Un párrafo que sigue en la página siguiente es un solo párrafo**, y una
sola toma: la primera línea de la página nueva continúa el anterior si la
página terminó con una línea llena y ella no tiene sangría (ni respecto al
margen ni respecto a la línea anterior, que es lo que importa en las listas).
Si volver a preparar cambia dónde se parten los párrafos, el estudio avisa al
arrancar qué tomas quedaron apuntando a otro texto.

El `lectura.toml` sigue siendo la fuente: lo que se elige con clics se guarda
como texto del libro (`desde`, `hasta`, `[[fija]]`), así que vale aunque se
cambie el rango de páginas, y se puede editar a mano y volver a preparar.

Por línea de comandos también se puede:

```bash
./estudio nueva --libro ~/Downloads/LIBRO.pdf --paginas 207-218 \
                --numero 22 --titulo "Belleza cuántica II: exuberancia"
./estudio preparar lecturas/22-belleza-cuantica-ii-exuberancia
```

Lo que deja en la carpeta:

| | |
|---|---|
| `lectura.toml` | lo único que va a git |
| `paginas/`, `tira.json` | las páginas recortadas al texto, con un hueco fijo entre una y otra |
| `marcas.json` | líneas con su posición exacta, párrafos, secciones, citas, figuras |
| `figuras/` | cada figura y lámina que el texto nombra, con su pie (`-pie.png`) y sin él |
| `audio/tts/` | el texto de cada cita, su clip de voz con el registro de cómo se hizo, y `VOCES.md` |
| `guion.txt` | el pasaje numerado, con dónde va cada cita y cada figura, para ensayar |
| `estudio/` | las tomas, la sesión y el render de esta lectura |

Las voces de las citas se hacen en el mismo paso 5; ver más abajo.

La lectura 07 se preparó con los guiones viejos de
`../lecturas/lectura-07/scripts/` y sigue abriéndose igual: es lo que monta
`./estudio abrir` sin carpeta.

### Las voces de las citas (2.i)

Cada cita textual va con voz sintética, y un personaje suena igual en todas
sus citas. En **Preparar › Citas**, después de guardar la lectura, cada
personaje tiene su descripción («cómo suena: edad, oficio, ánimo, acento») y
uno de tres motores:

- **ElevenLabs**, con la clave de quien graba. Se agrega en el mismo panel y
  queda en `voces/claves.json` de este computador (fuera de git, solo legible
  por su dueño); la consola nunca la vuelve a mostrar, solo sus cuatro últimos
  caracteres y cuántos caracteres del plan quedan. También vale la variable
  `ELEVENLABS_API_KEY`.
- **Qwen3-TTS**, en el computador: libre (Apache 2.0), sin cuenta y sin red
  después de bajar los modelos (unos 4 GB cada uno, la primera vez).
- **Audio propio**: un clip grabado o generado en otra parte se sube con el
  ícono de cada cita.

**Diseñar** hace tres voces candidatas a partir de la descripción, diciendo
las propias líneas del personaje. Se escuchan y «Usar esta» la guarda: en
ElevenLabs queda en la biblioteca de la cuenta, con Qwen en `voces/qwen/`.
Las dos sirven para otras lecturas en las que aparezca el mismo personaje; con
ElevenLabs también se puede elegir cualquier voz que ya esté en la cuenta.
Con Qwen la voz diseñada es una muestra de referencia que el modelo base clona
en cada cita, y eso es lo que la mantiene igual de una cita a otra.

**Generar** hace de una vez todos los clips que faltan, a −16 LUFS, la misma
sonoridad a la que el render deja la narración. Cada clip guarda al lado
(`NN-personaje.json`) el texto y la voz con que se hizo: si la cita cambia o
el personaje cambia de voz, el clip queda marcado y el botón lo vuelve a
hacer; los que siguen bien no se repiten (ni se vuelven a pagar). Al terminar
se prepara la lectura otra vez para que el estudio lea las duraciones.

Qwen en el contenedor corre en CPU y es lento; para tenerlo ahí,
`VOZ_LOCAL=1 docker compose build studio`. En un Mac es mejor correrlo afuera,
donde usa la GPU:

```bash
preparar/.venv/bin/python -m pip install -r preparar/requirements-voz.txt
./estudio voces disenar lecturas/22-… --personaje "Electrón" --motor qwen
./estudio voces elegir  lecturas/22-… --personaje "Electrón" --candidato 2
./estudio voces generar lecturas/22-…
```

`elegir` escribe la voz en `[voces]` de `lectura.toml`, y la consola ve los
clips en cuanto se recarga. `./estudio voces estado` lista cada clip y si está
listo; `./estudio voces subir … --clip 03-glaucon audio.wav` usa uno propio.

### Para subirla

Junto a `estudio/output/final.mp4` van `descripcion-youtube.txt` y
`miniatura.png`. La miniatura sale de `preparar/miniatura.py`, con la cara del
capítulo 18 (fuentes IBM Plex y escudo en `preparar/marca/`):

```bash
preparar/.venv/bin/python preparar/miniatura.py lecturas/22-… --titulo "Belleza cuántica II" \
    --destacado "exuberancia" --linea "Diamante · grafeno · nanotubos · buckyballs" \
    --figura figuras/figura-30.png
```

En YouTube (4.3): no es contenido para niños, formato estándar (no Short),
1080p. Si la lectura lleva música, su crédito CC BY va en la descripción.

### La figura fija a la derecha (2.f)

Mientras se leen los párrafos que hablan de una figura, el video la deja a la
derecha, sin pie, con el texto a la izquierda en la columna angosta, a la misma
letra que en el resto del video (ver «El pasaje se compone de nuevo»). La parte
de la pantalla para la figura la da `ancho_figura` (0,38 por defecto).

La ventana de presentación no la muestra mientras se graba: la dibuja el render.
En la lista de párrafos, una etiqueta «Figura N fija» dice en cuáles va a estar.

Los párrafos que hablan de una figura sin nombrarla («A la izquierda
tenemos…») se fijan a mano, en la página de preparación o con `[[fija]]` en
`lectura.toml`. Si un párrafo pasa de una figura a otra, la primera se ve desde
el comienzo y la siguiente entra en la frase que la nombra.

En el texto compuesto las figuras no están impresas entre párrafos: se ven
solas con su pie (2.d), sin pie un par de segundos (2.e), o fijas a la derecha
(2.f). Con las páginas del libro (lectura 07), una figura fija que también está
impresa en la página se tapa ahí mientras está a la derecha (`caja` en
`marcas.json`).

**El pie y lo que dice la imagen (2.d).** Cuando la figura trae texto dentro
(la Lámina FF: «(n,0) zigzag», «(10,10) nanotube (armchair)»…), también hay que
leerlo, en la misma toma, después del pie.

### El pie leído dentro del párrafo (2.d)

No hace falta grabar el pie aparte: se puede leer dentro de la toma del
párrafo, justo después de nombrar la figura. Al renderizar, el estudio pasa
por Whisper todas las tomas (con el texto compuesto, que corre línea por línea;
con las páginas, solo las que tienen una figura en medio) (`preparar/alinear.py`,
modelo `STUDIO_WHISPER_MODEL`, «small» por defecto; como contexto solo le da
los nombres y términos raros del texto, nunca el texto mismo, porque Whisper
se salta lo que cree haber oído ya), empareja las palabras que
oye con el texto, pie incluido, y corta en la pausa justo antes de la palabra
donde cambia la imagen. La transcripción se guarda en `estudio/alineacion/` y
se hace una sola vez por toma; la primera vez también se descarga el modelo.
Si Whisper no está o no entiende la toma, el corte se estima por la
proporción del texto y la pausa más cercana. Mientras se lee el pie, el video muestra la figura a pantalla
completa con su pie; después la deja `segundos_figura` en silencio y vuelve a
la página con la flecha en la línea donde sigue la lectura. Las menciones
siguientes (2.e) se muestran al terminar la frase que las nombra. Una figura
que también queda fija a la derecha solo aparece ahí después de su pie.

Si el pie se graba aparte (tecla **L**), esa toma va después del párrafo, como
antes.

## Estructura

```
studio/
├── cmd/studio/           arranque del servidor
├── internal/
│   ├── project/          lectura, sesión, cortes, traza  (+ tests)
│   ├── render/           grafo de ffmpeg y ejecutor      (+ tests)
│   └── api/              HTTP
├── frontend/             Nuxt 4 + Nuxt UI 4 + Tailwind 4
│   └── test/             aritmética del montaje, con el runner de node
├── preparar/             lector.py: del PDF y lectura.toml a los insumos;
│                         el estudio lo llama para la página de preparación.
│                         alinear.py (Whisper) y voz.py (voces de las citas)
├── lecturas/             una carpeta por lectura (en git solo su lectura.toml)
├── libros/               los PDF, fuera de git
├── voces/                voces diseñadas y la clave de ElevenLabs, fuera de git
├── musica/               pistas de fondo (2.j), fuera de git
├── design/               las seis direcciones exploradas
├── estudio               atajo: nueva, preparar, abrir, cerrar, voces
├── Dockerfile            Node → Go → alpine con ffmpeg
├── Dockerfile.prep       Python con PyMuPDF, para preparar sin instalar nada
└── docker-compose.yml
```
