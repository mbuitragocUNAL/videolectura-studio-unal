// Capturas de la consola real del estudio para los videos, a 1920×1080.
//
//   node guia/herramientas/capturas.mjs
//
// Escribe guia/compartido/capturas/*.png. Necesita el estudio abierto en :8100
// con una lectura preparada (./estudio abrir lecturas/22-…) y, para las
// capturas de un estudio vacío, otro sin lecturas en :8101:
//   docker run -d --name vls-vacio -p 127.0.0.1:8101:8080 estoc-studio:latest
import puppeteer from "puppeteer-core";
import { mkdirSync, readdirSync } from "node:fs";
import { homedir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const lectura = "22-belleza-cuantica-ii-exuberancia";
const salida = join(dirname(fileURLToPath(import.meta.url)), "..", "compartido", "capturas");
mkdirSync(salida, { recursive: true });

// Cada toma: la página, cuánto esperar, y opcionalmente un botón que pulsar o
// un paso del panel de preparación al que bajar.
const tomas = [
  { nombre: "vacio-lecturas", url: "http://localhost:8101/lecturas", espera: 2500 },
  { nombre: "vacio-nueva", url: "http://localhost:8101/preparar", espera: 3000 },
  { nombre: "lecturas", url: "http://localhost:8100/lecturas", espera: 2000 },
  { nombre: "nueva-estante", url: "http://localhost:8100/preparar", espera: 3000 },
  { nombre: "preparar-libro", url: `http://localhost:8100/preparar?id=${lectura}`, espera: 9000 },
  { nombre: "preparar-texto", url: `http://localhost:8100/preparar?id=${lectura}`, espera: 9000, pulsar: "Texto" },
  { nombre: "preparar-figuras", url: `http://localhost:8100/preparar?id=${lectura}`, espera: 9000, paso: "Figuras" },
  { nombre: "preparar-citas", url: `http://localhost:8100/preparar?id=${lectura}`, espera: 9000, paso: "Citas" },
  { nombre: "preparar-datos", url: `http://localhost:8100/preparar?id=${lectura}`, espera: 9000, paso: "Datos" },
  { nombre: "consola", url: "http://localhost:8100/", espera: 4000 },
  { nombre: "consola-montaje", url: "http://localhost:8100/?scope=montaje", espera: 5000 },
  { nombre: "consola-render", url: "http://localhost:8100/?scope=render", espera: 4000 },
  { nombre: "presentador", url: "http://localhost:8100/presentador", espera: 5000 },
];

const raiz = join(homedir(), ".cache", "puppeteer", "chrome-headless-shell");
const version = readdirSync(raiz).sort().at(-1);
const chrome = join(raiz, version, "chrome-headless-shell-mac-arm64", "chrome-headless-shell");
const navegador = await puppeteer.launch({ executablePath: chrome, headless: true, args: ["--hide-scrollbars"] });
const esperar = (ms) => new Promise((r) => setTimeout(r, ms));
const solo = process.argv.slice(2);

for (const t of tomas) {
  if (solo.length && !solo.includes(t.nombre)) continue;
  const pagina = await navegador.newPage();
  await pagina.setViewport({ width: 1920, height: 1080, deviceScaleFactor: 1 });
  await pagina.goto(t.url, { waitUntil: "networkidle0" });
  await esperar(t.espera);
  if (t.pulsar) {
    await pagina.evaluate((texto) => {
      const b = [...document.querySelectorAll("button")].find((e) => e.textContent.trim() === texto);
      b?.click();
    }, t.pulsar);
    await esperar(1500);
  }
  if (t.paso) {
    await pagina.evaluate((texto) => {
      const h = [...document.querySelectorAll("h2.paso")].find((e) => e.textContent.includes(texto));
      h?.scrollIntoView({ block: "start" });
    }, t.paso);
    await esperar(1200);
  }
  await pagina.screenshot({ path: join(salida, `${t.nombre}.png`) });
  console.log(`  ${t.nombre}.png`);
  await pagina.close();
}
await navegador.close();
