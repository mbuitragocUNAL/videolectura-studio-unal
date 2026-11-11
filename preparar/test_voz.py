"""voz.py contra un ElevenLabs de mentira en localhost.

    preparar/.venv/bin/python -m unittest preparar/test_voz.py
"""
from __future__ import annotations

import base64
import importlib
import json
import os
import re
import subprocess
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

AQUI = Path(__file__).resolve().parent


def tono(ruta: Path, segundos: float = 2.0, volumen: str = "-30dB") -> bytes:
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i",
                    f"sine=frequency=220:duration={segundos}", "-af", f"volume={volumen}",
                    "-c:a", "libmp3lame", "-q:a", "4", str(ruta)], check=True)
    return ruta.read_bytes()


def lufs(ruta: Path) -> float:
    p = subprocess.run(["ffmpeg", "-hide_banner", "-nostats", "-i", str(ruta), "-af", "ebur128",
                        "-f", "null", "-"], capture_output=True, text=True)
    return float(re.findall(r"I:\s+(-?[\d.]+) LUFS", p.stderr)[-1])


class Falso(BaseHTTPRequestHandler):
    audio = b""
    pedidos: list = []

    def log_message(self, *a):
        pass

    def responder(self, codigo, cuerpo, tipo="application/json"):
        datos = cuerpo if isinstance(cuerpo, bytes) else json.dumps(cuerpo).encode()
        self.send_response(codigo)
        self.send_header("Content-Type", tipo)
        self.send_header("Content-Length", str(len(datos)))
        self.end_headers()
        self.wfile.write(datos)

    def do_GET(self):
        if self.headers.get("xi-api-key") != "clave-de-prueba":
            return self.responder(401, {"detail": {"message": "Invalid API key"}})
        if self.path.startswith("/v1/user/subscription"):
            return self.responder(200, {"tier": "free", "character_count": 120, "character_limit": 10000})
        if self.path.startswith("/v2/voices"):
            return self.responder(200, {"voices": [
                {"voice_id": "premade1", "name": "Adam", "category": "premade"},
                {"voice_id": "v-soc", "name": "Sócrates · ESTOC", "category": "generated"}],
                "has_more": False})
        self.responder(404, {"detail": "no"})

    def do_POST(self):
        largo = int(self.headers.get("Content-Length") or 0)
        cuerpo = json.loads(self.rfile.read(largo) or b"{}")
        Falso.pedidos.append((self.path, cuerpo))
        if self.headers.get("xi-api-key") != "clave-de-prueba":
            return self.responder(401, {"detail": {"message": "Invalid API key"}})
        if self.path.startswith("/v1/text-to-speech/"):
            return self.responder(200, Falso.audio, "audio/mpeg")
        if self.path.startswith("/v1/text-to-voice/design"):
            b64 = base64.b64encode(Falso.audio).decode()
            return self.responder(200, {"text": cuerpo.get("text", "auto"), "previews": [
                {"audio_base_64": b64, "generated_voice_id": f"gen{i}", "media_type": "audio/mpeg",
                 "duration_secs": 2.0, "language": "es"} for i in (1, 2, 3)]})
        if self.path == "/v1/text-to-voice":
            return self.responder(200, {"voice_id": "nueva-" + cuerpo["generated_voice_id"],
                                        "name": cuerpo["voice_name"]})
        self.responder(404, {"detail": "no"})


