// Capturas de la consola real del estudio para la guía, a 1920×1080.
//
//   node guia/herramientas/capturas.mjs [http://localhost:8100]
//
// Escribe guia/video/assets/capturas/*.png. El estudio tiene que estar abierto
// (./estudio abrir lecturas/22-…) con una lectura preparada.
import puppeteer from "puppeteer-core";
import { mkdirSync, readdirSync } from "node:fs";
import { homedir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const base = process.argv[2] ?? "http://localhost:8100";
const lectura = "22-belleza-cuantica-ii-exuberancia";
const salida = join(dirname(fileURLToPath(import.meta.url)), "..", "video", "assets", "capturas");
mkdirSync(salida, { recursive: true });

// El Chrome que trae HyperFrames (puppeteer), sin instalar otro.
const raiz = join(homedir(), ".cache", "puppeteer", "chrome-headless-shell");
const version = readdirSync(raiz).sort().at(-1);
const chrome = join(raiz, version, "chrome-headless-shell-mac-arm64", "chrome-headless-shell");

const navegador = await puppeteer.launch({ executablePath: chrome, headless: true, args: ["--hide-scrollbars"] });
const pagina = await navegador.newPage();
await pagina.setViewport({ width: 1920, height: 1080, deviceScaleFactor: 1 });
const esperar = (ms) => new Promise((r) => setTimeout(r, ms));

async function foto(nombre) {
  await pagina.screenshot({ path: join(salida, `${nombre}.png`) });
  console.log(`  ${nombre}.png`);
}

// La biblioteca de lecturas.
await pagina.goto(`${base}/lecturas`, { waitUntil: "networkidle0" });
await esperar(1500);
await foto("lecturas");

// Preparar: arriba (libro y páginas), y cada paso del panel izquierdo.
await pagina.goto(`${base}/preparar?id=${lectura}`, { waitUntil: "networkidle0" });
await esperar(9000); // el PDF y el análisis
await foto("preparar-libro");
for (const [paso, nombre] of [["Figuras", "preparar-figuras"], ["Citas", "preparar-citas"], ["Música", "preparar-musica"]]) {
  await pagina.evaluate((texto) => {
    const h = [...document.querySelectorAll("h2.paso")].find((e) => e.textContent.includes(texto));
    h?.scrollIntoView({ block: "start" });
  }, paso);
  await esperar(1500);
  await foto(nombre);
}

// La consola de grabación.
await pagina.goto(`${base}/`, { waitUntil: "networkidle0" });
await esperar(4000);
await foto("consola");

await navegador.close();
