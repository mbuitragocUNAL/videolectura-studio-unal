# 5 · Montar y renderizar

[Video de este paso](https://youtu.be/7YBjXaY3VPY) · [← Grabar](4-grabar.md) · [Siguiente: entregar →](6-entregar.md)

## El montaje (opcional)

En la consola, **Montaje** edita una toma a la vez. Casi nunca hace falta: el
render ya recorta los silencios. Sirve para quitar un tropiezo sin volver a
grabar el párrafo.

- **Se ve entero:** arriba, el montaje completo, una toma por bloque, en el
  orden del pasaje. Se reproduce de corrido saltando lo cortado. Un clic en un
  bloque edita esa toma.
- **Se oye:** el reproductor salta lo cortado, así que lo que suena es lo que quedará.
- **Propuestas:** el estudio marca dónde probablemente sobra algo: las
  **relecturas** (cuando volviste atrás para repetir una frase) y los
  **silencios** largos. Nunca las aplica solo: `Tab` las recorre, `Enter` acepta la marcada.

| Tecla | |
|---|---|
| `espacio` | reproducir / pausar |
| `←` `→` | mover el cursor 1 s (con `shift`, 0,1 s) |
| `I` / `O` | marcar entrada y salida |
| `X` | cortar la selección |
| `Z` | deshacer el último corte |
| `E` / `F` | acercar a la selección / ver la toma entera |
| `Tab` / `Enter` | recorrer las propuestas / aceptar la marcada |

Un corte que se tragaría una cita se rechaza: ese silencio es donde suena la cita.

## El render

En **Render**, el resumen dice cuántas tomas hay y cuánto dura el montaje,
con un aviso si se sale de los 5–25 minutos. **Montar el video** lo arranca; la
barra muestra en qué va, y la consola sigue usable mientras tanto.

Lo que hace, en orden:

1. **Escucha cada toma** con Whisper (la primera vez descarga el modelo, ~500 MB; después cada toma se transcribe una sola vez).
2. **Recorta las pausas**: medio segundo entre párrafos, y ninguna pausa dentro de una toma de más de 0,7 s.
3. **Iguala el volumen** de todas las tomas (−16 LUFS).
4. **Compone la imagen**: el texto corre con tu voz, la línea que lees a la misma altura; cada figura aparece con su pie al leerlo, se queda 2 s y vuelve con flecha; la figura fija a la derecha con el texto angosto al mismo tamaño.
5. **Mezcla** las citas en el instante en que pulsaste `N`, y la música de fondo.

Queda en `lecturas/<tu-lectura>/estudio/output/final.mp4`, en 1080p. Una
lectura de 18 minutos tarda unos 3 a 5 minutos (más la primera vez, por las
transcripciones).

## Revisa el video antes de entregar

- [ ] Cada figura aparece **mientras** lees su pie, no después.
- [ ] Al volver de una figura, la flecha señala la línea donde sigues.
- [ ] Las figuras con texto dentro: lo leíste después del pie.
- [ ] Las citas suenan con su voz, en su lugar, y no se cortan.
- [ ] No hay frases cortadas ni repetidas entre una toma y la siguiente.
- [ ] La música se oye detrás de tu voz, sin taparla.
- [ ] Dura entre 5 y 25 minutos.

Si algo falla, corrige y vuelve a renderizar: lo que ya se transcribió no se
vuelve a transcribir. Si una figura aparece tarde o temprano, revisa que la toma
se oiga bien desde el primer segundo; si un párrafo suena mal, grábalo otra vez.
