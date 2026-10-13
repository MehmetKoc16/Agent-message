"""Faz 0 deneyi icin sahte (bellekte) relay sunucusu. YALNIZ LOKAL DENEY.

Protokolun kucuk bir alt kumesi: mesaj gonder, long-poll ile oku, tur siniri.
Gercek sunucu Faz 1'de yazilacak. Yalnizca stdlib; yalnizca 127.0.0.1.
"""
import argparse
import hmac
import json
import re
import sys
import threading
import time
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

HOST = "127.0.0.1"  # degistirilemez
MAX_BODY = 16384
MAX_WAIT = 50
TYPES = {"soru", "gorev", "cevap", "sonuc", "bilgi", "kapat"}
NAME_RE = re.compile(r"^[A-Za-z0-9._-]{1,64}$")


class Store:
    """Mesaj listesi + kilit. Tum erisim cond uzerinden."""

    def __init__(self, agents, max_turns):
        self.agents = agents          # {ad: token}
        self.max_turns = max_turns
        self.cond = threading.Condition()
        self.messages = []            # id = indeks + 1
        self.closed = set()           # (room, thread)
        self.closing = False

    def add(self, room, sender, data):
        """Mesaji ekler. (mesaj, None) ya da (None, hata_kodu) doner."""
        with self.cond:
            new_id = len(self.messages) + 1
            thread = data.get("thread") or f"T-{new_id}"
            key = (room, thread)
            turn = sum(1 for m in self.messages
                       if m["room"] == room and m["thread"] == thread) + 1
            if key in self.closed or turn > self.max_turns:
                return None, "thread_locked"
            msg = {
                "id": new_id, "room": room, "from": sender,
                "to": data.get("to") or "", "type": data["type"],
                "thread": thread, "reply_to": data.get("reply_to"),
                "turn": turn, "max_turns": self.max_turns, "body": data["body"],
                "created_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            }
            self.messages.append(msg)
            if msg["type"] == "kapat":
                self.closed.add(key)
            self.cond.notify_all()
            return msg, None

    def fetch(self, room, who, after, wait):
        """Gorunur mesajlari doner; yoksa en cok `wait` sn bekler."""
        def visible():
            return [m for m in self.messages[after:]
                    if m["room"] == room and m["from"] != who
                    and (not m["to"] or m["to"] == who)]
        deadline = time.monotonic() + wait
        with self.cond:
            while True:
                found = visible()
                left = deadline - time.monotonic()
                if found or left <= 0 or self.closing:
                    return found
                self.cond.wait(left)


class Handler(BaseHTTPRequestHandler):
    server_version = "MockRelay/0"

    def setup(self):
        # Soket okuma zaman asimi: yarim govde gonderen istemci thread'i kilitlemesin.
        # Long-poll bundan etkilenmez: bekleme Condition uzerinde, soket okumada degil.
        self.timeout = self.server.io_timeout
        super().setup()

    def log_message(self, fmt, *args):
        # Yalniz istek satiri; govde ve Authorization asla loglanmaz.
        if not getattr(self.server, "quiet", False):
            sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _send(self, code, obj):
        raw = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(raw)

    def _who(self):
        auth = self.headers.get("Authorization", "")
        if not auth.startswith("Bearer "):
            return None
        tok = auth[7:].strip().encode("utf-8")
        found = None
        for name, t in self.server.store.agents.items():
            if hmac.compare_digest(t.encode("utf-8"), tok):
                found = name
        return found

    def _route(self):
        u = urlparse(self.path)
        parts = [p for p in u.path.split("/") if p]
        if parts == ["v1", "health"]:
            return "health", None, u
        if (len(parts) == 4 and parts[:2] == ["v1", "rooms"] and parts[3] == "messages"
                and NAME_RE.match(parts[2])):
            return "messages", parts[2], u
        return None, None, u

    def do_GET(self):
        kind, room, u = self._route()
        if kind == "health":
            return self._send(200, {"ok": True})
        if kind != "messages":
            return self._send(404, {"error": "not_found"})
        who = self._who()
        if not who:
            return self._send(401, {"error": "unauthorized"})
        q = parse_qs(u.query)
        try:
            after = int(q.get("after", ["0"])[0])
            wait = float(q.get("wait", ["0"])[0])
            if after < 0 or wait != wait:
                raise ValueError
        except ValueError:
            return self._send(400, {"error": "bad_query"})
        wait = min(max(wait, 0.0), MAX_WAIT)
        self._send(200, {"messages": self.server.store.fetch(room, who, after, wait)})

    def do_POST(self):
        kind, room, _ = self._route()
        if kind != "messages":
            return self._send(404, {"error": "not_found"})
        who = self._who()
        if not who:
            return self._send(401, {"error": "unauthorized"})
        try:
            length = int(self.headers.get("Content-Length", ""))
        except ValueError:
            return self._send(400, {"error": "content_length_required"})
        if length < 0:
            return self._send(400, {"error": "bad_request"})
        if length > MAX_BODY:
            # Istemci yazarken baglanti kopmasin diye makul olcude bosa oku.
            left = min(length, 1 << 20)
            while left > 0:
                chunk = self.rfile.read(min(65536, left))
                if not chunk:
                    break
                left -= len(chunk)
            return self._send(413, {"error": "too_large"})
        try:
            raw = self.rfile.read(length)
        except OSError:  # zaman asimi ya da kopan baglanti
            self.close_connection = True
            try:
                return self._send(408, {"error": "request_timeout"})
            except OSError:
                return
        try:
            data = json.loads(raw.decode("utf-8"))
        except (ValueError, UnicodeDecodeError):
            return self._send(400, {"error": "bad_json"})
        err = validate(data)
        if err:
            return self._send(400, {"error": err})
        msg, err = self.server.store.add(room, who, data)  # data["from"] kullanilmaz
        if err:
            return self._send(409, {"error": err})
        self._send(201, msg)


def validate(d):
    if not isinstance(d, dict):
        return "bad_body"
    if d.get("type") not in TYPES:
        return "bad_type"
    if not isinstance(d.get("body"), str) or not d["body"].strip():
        return "bad_body"
    for k in ("to", "thread"):
        if d.get(k) is not None and not isinstance(d[k], str):
            return "bad_" + k
        if d.get(k) and not NAME_RE.match(d[k]):
            return "bad_" + k
    r = d.get("reply_to")
    if r is not None and (not isinstance(r, int) or isinstance(r, bool) or r <= 0):
        return "bad_reply_to"
    return None


def make_server(agents, port=0, max_turns=20, quiet=False, io_timeout=15):
    srv = ThreadingHTTPServer((HOST, port), Handler)
    srv.daemon_threads = True
    srv.store = Store(agents, max_turns)
    srv.quiet = quiet
    srv.io_timeout = io_timeout
    return srv


def main():
    ap = argparse.ArgumentParser(description="Faz 0 sahte relay (yalniz lokal deney)")
    ap.add_argument("--port", type=int, default=8799)
    ap.add_argument("--agent", action="append", default=[], metavar="AD:TOKEN")
    ap.add_argument("--max-turns", type=int, default=20)
    a = ap.parse_args()
    if sys.stderr is not None:
        sys.stderr.reconfigure(encoding="utf-8")
    agents = {}
    for item in a.agent:
        name, sep, tok = item.partition(":")
        if not sep or not name or not tok:
            ap.error("--agent ad:token biciminde olmali")
        if not NAME_RE.match(name):
            ap.error("ajan adi yalniz harf, rakam, . _ - icerebilir (en fazla 64)")
        if name in agents:
            ap.error(f"ajan adi tekrarlandi: {name}")
        if tok in agents.values():
            ap.error("ayni token iki ajana verilemez")
        agents[name] = tok
    if not agents:
        ap.error("en az bir --agent gerekli")
    srv = make_server(agents, a.port, a.max_turns)
    print(f"mock relay http://{HOST}:{srv.server_address[1]} ajanlar={', '.join(agents)} "
          f"max_turns={a.max_turns}", file=sys.stderr)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        with srv.store.cond:
            srv.store.closing = True
            srv.store.cond.notify_all()
        srv.server_close()
        print("kapandi", file=sys.stderr)


if __name__ == "__main__":
    main()
