import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import {readFile} from "node:fs/promises";

// Ürün betiğini küçük bir DOM taklidiyle çalıştırır; ağ yalnız testin sahte fetch'idir.
// Bu kontroller görsel tarayıcı testinin yerine geçmez.
class Element {
  constructor(tag = "div") {
    this.tagName = tag.toUpperCase(); this.open = false;
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
  querySelectorAll(selector) {
    const all = this.children.flatMap((child) => [child, ...child.querySelectorAll("*")]);
    if (selector === "*") return all;
    if (selector === "input:checked") return all.filter((child) => child.tagName === "INPUT" && child.checked);
    return [];
  }
  getClientRects() { return [{}]; }
  focus() {}
  showModal() { this.open = true; }
  close() { if (this.open) { this.open = false; this.listeners.get("close")?.({}); } }
  setCustomValidity(message) { this.validation = message; }
  checkValidity() { return !this.validation; }
  reportValidity() {}
}

async function harness(fetch, width = 1440) {
  const nodes = new Map();
  const el = (id) => { if (!nodes.has(id)) nodes.set(id, new Element()); return nodes.get(id); };
  const queries = new Map();
  const matches = (query) => /max-width: (\d+)px/.test(query) && width <= Number(query.match(/max-width: (\d+)px/)[1]);
  const media = (query) => {
    if (!queries.has(query)) {
      const listeners = [];
      queries.set(query, {matches: matches(query), addEventListener(_type, fn) { listeners.push(fn); }, listeners});
    }
    return queries.get(query);
  };
  const context = vm.createContext({
    fetch, TextEncoder, URLSearchParams, AbortController, setTimeout, clearTimeout, setInterval, clearInterval,
    location: {search: ""}, matchMedia: media, window: {matchMedia: media},
    sessionStorage: {getItem() { return null; }, setItem() {}, removeItem() {}},
    document: {getElementById: el, createElement: (tag) => new Element(tag), addEventListener() {},
      documentElement: new Element(), body: new Element()},
  });
  const source = await readFile(new URL("../../internal/server/izle/izle.js", import.meta.url), "utf8");
  vm.runInContext(source.replace(/\}\)\(\);\s*$/, `
    globalThis.inspect = {setSession(s) { active = s; }, addMessages, stop,
      sizes() { return {rows: rows.length, ids: ids.size, threads: threads.size}; }};
  })();`), context);
  const session = {name: "demo", token: "test-only", room: "r1", maxBody: 16384,
    controller: new AbortController(), roomController: new AbortController(), history: {oldest: 0, hasMore: false, browsing: false}};
  context.inspect.setSession(session);
  el("tip").value = "soru";
  return {el, session, inspect: context.inspect, resize(nextWidth) {
    width = nextWidth;
    for (const [query, value] of queries) {
      const next = matches(query);
      if (next === value.matches) continue;
      value.matches = next;
      for (const listener of value.listeners) listener(value);
    }
  }};
}

test("dar ekranda menü ve katılımcılar perdeyi paylaşır; perde doğru paneli kapatır", async () => {
  const h = await harness(async () => { throw new Error("ağ kullanılmamalı"); }, 390);
  h.el("katilimci-panel").hidden = true;
  h.el("menu-ac").listeners.get("click")();
  assert.equal(h.el("izleme").classList.contains("menu-acik"), true);
  assert.equal(h.el("menu-perde").hidden, false);
  h.el("katilimcilar-ac").listeners.get("click")();
  assert.equal(h.el("izleme").classList.contains("menu-acik"), false);
  assert.equal(h.el("katilimci-panel").hidden, false);
  assert.equal(h.el("menu-perde").hidden, false);
  h.el("menu-perde").listeners.get("click")();
  assert.equal(h.el("katilimci-panel").hidden, true);
  assert.equal(h.el("menu-perde").hidden, true);
  h.el("menu-ac").listeners.get("click")();
  h.el("menu-perde").listeners.get("click")();
  assert.equal(h.el("izleme").classList.contains("menu-acik"), false);
  assert.equal(h.el("menu-perde").hidden, true);
});

