/**
 * The book, in the browser.
 *
 * Preparing a reading means looking at the book: which chapter, which pages,
 * where the reading starts and stops, what each figure looks like. pdf.js
 * draws all of that here, from the PDF the studio serves, so choosing never
 * waits on the server. The analysis — lines, paragraphs, figures — still comes
 * from preparar/lector.py, in PDF points; pdf.js draws in the same points, so
 * the two line up without any conversion beyond a scale.
 */
import * as pdfjs from 'pdfjs-dist'
import type { PDFDocumentProxy } from 'pdfjs-dist'
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

const documents = new Map<string, Promise<PDFDocumentProxy>>()

export function loadPdf(url: string) {
  let doc = documents.get(url)
  if (!doc) {
    doc = pdfjs.getDocument({ url }).promise
    documents.set(url, doc)
    doc.catch(() => documents.delete(url))
  }
  return doc
}

export interface Chapter {
  titulo: string
  nivel: number
  /** First page, 1-based. */
  pagina: number
  /** Last page: the page before the next entry at the same level or above. */
  hasta: number
}

/** The book's table of contents, flattened, with each entry's page range. */
export async function chaptersOf(doc: PDFDocumentProxy): Promise<Chapter[]> {
  const outline = (await doc.getOutline()) ?? []
  const flat: Omit<Chapter, 'hasta'>[] = []
  async function walk(items: typeof outline, nivel: number) {
    for (const item of items) {
      let pagina = 0
      try {
        const dest = typeof item.dest === 'string' ? await doc.getDestination(item.dest) : item.dest
        const ref = dest?.[0]
        if (ref && typeof ref === 'object') pagina = (await doc.getPageIndex(ref)) + 1
        else if (typeof ref === 'number') pagina = ref + 1
      } catch { /* an entry without a destination is a label, not a chapter */ }
      if (pagina > 0) flat.push({ titulo: item.title.trim(), nivel, pagina })
      if (item.items?.length) await walk(item.items, nivel + 1)
    }
  }
  await walk(outline, 1)
  return flat.map((c, i) => {
    const next = flat.slice(i + 1).find(n => n.nivel <= c.nivel && n.pagina > c.pagina)
    return { ...c, hasta: next ? next.pagina - 1 : doc.numPages }
  })
}

/** Draw a page into a canvas `width` CSS pixels wide; returns points → pixels. */
export async function drawPage(doc: PDFDocumentProxy, n: number, canvas: HTMLCanvasElement, width: number) {
  const page = await doc.getPage(n)
  const base = page.getViewport({ scale: 1 })
  const scale = width / base.width
  const ratio = window.devicePixelRatio || 1
  const viewport = page.getViewport({ scale: scale * ratio })
  canvas.width = viewport.width
  canvas.height = viewport.height
  canvas.style.width = `${width}px`
  canvas.style.height = `${base.height * scale}px`
  await page.render({ canvasContext: canvas.getContext('2d')!, viewport }).promise
  return { scale, height: base.height * scale }
}

/** A region of a page, [x0, y0, x1, y1] in points, as an image. */
export async function cropOf(doc: PDFDocumentProxy, n: number, box: number[], width = 360) {
  const [x0 = 0, y0 = 0, x1 = 1, y1 = 1] = box
  const page = await doc.getPage(n)
  const scale = Math.min(4, width / Math.max(1, x1 - x0))
  const viewport = page.getViewport({ scale })
  const full = document.createElement('canvas')
  full.width = viewport.width
  full.height = viewport.height
  await page.render({ canvasContext: full.getContext('2d')!, viewport }).promise
  const out = document.createElement('canvas')
  out.width = Math.max(1, Math.round((x1 - x0) * scale))
  out.height = Math.max(1, Math.round((y1 - y0) * scale))
  out.getContext('2d')!.drawImage(full, x0 * scale, y0 * scale, out.width, out.height, 0, 0, out.width, out.height)
  return out.toDataURL('image/png')
}
