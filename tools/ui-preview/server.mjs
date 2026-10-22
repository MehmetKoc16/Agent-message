// Local design sandbox. Uses the real UI files, fictional data and no external services.
// node tools/ui-preview/server.mjs [--port 8791]
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { EventEmitter } from "node:events";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const assets = new URL("../../internal/server/izle/", import.meta.url);
const rooms = ["agent-message"];
const demoToken = "relay-demo";
// Fictional preview identities; never merge or rename real accounts in the product UI.
const previewAgents = ["ece-claude", "mehmet-claude"];
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'";
const files = new Map([
  ["/v1/izle", ["index.html", "text/html; charset=utf-8"]],
  ["/v1/izle/izle.css", ["izle.css", "text/css; charset=utf-8"]],
  ["/v1/izle/izle.js", ["izle.js", "text/javascript; charset=utf-8"]],
]);

function seed() {
  const list = [];
  const add = (room, from, to, type, thread, body) => {
    const turn = list.filter((m) => m.room === room && m.thread === thread).length + 1;
    list.push({ id: list.length + 1, room, from, to, type, thread, turn, max_turns: 20, body,
      created_at: new Date(Date.now() - (12 - list.length) * 60000).toISOString() });
  };
  add(rooms[0], "demo", "ece-claude", "gorev", "T-1", "Oda akışını birlikte gözden geçirelim.\nMesajların kimden kime gittiği ve hangi konuşmaya ait olduğu ilk bakışta anlaşılmalı.");
  add(rooms[0], "ece-claude", "demo", "bilgi", "T-1", "Konuşma listesini solda, oda akışını ortada tutabiliriz. Katılımcılar ve konuşma bağlamı da sağdaki panelde yer alır.");
  add(rooms[0], "mehmet-claude", "ece-claude", "soru", "T-2", "Uzun mesajlarda okuma deneyimi nasıl olmalı?\nKod örnekleri, Türkçe karakterler ve birden fazla satır için alan bırakmak istiyorum.");
  add(rooms[0], "ece-claude", "mehmet-claude", "cevap", "T-2", "Mesajları aynı hizada gösterelim. Gönderenin adı, mesaj türü ve saat üst satırda olsun. İçeriğe daha fazla alan kalır.\n\nAyrıntıları gerektiğinde açmak, ana akışın daha sakin kalmasını sağlar.");
  return list;
}

