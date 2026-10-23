import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import {readFile} from "node:fs/promises";

// Ürün betiğini küçük bir DOM taklidiyle çalıştırır; ağ yalnız testin sahte fetch'idir.
// Bu kontroller görsel tarayıcı testinin yerine geçmez.
class Element {
  constructor() {
    this.children = []; this.listeners = new Map(); this.value = ""; this.disabled = false;
    this.hidden = false; this.dataset = {}; this.textContent = ""; this.scrollTop = 0;
    this.scrollHeight = 0; this.clientHeight = 0; this.parentElement = {setAttribute() {}};
    const classes = new Set();
    this.classList = {add: (c) => classes.add(c), remove: (c) => classes.delete(c), contains: (c) => classes.has(c),
      toggle: (c, on) => { if (on ?? !classes.has(c)) classes.add(c); else classes.delete(c); }};
  }
  addEventListener(type, fn) { this.listeners.set(type, fn); }
  append(...children) { for (const child of children) { child.parentElement = this; this.children.push(child); } }
  prepend(child) { child.parentElement = this; this.children.unshift(child); }
  replaceChildren(...children) { this.children = []; this.append(...children); }
  remove() { if (this.parentElement.children) this.parentElement.children = this.parentElement.children.filter((c) => c !== this); }
  get firstChild() { return this.children[0]; }
  get options() { return this.children; }
  setAttribute() {}
  querySelectorAll() { return []; }
  getClientRects() { return [{}]; }
  focus() {}
  close() {}
  setCustomValidity(message) { this.validation = message; }
  checkValidity() { return !this.validation; }
  reportValidity() {}
}

async function harness(fetch) {
  const nodes = new Map();
  const el = (id) => { if (!nodes.has(id)) nodes.set(id, new Element()); return nodes.get(id); };
  const media = () => ({matches: false, addEventListener() {}});
  const context = vm.createContext({
    fetch, TextEncoder, URLSearchParams, AbortController, setTimeout, clearTimeout, setInterval, clearInterval,
    location: {search: ""}, matchMedia: media, window: {matchMedia: media},
    sessionStorage: {getItem() { return null; }, setItem() {}, removeItem() {}},
    document: {getElementById: el, createElement: () => new Element(), addEventListener() {},
      documentElement: new Element(), body: new Element()},
  });
  const source = await readFile(new URL("../../internal/server/izle/izle.js", import.meta.url), "utf8");
  vm.runInContext(source.replace(/\}\)\(\);\s*$/, `
    globalThis.inspect = {setSession(s) { active = s; }, addMessages,
      sizes() { return {rows: rows.length, ids: ids.size, threads: threads.size}; }};
  })();`), context);
  const session = {name: "demo", token: "test-only", room: "r1", maxBody: 16384,
    roomController: new AbortController(), history: {oldest: 0, hasMore: false, browsing: false}};
  context.inspect.setSession(session);
  el("tip").value = "soru";
  return {el, session, inspect: context.inspect};
}

test("409 yeniden denemesi alıcıyı, türü ve ilk metni korur; yeni taslağı silmez", async () => {
  let finishFirst;
  const initial = new Promise((resolve) => { finishFirst = resolve; });
  const sent = [];
  const h = await harness(async (_path, options) => {
    sent.push(JSON.parse(options.body));
    return sent.length === 1 ? initial : {status: 201, json: async () => ({thread: "T-2"})};
  });
  h.inspect.addMessages([{id: 1, from: "other", thread: "T-1", type: "soru", body: "önceki", created_at: "2026-10-23T00:00:00Z"}], h.session);
  h.el("alici").value = "ilk-alici"; h.el("metin").value = "ilk mesaj";
  const pending = h.el("gonderme").listeners.get("submit")({preventDefault() {}});
  assert.equal(sent.length, 1);
  h.el("alici").value = "baska-alici"; h.el("tip").value = "gorev"; h.el("metin").value = "yeni taslak";
  finishFirst({status: 409, json: async () => ({error: "konusma_kilitli"})});
  await pending;
  assert.deepEqual(sent[1], {to: "ilk-alici", type: "soru", body: "ilk mesaj"});
  assert.equal(h.el("metin").value, "yeni taslak");
});

test("UTF-8 sınırını aşan Türkçe metin ağ isteği yapmadan reddedilir", async () => {
  let calls = 0;
  const h = await harness(async () => { calls++; throw new Error("ağ kullanılmamalı"); });
  h.el("metin").value = "ş".repeat(9000);
  await h.el("gonderme").listeners.get("submit")({preventDefault() {}});
  assert.equal(calls, 0);
  assert.equal(h.el("sayac").textContent, "18000 / 16384 bayt");
  assert.equal(h.el("metin").checkValidity(), false);
});

test("canlı mesaj akışında DOM, kimlikler ve konuşmalar sınırlı tutulur", async () => {
  const h = await harness(async () => { throw new Error("ağ kullanılmamalı"); });
  const list = Array.from({length: 700}, (_, i) => ({id: i+1, from: "other", thread: "T-"+(i+1),
    type: "bilgi", body: "mesaj", created_at: "2026-10-23T00:00:00Z"}));
  h.inspect.addMessages(list, h.session);
  const sizes = h.inspect.sizes();
  assert.equal(sizes.rows, 500); assert.equal(sizes.ids, 500); assert.equal(sizes.threads, 500);
  assert.equal(h.el("mesajlar").children.length, 500);
  assert.equal(h.session.history.oldest, 201); assert.equal(h.session.history.hasMore, true);
});