test("ekran büyüyüp küçülünce panel ve perde masaüstü/mobil yerleşime uyum sağlar", async () => {
  const h = await harness(async () => { throw new Error("ağ kullanılmamalı"); });
  h.resize(820);
  assert.equal(h.el("katilimci-panel").hidden, true);
  h.el("katilimcilar-ac").listeners.get("click")();
  assert.equal(h.el("menu-perde").hidden, false);
  h.resize(1440);
  assert.equal(h.el("katilimci-panel").hidden, false);
  assert.equal(h.el("menu-perde").hidden, true);
  h.resize(390);
  h.el("menu-ac").listeners.get("click")();
  h.resize(820);
  assert.equal(h.el("izleme").classList.contains("menu-acik"), false);
  assert.equal(h.el("menu-perde").hidden, true);
});

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

test("yeni oda doğrulama, açık üye seçimi ve başlıkla otomatik açılma akışını tamamlar", async (t) => {
  const calls = [];
  const result = (data, status = 200) => ({ok:status < 300,status,json:async()=>data});
  const h = await harness(async (path, options) => {
    calls.push({path,body:options.body && JSON.parse(options.body)});
    if (path === "/v1/yonetim/oturum") return result({oturum:"test-management",bitis:new Date(Date.now()+600000).toISOString()});
    if (path === "/v1/yonetim/hesaplar") return result([{name:"demo",kind:"insan",yonetici:true},{name:"davetli",kind:"insan"},{name:"iptal",revoked:true}]);
    if (path === "/v1/yonetim/odalar") return result({name:"oda-test",title:"Admin Sohbeti"},201);
    if (path === "/v1/rooms") return result({rooms:[{name:"r1",title:"Eski oda"},{name:"oda-test",title:"Admin Sohbeti"}]});
    if (path.endsWith("/log?tail=1")) return result({messages:[],has_more:false});
    if (path.endsWith("/agents")) return result({agents:[]});
    return new Promise(()=>{});
  });
  t.after(()=>h.inspect.stop());
  h.session.yonetici = true;
  await h.el("oda-ekle").listeners.get("click")();
  assert.equal(h.el("oda-dialog").open,true);
  assert.equal(h.el("oda-bilgiler").disabled,true);
  assert.equal(calls.length,0);
  h.el("oda-kod").value = "123456";
  await h.el("oda-form").listeners.get("submit")({preventDefault(){}});
  assert.equal(h.el("oda-kod").value,"");
  assert.equal(h.el("oda-bilgiler").disabled,false);
  assert.equal(h.el("oda-uyeler").children.length,2);
  const selected = h.el("oda-uyeler").children[1].children[0]; selected.checked=true;
  h.el("oda-baslik-girdi").value = "Admin Sohbeti";
  await h.el("oda-form").listeners.get("submit")({preventDefault(){}});
  const creation = calls.find((c)=>c.path === "/v1/yonetim/odalar");
  assert.equal(creation.body.baslik,"Admin Sohbeti");
  assert.deepEqual([...creation.body.uyeler],["demo","davetli"]);
  assert.equal(h.el("oda-baslik").textContent,"Admin Sohbeti");
  assert.equal(h.el("oda").value,"oda-test");
  assert.equal(h.el("oda-dialog").open,false);
});

test("yönetici olmayan oturum yeni oda akışını başlatamaz", async () => {
  let calls=0;
  const h=await harness(async()=>{calls++;throw new Error("ağ kullanılmamalı");});
  h.session.yonetici=false;
  await h.el("oda-ekle").listeners.get("click")();
  await h.el("oda-form").listeners.get("submit")({preventDefault(){}});
  assert.equal(calls,0);assert.equal(h.el("oda-dialog").open,false);
});