class PruebaVoz(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = Path(tempfile.mkdtemp())
        cls.servidor = HTTPServer(("127.0.0.1", 0), Falso)
        threading.Thread(target=cls.servidor.serve_forever, daemon=True).start()
        Falso.audio = tono(cls.tmp / "tono.mp3")
        os.environ.update(STUDIO_VOCES_DIR=str(cls.tmp / "voces"),
                          ELEVENLABS_URL=f"http://127.0.0.1:{cls.servidor.server_port}",
                          ELEVENLABS_API_KEY="clave-de-prueba")
        sys.path.insert(0, str(AQUI))
        cls.voz = importlib.import_module("voz")
        importlib.reload(cls.voz)

    @classmethod
    def tearDownClass(cls):
        cls.servidor.shutdown()

    def setUp(self):
        Falso.pedidos = []
        self.carpeta = Path(tempfile.mkdtemp(dir=self.tmp))
        (self.carpeta / "audio" / "tts").mkdir(parents=True)
        self.clips = [
            {"clip": "01-socrates", "personaje": "Sócrates", "texto": "Y ahora, dije, déjame mostrar en una figura.", "linea": 3},
            {"clip": "02-glaucon", "personaje": "Glaucón", "texto": "Lo veo.", "linea": 5},
        ]
        self.escribir_marcas()
        (self.carpeta / "lectura.toml").write_text(
            '[lectura]\nnumero = 7\ntitulo = "La caverna"\n\n'
            '[voces."Sócrates"]\ndescripcion = "anciano, socarrón"\nmotor = "elevenlabs"\nvoz = "v-soc"\n\n'
            '[voces."Glaucón"]\nmotor = "manual"\n', encoding="utf-8")

    def escribir_marcas(self):
        (self.carpeta / "marcas.json").write_text(json.dumps({"clips": self.clips}), encoding="utf-8")

    def estados(self):
        return {c["clip"]: c["estado"] for c in self.voz.estado(self.carpeta)["clips"]}

    def test_genera_lo_que_falta_a_la_sonoridad_de_la_narracion(self):
        self.assertEqual(self.estados(), {"01-socrates": "falta", "02-glaucon": "falta"})
        r = self.voz.generar(self.carpeta)
        self.assertEqual([h["clip"] for h in r["hechos"]], ["01-socrates"])
        # Glaucón va a mano: no se genera, se dice por qué.
        self.assertEqual(r["errores"], [{"clip": "02-glaucon", "error": "se sube a mano"}])
        mp3 = self.carpeta / "audio" / "tts" / "01-socrates.mp3"
        self.assertAlmostEqual(lufs(mp3), -16, delta=1.0)
        ruta, cuerpo = Falso.pedidos[0]
        self.assertEqual(ruta, "/v1/text-to-speech/v-soc?output_format=mp3_44100_128")
        self.assertEqual(cuerpo["text"], self.clips[0]["texto"])

        # Lo que ya está no se vuelve a pagar.
        Falso.pedidos = []
        self.voz.generar(self.carpeta)
        self.assertEqual(Falso.pedidos, [])
        self.assertEqual(self.estados()["01-socrates"], "lista")

    def test_sabe_cuando_la_cita_o_la_voz_cambiaron(self):
        self.voz.generar(self.carpeta)
        self.clips[0]["texto"] = "Otra cosa distinta."
        self.escribir_marcas()
        self.assertEqual(self.estados()["01-socrates"], "texto")
        self.voz.generar(self.carpeta)
        self.assertEqual(self.estados()["01-socrates"], "lista")

        toml = self.carpeta / "lectura.toml"
        toml.write_text(toml.read_text(encoding="utf-8").replace('voz = "v-soc"', 'voz = "otra"'), encoding="utf-8")
        self.assertEqual(self.estados()["01-socrates"], "otra-voz")

    def test_un_audio_propio_queda_como_manual(self):
        r = self.voz.subir(self.carpeta, "02-glaucon", self.tmp / "tono.mp3")
        self.assertGreater(r["dur"], 1.5)
        self.assertEqual(self.estados()["02-glaucon"], "lista")
        # Cambiar de voz no invalida lo que se subió a mano; cambiar el texto sí.
        self.clips[1]["texto"] = "Lo veo bien."
        self.escribir_marcas()
        self.assertEqual(self.estados()["02-glaucon"], "texto")

    def test_disenar_y_elegir_en_elevenlabs(self):
        r = self.voz.disenar(self.carpeta, "Sócrates", "elevenlabs")
        self.assertEqual([c["id"] for c in r["candidatas"]], ["gen1", "gen2", "gen3"])
        # Sus líneas son menos de 100 caracteres: ElevenLabs inventa el texto.
        self.assertTrue(Falso.pedidos[0][1]["auto_generate_text"])
        self.assertIn("anciano, socarrón", Falso.pedidos[0][1]["voice_description"])
        self.assertTrue((self.voz.VOCES_DIR / r["carpeta"] / "2.mp3").exists())

        e = self.voz.elegir("Sócrates", "gen2")
        self.assertEqual(e["voz"], "nueva-gen2")
        self.voz.asignar(self.carpeta, "Sócrates", e["motor"], e["voz"], e["vozNombre"])
        texto = (self.carpeta / "lectura.toml").read_text(encoding="utf-8")
        self.assertIn('voz = "nueva-gen2"', texto)
        self.assertIn('descripcion = "anciano, socarrón"', texto)

    def test_disenar_pide_la_descripcion(self):
        with self.assertRaisesRegex(self.voz.ErrorVoz, "cómo suena Glaucón"):
            self.voz.disenar(self.carpeta, "Glaucón", "elevenlabs")

    def test_una_clave_mala_se_explica(self):
        os.environ["ELEVENLABS_API_KEY"] = "mala"
        try:
            with self.assertRaisesRegex(self.voz.ErrorVoz, "no acepta la clave: Invalid API key"):
                self.voz.eleven_cuenta()
        finally:
            os.environ["ELEVENLABS_API_KEY"] = "clave-de-prueba"

    def test_cuenta_y_voces(self):
        self.assertEqual(self.voz.eleven_cuenta()["limite"], 10000)
        voces = self.voz.eleven_voces()
        self.assertEqual(voces[0]["id"], "v-soc")  # las propias primero


if __name__ == "__main__":
    unittest.main()
