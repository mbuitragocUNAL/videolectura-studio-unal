# 6 · Entregar

[Video de este paso](https://youtu.be/j1wxLYfk07M) · [← Montar y renderizar](5-montar-y-renderizar.md) · [Volver al inicio](../README.md)

## Subir a YouTube (sección 4.3 de los lineamientos)

| | |
|---|---|
| **Título** | `ME0Z Lectura XX Título de la sección`: el mismo que muestra Preparar › Datos del video (2.k). Ej.: `ME01 Lectura 22 Belleza cuántica II: exuberancia`. |
| **Audiencia** | **No, no es contenido para niños.** Si lo es, el profesor no puede agregarlo a las listas del curso. |
| **Formato** | Video estándar, no *Short*. |
| **Calidad** | 1080p: `final.mp4` ya lo es. |
| **Visibilidad** | La que pida el profesor (normalmente público o no listado). |

## La descripción

Conviene incluir: el libro, la sección y las páginas; los créditos de la voz
sintética (2.i: qué herramienta) y de la música, con la atribución exacta que pida
su licencia (CC BY: título, autor, fuente y licencia).

## La miniatura

Opcional, con la misma cara para todas las lecturas:

```bash
preparar/.venv/bin/python preparar/miniatura.py lecturas/22-… \
    --titulo "Belleza cuántica II" --destacado "exuberancia" \
    --linea "Diamante · grafeno · nanotubos · buckyballs" \
    --figura figuras/figura-30.png
```

Queda en `lecturas/22-…/estudio/output/miniatura.png` (1280×720). Para el
escudo de la universidad, ver `preparar/marca/README.md`.

## El correo al profesor (2.m)

Responde el correo **«Ganador y elegibles para la lectura…»**, **solo al
profesor** (no a todos), dentro de las fechas que da ese correo, con el enlace
de YouTube.

## Lista final

- [ ] Título en el formato del curso.
- [ ] No dirigido a niños, formato estándar, 1080p.
- [ ] Créditos de la voz sintética y de la música en la descripción.
- [ ] Revisaste el video completo ([lista del paso 5](5-montar-y-renderizar.md#revisa-el-video-antes-de-entregar)).
- [ ] El correo va solo al profesor y dentro del plazo.
