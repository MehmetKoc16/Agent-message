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
  const documentListeners = new Map();
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
    document: {visibilityState: "visible", getElementById: el, createElement: (tag) => new Element(tag),
      addEventListener(type, fn) { documentListeners.set(type, fn); },
      documentElement: new Element(), body: new Element()},
  });
  const source = await readFile(new URL("../../internal/server/izle/izle.js", import.meta.url), "utf8");
  vm.runInContext(source.replace(/\}\)\(\);\s*$/, `
    globalThis.inspect = {setSession(s) { active = s; }, addMessages, stop, refreshRecipients, refreshUnread, markSelectedRead, openUnreadThread, showFilter,
      sizes() { return {rows: rows.length, ids: ids.size, threads: threads.size}; }};
  })();`), context);
  const session = {name: "demo", token: "test-only", room: "r1", maxBody: 16384,
    controller: new AbortController(), roomController: new AbortController(), history: {oldest: 0, hasMore: false, browsing: false}};
  context.inspect.setSession(session);
  el("tip").value = "soru";
  return {el, session, inspect: context.inspect, documentListeners, setVisibility(value) { context.document.visibilityState = value; }, resize(nextWidth) {
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

test("listeden çıkan alıcı herkese dönüşmez; gönderim engellenir ve açık seçimle devam eder", async () => {
  const sent = [];
  const h = await harness(async (_path, options) => {
    sent.push(JSON.parse(options.body));
    return {status: 201, json: async () => ({thread: "T-1"})};
  });
  h.inspect.refreshRecipients([{name: "demo"}, {name: "davetli"}]);
  h.el("alici").value = "davetli"; h.el("metin").value = "belirli kişiye taslak";
  h.inspect.refreshRecipients([{name: "demo"}]);
  assert.equal(h.el("alici").value, "davetli");
  assert.equal(h.el("alici").options.find((o) => o.value === "davetli").disabled, true);
  assert.equal(h.el("alici-uyari").hidden, false);
  assert.equal(h.el("alici").checkValidity(), false);
  await h.el("gonderme").listeners.get("submit")({preventDefault() {}});
  assert.equal(sent.length, 0);
  assert.equal(h.el("metin").value, "belirli kişiye taslak");
  h.inspect.refreshRecipients([{name: "demo"}]);
  assert.equal(h.el("alici").value, "davetli");
  h.el("alici").value = ""; h.el("alici").listeners.get("change")();
  assert.equal(h.el("alici-uyari").hidden, true);
  await h.el("gonderme").listeners.get("submit")({preventDefault() {}});
  assert.equal(sent.length, 1);
  assert.equal(sent[0].to, "");
});

test("alıcı yeniden katıldığında aynı seçim ve taslak korunur, gönderim yeniden açılır", async () => {
  const h = await harness(async () => { throw new Error("ağ kullanılmamalı"); });
  h.inspect.refreshRecipients([{name: "davetli"}]); h.el("alici").value = "davetli";
  h.el("metin").value = "taslak";
  h.inspect.refreshRecipients([]);
  h.inspect.refreshRecipients([{name: "davetli"}]);
  assert.equal(h.el("alici").value, "davetli");
  assert.equal(h.el("alici").options.find((o) => o.value === "davetli").disabled, false);
  assert.equal(h.el("alici").checkValidity(), true);
  assert.equal(h.el("alici-uyari").hidden, true);
  assert.equal(h.el("metin").value, "taslak");
});

test("iki güncele dönüş düğmesi de taslağı, alıcıyı, türü ve yanıt bağlamını korur", async (t) => {
  for (const button of ["guncel", "yeni"]) {
    const sent = [], message = (id, thread) => ({id, thread, from: "davetli", type: "soru", body: "önceki", created_at: "2026-10-23T00:00:00Z"});
    const result = (data, status = 200) => ({ok: true, status, json: async () => data});
    const h = await harness(async (path, options) => {
      if (options.method === "POST") { sent.push(JSON.parse(options.body)); return result({thread: "T-1"}, 201); }
      if (path.endsWith("/log?tail=1")) return result({messages: [message(2, "T-2")], has_more: false});
      if (path.endsWith("/agents")) return result({agents: [{name: "davetli", kind: "ajan"}]});
      return new Promise(() => {});
    });
    t.after(() => h.inspect.stop());
    h.inspect.refreshRecipients([{name: "davetli"}]);
    h.inspect.addMessages([message(1, "T-1")], h.session);
    h.el("mesajlar").children[0].children[3].children[1].listeners.get("click")();
    h.el("metin").value = "yanıt taslağı";
    h.session.history.browsing = true;
    h.el(button).listeners.get("click")();
    await new Promise(setImmediate);
    assert.equal(h.el("metin").value, "yanıt taslağı", button);
    assert.equal(h.el("alici").value, "davetli", button);
    assert.equal(h.el("tip").value, "cevap", button);
    assert.equal(h.el("yanit").hidden, false, button);
    await h.el("gonderme").listeners.get("submit")({preventDefault() {}});
    assert.deepEqual(sent, [{to: "davetli", type: "cevap", body: "yanıt taslağı", thread: "T-1", reply_to: 1}], button);
  }
});

test("yönetime geçiş menü ve paneli kapatır; gizli sohbet Tab tuşunu engellemez", async () => {
  const h = await harness(async () => { throw new Error("ağ kullanılmamalı"); }, 390);
  h.el("katilimcilar-ac").listeners.get("click")();
  await h.el("yonetim-ac").listeners.get("click")();
  assert.equal(h.el("izleme").hidden, true);
  assert.equal(h.el("katilimci-panel").hidden, true);
  assert.equal(h.el("menu-perde").hidden, true);
  let prevented = false;
  h.documentListeners.get("keydown")({key: "Tab", preventDefault() { prevented = true; }});
  assert.equal(prevented, false);
  h.el("sohbete-don").listeners.get("click")();
  h.el("menu-ac").listeners.get("click")();
  await h.el("yonetim-ac").listeners.get("click")();
  assert.equal(h.el("izleme").classList.contains("menu-acik"), false);
});

test("yazılan taslağın konuşması canlı mesaj ve güncele dönüşle değişmez; başka odaya taşınmaz", async (t) => {
  const sent = [], message = (id, thread) => ({id, thread, from: "davetli", type: "bilgi", body: "mesaj", created_at: "2026-10-23T00:00:00Z"});
  const result = (data, status = 200) => ({ok: true, status, json: async () => data});
  const h = await harness(async (path, options) => {
    if (options.method === "POST") { sent.push(JSON.parse(options.body)); return result({thread: "T-1"}, 201); }
    if (path.endsWith("/log?tail=1")) return result({messages: [message(3, "T-3")], has_more: false});
    return new Promise(() => {});
  });
  t.after(() => h.inspect.stop());
  h.inspect.addMessages([message(1, "T-1")], h.session);
  h.el("metin").value = "sabit konuşma taslağı";
  h.inspect.addMessages([message(2, "T-2")], h.session);
  h.el("guncel").listeners.get("click")();
  await new Promise(setImmediate);
  await h.el("gonderme").listeners.get("submit")({preventDefault() {}});
  assert.equal(sent[0].thread, "T-1");
  h.el("metin").value = "yalnız bu odanın taslağı";
  h.el("oda").value = "r2"; h.el("oda").listeners.get("change")();
  await new Promise(setImmediate);
  assert.equal(h.el("metin").value, "");
  assert.equal(h.el("alici").value, "");
  assert.equal(h.el("yanit").hidden, true);
});

test("odasız hesabın mobil menüsü açılır ve çıkış işlemi giriş ekranına döner", async (t) => {
  const result = (data) => ({ok: true, status: 200, json: async () => data});
  const h = await harness(async (path) => {
    if (path === "/v1/me") return result({name: "demo", kind: "insan", yonetici: false});
    if (path === "/v1/rooms") return result({rooms: []});
    throw new Error("beklenmeyen istek");
  }, 390);
  t.after(() => h.inspect.stop());
  h.el("ad").value = "demo"; h.el("token").value = "test-only";
  h.el("giris").listeners.get("submit")({preventDefault() {}});
  await new Promise(setImmediate);
  assert.equal(h.el("odasiz-durum").hidden, false);
  assert.equal(h.el("oda-icerik").hidden, true);
  h.el("odasiz-menu-ac").listeners.get("click")();
  assert.equal(h.el("izleme").classList.contains("menu-acik"), true);
  assert.equal(h.el("menu-perde").hidden, false);
  h.el("cikis").listeners.get("click")();
  assert.equal(h.el("giris-ekrani").hidden, false);
  assert.equal(h.el("izleme").hidden, true);
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

test("okunmamış işareti yalnız açılan konuşmada kalkar; arka plan ve eski özet okumayı geri almaz", async () => {
  const writes = [];
  const snapshots = [{room:"r1",thread:"T-1",title:"first",total:1,last_id:1,last_incoming_id:1,read_id:0},
    {room:"r1",thread:"T-2",title:"second",total:1,last_id:2,last_incoming_id:2,read_id:0}];
  const h = await harness(async (path, options) => {
    if (path === "/v1/unread") return {ok:true, json:async () => ({threads: snapshots.map((s)=>({...s}))})};
    writes.push({path, body:JSON.parse(options.body)});
    return {ok:true,json:async()=>({ok:true})};
  });
  const msg = (id, thread, from="other") => ({id,thread,room:"r1",from,type:"bilgi",body:thread,created_at:"2026-10-25T00:00:00Z"});
  h.el("izleme").hidden = false;
  h.inspect.addMessages([msg(1,"T-1"),msg(2,"T-2")],h.session);
  await h.inspect.refreshUnread(h.session);
  const dot = (name) => h.el("konusma-listesi").children.find((button)=>button.title===name).children[0];
  assert.equal(dot("T-1").hidden,false);
  assert.equal(dot("T-2").hidden,false);
  assert.equal(writes.length,0, "loading all conversations must not mark them read");
  h.inspect.showFilter("T-1");await new Promise(setImmediate);
  assert.deepEqual(writes.map((r)=>r.body.last_id),[1]);
  assert.equal(dot("T-1").hidden,true);
  assert.equal(dot("T-2").hidden,false);
  await h.inspect.refreshUnread(h.session);
  assert.equal(dot("T-1").hidden,true,"stale server snapshot resurrected a cleared dot");
  h.setVisibility("hidden");
  h.inspect.addMessages([msg(3,"T-1")],h.session);await new Promise(setImmediate);
  assert.equal(writes.length,1);
  assert.equal(dot("T-1").hidden,false);
  h.setVisibility("visible");h.documentListeners.get("visibilitychange")();await new Promise(setImmediate);
  assert.equal(writes.at(-1).body.last_id,3);
  assert.equal(dot("T-1").hidden,true);
  h.el("izleme").hidden = true;
  h.inspect.addMessages([msg(4,"T-1")],h.session);await new Promise(setImmediate);
  assert.equal(writes.length,2,"management view marked hidden messages read");
  assert.equal(dot("T-1").hidden,false);
});

test("başarısız okuma kaydı işareti silmez ve görünmeyen yeni mesajı okundu saymaz", async () => {
  const writes = [];
  const h = await harness(async (path, options) => {
    if (path==="/v1/unread") return {ok:true,json:async()=>({threads:[{room:"r1",thread:"T-1",title:"old",total:2,last_id:200,last_incoming_id:200,read_id:0}]})};
    writes.push(JSON.parse(options.body));return {ok:false,json:async()=>({error:"test"})};
  });
  h.el("izleme").hidden = false;
  h.inspect.addMessages([{id:1,thread:"T-1",room:"r1",from:"other",type:"bilgi",body:"old",created_at:"2026-10-25T00:00:00Z"}],h.session);
  await h.inspect.refreshUnread(h.session);
  h.inspect.showFilter("T-1");await new Promise(setImmediate);
  assert.deepEqual(writes,[{last_id:1}]);
  assert.equal(h.el("konusma-listesi").children[0].children[0].hidden,false);
});
