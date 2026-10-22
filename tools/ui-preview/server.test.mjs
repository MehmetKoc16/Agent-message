import test from "node:test";
import assert from "node:assert/strict";
import { once } from "node:events";
import { readFile } from "node:fs/promises";
import { createPreviewServer } from "./server.mjs";

async function preview(t) {
  const server = createPreviewServer();
  server.listen(0, "127.0.0.1"); await once(server, "listening");
  t.after(() => { server.close(); server.closeAllConnections(); });
  const base = `http://127.0.0.1:${server.address().port}`;
  return (path, options = {}) => fetch(base + path, {
    signal: AbortSignal.timeout(4000), ...options,
    headers: { Authorization: "Bearer relay-demo", ...options.headers },
  });
}
const post = (data) => ({ method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data) });

test("preview serves the product assets with CSP and an explicit demo notice", async (t) => {
  const request = await preview(t);
  const html = await request("/v1/izle", { headers: { Authorization: "" } });
  assert.equal(html.status, 200);
  assert.match(await html.text(), /YEREL ÖNİZLEME/);
  assert.match(html.headers.get("content-security-policy"), /script-src 'self'/);
  for (const name of ["izle.css", "izle.js"]) {
    const response = await request("/v1/izle/" + name);
    assert.equal(await response.text(), await readFile(new URL("../../internal/server/izle/" + name, import.meta.url), "utf8"));
  }
  const head = await request("/v1/izle", { method: "HEAD" });
  assert.equal(await head.text(), "");
  assert.equal((await request("/v1/izle/server.mjs")).status, 404);
});

test("demo login rejects other credentials and exposes only the project room", async (t) => {
  const request = await preview(t);
  assert.equal((await request("/v1/me", { headers: { Authorization: "Bearer wrong" } })).status, 401);
  const me = await (await request("/v1/me")).json();
  assert.equal(me.name, "demo"); assert.equal(me.yonetici, false);
  assert.deepEqual(me.rooms, ["agent-message"]);
  assert.equal((await request("/v1/rooms/private/log")).status, 403);
  const log = await (await request("/v1/rooms/agent-message/log")).json();
  assert.ok(log.messages.every((m) => m.room === "agent-message"));
  const agents = await (await request("/v1/rooms/agent-message/agents")).json();
  assert.deepEqual(agents.agents.map((a) => a.name), ["demo", "ece-claude", "mehmet-claude"]);
});

test("send wakes the live log and preserves untrusted content as data", async (t) => {
  const request = await preview(t);
  const original = await (await request("/v1/rooms/agent-message/log")).json();
  const after = original.messages.at(-1).id;
  const waiting = request(`/v1/rooms/agent-message/log?after=${after}&wait=2`);
  const body = '<img src=x onerror="alert(1)">\nTürkçe: ıİşğü';
  const response = await request("/v1/rooms/agent-message/messages", post({ type: "soru", body }));
  assert.equal(response.status, 201);
  const sent = await response.json();
  const live = await (await waiting).json();
  assert.equal(live.messages[0].id, sent.id); assert.equal(live.messages[0].body, body);
  assert.equal((await request("/v1/rooms/other-room/log")).status, 403);
});

test("replies stay in the selected thread and closed threads reject further sends", async (t) => {
  const request = await preview(t);
  const response = await request("/v1/rooms/agent-message/messages", post({ type: "cevap", body: "Yanıt", thread: "T-2", reply_to: 3 }));
  assert.equal(response.status, 201); assert.equal((await response.json()).thread, "T-2");
  const badReply = await request("/v1/rooms/agent-message/messages", post({ type: "cevap", body: "Yanıt", thread: "T-1", reply_to: 3 }));
  assert.equal(badReply.status, 400);
  assert.equal((await request("/v1/rooms/agent-message/threads/T-2/close", { method: "POST" })).status, 200);
  assert.equal((await request("/v1/rooms/agent-message/messages", post({ type: "bilgi", body: "Geç", thread: "T-2" }))).status, 409);
  assert.equal((await request("/v1/rooms/agent-message/messages", post({ type: "bilgi", body: "Yeni konuşma" }))).status, 201);
});

test("invalid bodies and oversized UTF-8 messages fail without modifying the log", async (t) => {
  const request = await preview(t);
  const before = await (await request("/v1/rooms/agent-message/log")).json();
  for (const data of [null, {}, { type: "wrong", body: "x" }, { type: "soru", body: " " }]) {
    assert.equal((await request("/v1/rooms/agent-message/messages", post(data))).status, 400);
  }
  assert.equal((await request("/v1/rooms/agent-message/messages", post({ type: "soru", body: "ş".repeat(9000) }))).status, 413);
  assert.equal((await request("/v1/rooms/agent-message/log?after=-1")).status, 400);
  assert.deepEqual(await (await request("/v1/rooms/agent-message/log")).json(), before);
});

test("each preview server starts with fresh disposable data", async (t) => {
  const a = await preview(t), b = await preview(t);
  const before = await (await b("/v1/rooms/agent-message/log")).json();
  await a("/v1/rooms/agent-message/messages", post({ type: "bilgi", body: "Sadece A" }));
  assert.deepEqual(await (await b("/v1/rooms/agent-message/log")).json(), before);
});