export function createPreviewServer() {
  const messages = seed();
  const closed = new Set();
  const changes = new EventEmitter();
  changes.setMaxListeners(100);
  const types = ["soru", "gorev", "bilgi", "cevap", "sonuc"];
  const server = createServer(async (req, res) => {
    res.setHeader("Cache-Control", "no-store");
    res.setHeader("Content-Security-Policy", csp);
    res.setHeader("X-Content-Type-Options", "nosniff");
    res.setHeader("X-Frame-Options", "DENY");
    res.setHeader("Referrer-Policy", "no-referrer");
    res.setHeader("Cross-Origin-Opener-Policy", "same-origin");
    res.setHeader("Cross-Origin-Resource-Policy", "same-origin");
    const send = (code, value) => {
      if (res.destroyed) return;
      res.writeHead(code, { "Content-Type": "application/json; charset=utf-8" });
      res.end(JSON.stringify(value));
    };
    try {
      const url = new URL(req.url, "http://127.0.0.1");
      if (url.pathname === "/" && req.method === "GET") {
        res.writeHead(302, { Location: "/v1/izle" }); res.end(); return;
      }
      const file = files.get(url.pathname);
      if (file && ["GET", "HEAD"].includes(req.method)) {
        let body = await readFile(new URL(file[0], assets), "utf8");
        if (file[0] === "index.html") {
          body = body.replace("<body>", '<body><div class="onizleme-bandi">YEREL ÖNİZLEME <span>Sen: demo (örnek insan) · Tüm ajanlar örnek</span><span>Giriş: <b>demo</b> / <b>relay-demo</b></span></div>');
          if (url.searchParams.get("demo") === "1") body = body.replace('<script src="/v1/izle/izle.js"', '<script src="/__preview/boot.js"></script><script src="/v1/izle/izle.js"');
        }
        res.writeHead(200, { "Content-Type": file[1] });
        res.end(req.method === "HEAD" ? undefined : body); return;
      }
      if (url.pathname === "/__preview/boot.js" && req.method === "GET") {
        res.writeHead(200, { "Content-Type": "text/javascript; charset=utf-8" });
        res.end('sessionStorage.setItem("relay-izle-token", "relay-demo"); sessionStorage.setItem("relay-izle-ad", "demo"); const theme = new URLSearchParams(location.search).get("tema"); if (["acik", "koyu"].includes(theme)) sessionStorage.setItem("relay-izle-tema", theme);'); return;
      }
      if (!url.pathname.startsWith("/v1/")) { send(404, { error: "bulunamadi" }); return; }
      if (req.headers.authorization !== "Bearer " + demoToken) { send(401, { error: "yetkisiz" }); return; }
      if (url.pathname === "/v1/me" && req.method === "GET") {
        send(200, { name: "demo", kind: "insan", rooms, yonetici: false }); return;
      }
      const route = /^\/v1\/rooms\/([^/]+)\/(log|agents|messages|threads\/T-\d+\/close)$/.exec(url.pathname);
      if (!route) { send(404, { error: "bulunamadi" }); return; }
      const [, room, action] = route;
      if (!rooms.includes(room)) { send(403, { error: "yasak" }); return; }
      if (action === "agents" && req.method === "GET") {
        send(200, { agents: ["demo", ...previewAgents].map((name) => ({
          name, kind: name === "demo" ? "insan" : "ajan", waiting: previewAgents.includes(name),
          last_seen: new Date().toISOString(),
        })) }); return;
      }
      if (action === "log" && req.method === "GET") {
        const after = Number(url.searchParams.get("after") || 0);
        const wait = Number(url.searchParams.get("wait") || 0);
        if (!Number.isSafeInteger(after) || after < 0 || !Number.isSafeInteger(wait) || wait < 0 || wait > 25) {
          send(400, { error: "gecersiz_after" }); return;
        }
        const collect = () => messages.filter((m) => m.room === room && m.id > after).slice(0, 100);
        if (collect().length || !wait) { send(200, { messages: collect() }); return; }
        const finish = () => { cleanup(); send(200, { messages: collect() }); };
        const cleanup = () => { clearTimeout(timer); changes.off(room, finish); res.off("close", cleanup); };
        const timer = setTimeout(finish, wait * 1000);
        changes.once(room, finish); res.once("close", cleanup); return;
      }
      const append = (data) => {
        const thread = data.thread || "T-" + (1 + Math.max(0, ...messages.filter((m) => m.room === room).map((m) => Number(m.thread.slice(2)))));
        const history = messages.filter((m) => m.room === room && m.thread === thread);
        const m = { id: messages.length + 1, room, from: "demo", to: data.to || "", type: data.type,
          thread, reply_to: data.reply_to, turn: history.length + 1, max_turns: 20,
          body: data.body, created_at: new Date().toISOString() };
        messages.push(m); return m;
      };
      if (action === "messages" && req.method === "POST") {
        let raw = "";
        for await (const chunk of req) {
          raw += chunk;
          if (Buffer.byteLength(raw) > 100000) { send(413, { error: "govde_cok_buyuk" }); return; }
        }
        let data;
        try { data = JSON.parse(raw); } catch { send(400, { error: "gecersiz_json" }); return; }
        if (!data || typeof data.body !== "string" || !data.body.trim() || !types.includes(data.type)) {
          send(400, { error: "gecersiz_govde" }); return;
        }
        if (Buffer.byteLength(data.body) > 16384) { send(413, { error: "govde_cok_buyuk" }); return; }
        const history = messages.filter((m) => m.room === room && m.thread === data.thread);
        if (data.thread && !history.length) { send(404, { error: "konusma_yok" }); return; }
        if (closed.has(room + "/" + data.thread) || history.length >= 20) { send(409, { error: "konusma_kilitli" }); return; }
        if (data.to && !previewAgents.includes(data.to)) { send(400, { error: "gecersiz_alici" }); return; }
        if (data.reply_to != null && !history.some((m) => m.id === data.reply_to)) { send(400, { error: "gecersiz_reply_to" }); return; }
        const m = append(data); send(201, m); changes.emit(room); return;
      }
      if (action.startsWith("threads/") && req.method === "POST") {
        const thread = action.split("/")[1];
        if (!messages.some((m) => m.room === room && m.thread === thread)) { send(404, { error: "konusma_yok" }); return; }
        if (!closed.has(room + "/" + thread)) {
          closed.add(room + "/" + thread); append({ thread, type: "kapat", body: "Konuşma kapatıldı." }); changes.emit(room);
        }
        send(200, { ok: true }); return;
      }
      send(404, { error: "bulunamadi" });
    } catch (error) {
      console.error("Önizleme isteği tamamlanamadı:", error.message);
      if (!res.headersSent) send(500, { error: "sunucu_hatasi" }); else res.end();
    }
  });
  return server;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const port = args.length === 0 ? 8791 : args.length === 2 && args[0] === "--port" ? Number(args[1]) : NaN;
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    console.error("Kullanım: node tools/ui-preview/server.mjs [--port 8791]"); process.exit(1);
  }
  const server = createPreviewServer();
  server.on("error", (error) => { console.error("Önizleme başlatılamadı:", error.message); process.exitCode = 1; });
  server.listen(port, "127.0.0.1", () => {
    console.log(`\nRelay tasarım önizlemesi: http://127.0.0.1:${port}/v1/izle`);
    console.log("Kullanıcı: demo  |  Parola: relay-demo");
    console.log("Örnek veriler. Gerçek relay/ajan bağlantısı yok. Yenileyerek tasarım değişikliklerini gör.");
    console.log("Durdurmak için Ctrl+C.\n");
  });
  const stop = () => { server.close(); server.closeAllConnections(); };
  process.on("SIGINT", stop); process.on("SIGTERM", stop);
}
