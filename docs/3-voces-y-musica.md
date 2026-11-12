# 3 · Voces y música

[Video de este paso](https://youtu.be/A0JK6u84_pw) · [← Preparar](2-preparar.md) · [Siguiente: grabar →](4-grabar.md)

## Las voces de las citas (lineamiento 2.i)

Cada cita textual se lee con **voz sintética**, y un personaje suena igual en
todas sus citas. Todo pasa en **Preparar › Citas con voz sintética**, después
de guardar la lectura.

### 1. Describe al personaje

Cada personaje tiene una caja «cómo suena»: edad, oficio, ánimo, acento. Lo que
importa es cómo suena, no quién es: «hombre de unos 60 años, voz grave y
pausada, tono irónico, español neutro» funciona mejor que «Sócrates».

### 2. Elige el motor

| Motor | Cómo | Para tener en cuenta |
|---|---|---|
| **ElevenLabs** | Pulsa **Agregar clave** y pega tu clave de API. Se guarda solo en `voces/claves.json`, en tu computador; la consola no la vuelve a mostrar. Arriba se ve cuántos caracteres te quedan del plan. | El plan gratuito trae 10 000 caracteres al mes; una lectura usa unos pocos cientos. |
| **Qwen3-TTS (local)** | Libre, sin cuenta ni red después de bajar los modelos (~4 GB cada uno, la primera vez). | En el contenedor corre en CPU y es lento. En un Mac con Apple Silicon, córrelo desde la terminal (abajo). |
| **Audio propio** | El ícono de subir en cada cita: un clip grabado o generado en otra parte. | Se deja a la misma sonoridad que los demás. |

### 3. Diseña o elige la voz

- **Diseñar** hace **tres voces candidatas** a partir de la descripción, diciendo
  las propias líneas del personaje. Escúchalas y pulsa **Usar esta**. En
  ElevenLabs queda en la biblioteca de tu cuenta; con Qwen, en `voces/qwen/`.
  Volver a pulsar Diseñar da tres nuevas.
- Con ElevenLabs también puedes elegir **cualquier voz que ya esté en tu cuenta**
  en la lista de al lado.

### 4. Genera

**Generar N clips** hace de una vez todos los que faltan, a −16 LUFS, la misma
sonoridad que tendrá tu voz en el video. Cada cita muestra su estado:

- 🟢 **lista**: se puede escuchar con ▶;
- ⚪ **falta**;
- 🟡 **la cita cambió** u **otra voz**: el texto o la voz del personaje cambiaron
  desde que se hizo; el botón la vuelve a hacer. Las que siguen bien no se
  repiten (ni se vuelven a pagar).

↻ rehace una sola cita.

### Qwen en un Mac, desde la terminal

```bash
python3 -m venv preparar/.venv
preparar/.venv/bin/python -m pip install -r preparar/requirements.txt -r preparar/requirements-voz.txt
./estudio voces disenar lecturas/22-… --personaje "Electrón" --motor qwen   # tres candidatas
./estudio voces elegir  lecturas/22-… --personaje "Electrón" --candidato 2  # la guarda en lectura.toml
./estudio voces generar lecturas/22-…                                       # todos los clips
```

`./estudio voces estado lecturas/22-…` lista cada clip y si está listo. La
consola ve los clips en cuanto se recarga.

## La música de fondo (lineamiento 2.j)

Opcional, en **Preparar › Música de fondo**:

1. Elige una pista de la lista o súbela con el ícono de subir. Las pistas quedan
   en `musica/`, fuera de git, y sirven para todas tus lecturas.
2. **Bajo la voz**: cuántos dB por debajo de tu voz suena. 14 dB (por defecto)
   se oye detrás de la voz sin taparla; más la aleja, menos la acerca.
3. **Bajarla un poco más mientras se lee**: unos 3 dB menos cuando hablas, y
   vuelve a subir en las pausas.

El render la repite hasta el final con un fundido entre vueltas, y la hace
entrar y salir suavemente.

**Licencia:** usa música que puedas poner en YouTube. Dos fuentes que sirven:
[incompetech.com](https://incompetech.com) (Kevin MacLeod, CC BY 4.0: el crédito
va en la descripción) y la [música de Pixabay](https://pixabay.com/music/) (sin
atribución; prefiere pistas sin «Content ID Registered» para evitar reclamos automáticos).
