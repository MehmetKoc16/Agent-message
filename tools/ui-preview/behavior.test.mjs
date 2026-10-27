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
  before(node) { const list = this.parentElement.children; node.parentElement = this.parentElement; list.splice(list.indexOf(this), 0, node); }
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
  // Rows stack at 100px each inside their scrolled parent; hidden rows take no space.
  getBoundingClientRect() {
    const siblings = this.parentElement.children || [];
    const top = siblings.slice(0, siblings.indexOf(this)).filter((child) => !child.hidden).length * 100;
    return {top: top - (this.parentElement.scrollTop || 0), height: 100};
  }
  focus() {}
  showModal() { this.open = true; }
  close() { if (this.open) { this.open = false; this.listeners.get("close")?.({}); } }
  setCustomValidity(message) { this.validation = message; }
  checkValidity() { return !this.validation; }
  reportValidity() {}
}

async function harness(fetch, width = 1440, globals = {}) {
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
    ...globals,
  });
  const source = await readFile(new URL("../../internal/server/izle/izle.js", import.meta.url), "utf8");
  vm.runInContext(source.replace(/\}\)\(\);\s*$/, `
    globalThis.inspect = {setSession(s) { active = s; }, addMessages, stop, refreshRecipients, refreshUnread, markSelectedRead, openUnreadThread, showFilter, unreadLoop,
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

test("oda içinden mevcut yönetici hesabı eklenir ve çıkarılır; her değişiklik TOTP oturumu taşır", async (t) => {
  let joined = false;
  const calls = [];
  const h = await harness(async (path, options) => {
    calls.push({path, options});
    const data = path.endsWith("/oturum") ? {oturum:"test-session", bitis:new Date(Date.now()+600000).toISOString()} :
      path.endsWith("/hesaplar") ? [{name:"other-admin",kind:"insan",yonetici:true,rooms:joined?["r1"]:[]}] : {ok:true};
    if (options.method === "POST" && path.endsWith("/odalar")) joined = true;
    if (options.method === "DELETE") joined = false;
    return {ok:true, status:200, json:async () => data};
  });
  t.after(() => h.inspect.stop());
  h.session.yonetici = true;
  await h.el("uyeleri-yonet").listeners.get("click")();
  assert.equal(h.el("uyelik-hesaplar").children.length, 0);
  h.el("uyelik-kod").value = "123456";
  await h.el("uyelik-form").listeners.get("submit")({preventDefault(){}});
  let button = h.el("uyelik-hesaplar").children[0].children[3];
  assert.equal(button.textContent, "Ekle");
  await button.listeners.get("click")();
  button = h.el("uyelik-hesaplar").children[0].children[3];
  assert.equal(button.textContent, "Çıkar");
  await button.listeners.get("click")();
  assert.equal(joined, false);
  const changes = calls.filter(({options, path}) => ["POST","DELETE"].includes(options.method) && !path.endsWith("/oturum"));
  assert.equal(changes.length, 2);
  assert.equal(changes[0].options.headers["X-Yonetim-Oturum"], "test-session");
  assert.equal(changes[1].options.headers["X-Yonetim-Oturum"], "test-session");
});

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

test("okunmamış işareti açılan konuşmada kalkar; arka plan ve eski özet okumayı geri almaz", async () => {
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

test("tüm konuşmalarda yalnız ekranda duran mesajlar okundu sayılır", async () => {
  const writes = [];
  let observer;
  class FakeIntersectionObserver {
    constructor(callback) { this.callback = callback; this.targets = new Set(); observer = this; }
    observe(target) { this.targets.add(target); }
    unobserve(target) { this.targets.delete(target); }
    disconnect() { this.targets.clear(); }
  }
  const h = await harness(async (path, options) => {
    if (path === "/v1/unread") return {ok:true, json:async () => ({threads: [1, 2, 3].map((n) =>
      ({room:"r1",thread:"T-"+n,title:"T-"+n,total:1,last_id:n,last_incoming_id:n,read_id:0}))})};
    writes.push({path, body:JSON.parse(options.body)});
    return {ok:true,json:async()=>({ok:true})};
  }, 1440, {IntersectionObserver: FakeIntersectionObserver});
  const msg = (id) => ({id,thread:"T-"+id,room:"r1",from:"other",type:"bilgi",body:"T-"+id,created_at:"2026-10-25T00:00:00Z"});
  h.el("izleme").hidden = false;
  h.inspect.addMessages([msg(1),msg(2),msg(3)],h.session);
  await h.inspect.refreshUnread(h.session);
  assert.equal(writes.length,0,"loading all conversations must not mark them read");
  const row = (id) => [...observer.targets][id - 1];
  const show = (id, ratio = 1, height = 100) => ({target:row(id),isIntersecting:ratio > 0,intersectionRatio:ratio,
    intersectionRect:{height},rootBounds:{height:400}});
  const settle = () => new Promise((resolve) => setTimeout(resolve, 550));
  observer.callback([show(1, 0.1, 10), show(2), show(3)]); await settle();
  assert.deepEqual(writes.map((w) => [w.path.split("/")[5], w.body.last_id]).sort(), [["T-2",2],["T-3",3]]);
  const dot = (name) => h.el("konusma-listesi").children.find((button)=>button.title===name).children[0];
  assert.equal(dot("T-1").hidden,false,"a row barely on screen was marked read");
  assert.equal(dot("T-2").hidden,true);
  h.setVisibility("hidden");
  observer.callback([show(1)]); await settle();
  assert.equal(writes.length,2,"a background tab marked rows read");
  h.setVisibility("visible");h.documentListeners.get("visibilitychange")();await new Promise(setImmediate);
  assert.equal(writes.at(-1).body.last_id,1);
  assert.equal(dot("T-1").hidden,true);
});

test("mesajın konuşma etiketi ve Yanıtla düğmesi tıklanan mesajı yerinde tutar", async () => {
  const h = await harness(async () => ({ok:true,json:async()=>({ok:true})}));
  const list = Array.from({length: 10}, (_, i) => ({id:i+1,thread:i % 2 ? "T-2" : "T-1",room:"r1",from:"other",type:"bilgi",
    body:"m"+(i+1),created_at:"2026-10-25T00:00:00Z"}));
  h.inspect.addMessages(list,h.session);
  const messages = h.el("mesajlar"), rowOf = (id) => messages.children[id - 1];
  for (const button of ["konusma", "yanitla"]) {
    h.inspect.showFilter("");
    messages.scrollTop = 500;
    const before = rowOf(7).getBoundingClientRect().top;
    const index = button === "konusma" ? 0 : 1;
    rowOf(7).children.at(-1).children[index].listeners.get("click")();
    assert.equal(h.el("filtre").textContent,"Konuşma: T-1");
    assert.equal(rowOf(7).getBoundingClientRect().top,before,button + " moved the clicked message");
  }
});

test("eski konuşmadan tüm konuşmalara dönünce oda görünümü yeniden yüklenir", async (t) => {
  const msg = (id, thread) => ({id,thread,room:"r1",from:"other",type:"bilgi",body:thread+"/"+id,created_at:"2026-10-25T00:00:00Z"});
  const paths = [];
  const h = await harness(async (path) => {
    paths.push(path);
    if (path.includes("wait=")) return new Promise(() => {});
    const data = path === "/v1/unread" ? {threads: [["T-1",101],["T-2",102],["T-9",9]].map(([thread,id]) =>
        ({room:"r1",thread,title:thread,total:1,last_id:id,last_incoming_id:id,read_id:id}))} :
      path === "/v1/rooms/r1/threads/T-9/log" ? {messages: [msg(8,"T-9"), msg(9,"T-9")], has_more: false} :
      path === "/v1/rooms/r1/log?tail=1" ? {messages: [msg(101,"T-1"), msg(102,"T-2")], has_more: true} :
      path.endsWith("/agents") ? {agents: []} : {ok: true};
    return {ok:true,status:200,json:async () => data};
  });
  t.after(() => h.inspect.stop());
  const shown = () => h.el("mesajlar").children.filter((row) => !row.hidden).map((row) => row.children[2].textContent);
  h.inspect.addMessages([msg(101,"T-1"), msg(102,"T-2")], h.session);
  await h.inspect.refreshUnread(h.session);
  await h.inspect.openUnreadThread("T-9");
  assert.deepEqual(shown(), ["T-9/8","T-9/9"]);
  h.inspect.showFilter("");
  for (let i = 0; i < 5; i++) await new Promise(setImmediate);
  assert.equal(h.el("filtre").textContent, "Tüm konuşmalar");
  assert.deepEqual(shown(), ["T-1/101","T-2/102"], "room messages were lost after an old conversation");
  assert.ok(paths.includes("/v1/rooms/r1/log?tail=1"));
});

test("arka plan sekmesi okunmamış özetini sorgulamaz, görünür olunca hemen yeniler", async (t) => {
  let summaries = 0;
  const h = await harness(async (path) => {
    if (path === "/v1/unread") summaries++;
    return {ok:true,status:200,json:async () => ({threads: []})};
  });
  t.after(() => h.inspect.stop());
  h.setVisibility("hidden");
  h.inspect.unreadLoop(h.session).catch(() => {});
  await new Promise(setImmediate);
  assert.equal(summaries, 0, "a hidden tab queried unread summaries");
  h.session.unreadSummaries = new Map();
  h.setVisibility("visible"); h.documentListeners.get("visibilitychange")();
  await new Promise(setImmediate);
  assert.equal(summaries, 1);
});

test("yönetici mesajı ve konuşmayı TOTP oturumuyla siler; konuşma için numarası yazılır", async (t) => {
  const calls = [];
  const h = await harness(async (path, options = {}) => {
    calls.push({path, options});
    const data = path.endsWith("/oturum") ? {oturum: "test-session", bitis: new Date(Date.now() + 600000).toISOString()} :
      path.includes("/konusmalar/") ? {ok: true, silinen: 2} : path.includes("/mesajlar/") ? {ok: true, thread: "T-1"} : {threads: []};
    return {ok: true, status: 200, json: async () => data};
  });
  t.after(() => h.inspect.stop());
  h.session.yonetici = true;
  h.inspect.addMessages([1, 2, 3].map((id) => ({id, thread: id === 1 ? "T-1" : "T-2", room: "r1", from: "other", type: "bilgi",
    body: "m" + id, created_at: "2026-10-27T00:00:00Z"})), h.session);
  const messages = h.el("mesajlar"), submit = () => h.el("silme-form").listeners.get("submit")({preventDefault() {}});
  const remove = messages.children[0].children.at(-1).children.at(-1);
  assert.equal(remove.textContent, "Sil");
  remove.listeners.get("click")();
  assert.equal(h.el("silme-dialog").open, true);
  assert.equal(h.el("silme-dogrulama").hidden, false);
  h.el("silme-kod").value = "123456";
  await submit();
  assert.equal(h.el("silme-dialog").open, false);
  assert.deepEqual(messages.children.map((row) => row.children[2].textContent), ["m2", "m3"]);
  const single = calls.find((c) => c.options.method === "DELETE");
  assert.equal(single.path, "/v1/yonetim/odalar/r1/mesajlar/1");
  assert.equal(single.options.headers["X-Yonetim-Oturum"], "test-session");

  h.inspect.showFilter("T-2");
  assert.equal(h.el("konusma-sil").hidden, false);
  h.el("konusma-sil").listeners.get("click")();
  assert.equal(h.el("silme-dogrulama").hidden, true, "open management session asked for a new code");
  h.el("silme-onay-girdi").value = "T-1";
  await submit();
  assert.equal(h.el("silme-dialog").open, true);
  assert.match(h.el("silme-hata").textContent, /T-2/);
  h.el("silme-onay-girdi").value = "T-2";
  await submit();
  assert.equal(messages.children.length, 0);
  assert.equal(h.el("filtre").textContent, "Tüm konuşmalar");
  assert.equal(calls.filter((c) => c.path === "/v1/yonetim/oturum").length, 1);
  assert.equal(calls.filter((c) => c.options.method === "DELETE").at(-1).path, "/v1/yonetim/odalar/r1/konusmalar/T-2");
});

test("yönetici olmayan oturumda silme düğmeleri görünmez", async (t) => {
  const h = await harness(async () => ({ok: true, status: 200, json: async () => ({})}));
  t.after(() => h.inspect.stop());
  h.inspect.addMessages([{id: 1, thread: "T-1", room: "r1", from: "other", type: "bilgi", body: "m1", created_at: "2026-10-27T00:00:00Z"}], h.session);
  assert.deepEqual(h.el("mesajlar").children[0].children.at(-1).children.map((b) => b.className), ["konusma", "yanitla"]);
  h.inspect.showFilter("T-1");
  assert.equal(h.el("konusma-sil").hidden, true);
});

test("son mesaj T-0 duyurusuysa yazılan mesaj yeni konuşma açar; duyuruda Yanıtla görünmez", async (t) => {
  const sent = [];
  const h = await harness(async (_path, options) => {
    sent.push(JSON.parse(options.body));
    return {ok: true, status: 201, json: async () => ({thread: "T-3"})};
  });
  t.after(() => h.inspect.stop());
  const message = (id, thread) => ({id, thread, room: "r1", from: "other", type: "bilgi", body: "m" + id, created_at: "2026-10-27T00:00:00Z"});
  h.inspect.addMessages([message(1, "T-1"), message(2, "T-0")], h.session);
  const answers = h.el("mesajlar").children.map((row) => row.children.at(-1).children[1].hidden);
  assert.deepEqual(answers, [false, true]);
  assert.equal(h.el("baglam").textContent, "Yeni konuşma başlatıyorsun");
  h.el("metin").value = "duyurudan sonra soru";
  await h.el("gonderme").listeners.get("submit")({preventDefault() {}});
  assert.equal("thread" in sent[0], false, "T-0 was used as the target");
  h.inspect.showFilter("T-0");
  assert.equal(h.el("baglam").textContent, "Duyurulara yanıt yazılmaz; yeni konuşma başlatıyorsun");
  h.el("metin").value = "duyuru görünümünden soru";
  await h.el("gonderme").listeners.get("submit")({preventDefault() {}});
  assert.equal("thread" in sent[1], false);
  assert.equal(h.el("filtre").textContent, "Konuşma: T-3");
  h.inspect.showFilter("T-1");
  assert.equal(h.el("baglam").textContent, "T-1 konuşmasına yazıyorsun");
});

test("bir odadan gelen 403 oturumu kapatmaz; oda listesi yenilenir, aynı oda yeniden reddedilirse çıkış yapılır", async (t) => {
  let rooms = [{name: "r2", title: "İki"}];
  const calls = [];
  const h = await harness(async (path) => {
    calls.push(path);
    if (path === "/v1/rooms") return {ok: true, status: 200, json: async () => ({rooms})};
    if (path.startsWith("/v1/rooms/r1/")) return {ok: false, status: 403, json: async () => ({error: "yasak"})};
    if (path === "/v1/rooms/r2/log?tail=1") return {ok: true, status: 200, json: async () => ({messages: [], has_more: false})};
    return new Promise(() => {});
  });
  t.after(() => h.inspect.stop());
  h.session.rooms = [{name: "r1", title: "Bir"}, ...rooms];
  h.el("oda").value = "r1"; h.el("oda").listeners.get("change")();
  for (let i = 0; i < 5; i++) await new Promise(setImmediate);
  assert.equal(h.el("izleme").hidden, false, "403 logged the session out");
  assert.equal(h.session.room, "r2");
  assert.equal(h.el("gonderim-durum").textContent, "Bir odasına erişimin kalmadı; oda listesi yenilendi.");
  assert.ok(calls.includes("/v1/rooms"));

  // Liste reddedilen odayı yine verirse yeniden yükleme döngüsü yerine çıkış yapılır.
  rooms = [{name: "r1", title: "Bir"}];
  h.el("oda").value = "r1"; h.el("oda").listeners.get("change")();
  for (let i = 0; i < 10; i++) await new Promise(setImmediate);
  assert.equal(h.el("izleme").hidden, true);
  assert.equal(h.el("durum").textContent, "Bu odaya erişimin yok.");
  assert.ok(calls.filter((p) => p === "/v1/rooms").length <= 3, "room list reloaded in a loop");
});

test("okunmamış sayısı noktanın yanında görünür; çizgi odaya girişte okunmamış ilk mesajın üstüne konur ve odada kalır", async (t) => {
  const reads = [];
  const h = await harness(async (path, options = {}) => {
    if (path.endsWith("/read")) { reads.push(JSON.parse(options.body).last_id); return {ok: true, status: 200, json: async () => ({ok: true})}; }
    return {ok: true, status: 200, json: async () => ({threads: [{room: "r1", thread: "T-1", title: "m1", total: 4, last_id: 4, last_incoming_id: 4, read_id: 2, unread: 2}]})};
  });
  t.after(() => h.inspect.stop());
  const message = (id) => ({id, thread: "T-1", room: "r1", from: "other", type: "bilgi", body: "m" + id, created_at: "2026-10-27T00:00:00Z"});
  h.inspect.addMessages([1, 2, 3, 4].map(message), h.session);
  await h.inspect.refreshUnread(h.session);
  const messages = h.el("mesajlar"), button = () => h.el("konusma-listesi").children[0];
  const badge = () => button().children[1].children[0].children[1];
  const dividerAt = () => messages.children.findIndex((n) => n.className === "yeni-ayirac");
  assert.equal(badge().hidden, false);
  assert.equal(badge().textContent, "2 yeni");
  assert.equal(dividerAt(), 2, "the divider must sit above the first unread message (#3)");
  h.inspect.showFilter("T-1");
  await new Promise(setImmediate); await new Promise(setImmediate);
  assert.deepEqual(reads, [4]);
  assert.equal(badge().hidden, true);
  assert.equal(button().children[0].hidden, true);
  assert.equal(dividerAt(), 2, "the read write removed the divider from the open conversation");
  // Çizgi odadan çıkana kadar "girdiğinden beri yeni" olanı göstermeye devam eder.
  h.inspect.showFilter(""); h.inspect.showFilter("T-1");
  assert.equal(dividerAt(), 2, "switching views lost the divider before leaving the room");
  h.inspect.showFilter("");
  h.inspect.addMessages([message(5)], h.session);
  assert.equal(badge().textContent, "1 yeni");
});

test("sayı vermeyen eski sunucuda yalnız nokta görünür", async (t) => {
  const h = await harness(async () => ({ok: true, status: 200, json: async () => ({threads: [{room: "r1", thread: "T-1", title: "m1", total: 1, last_id: 1, last_incoming_id: 1, read_id: 0}]})}));
  t.after(() => h.inspect.stop());
  h.inspect.addMessages([{id: 1, thread: "T-1", room: "r1", from: "other", type: "bilgi", body: "m1", created_at: "2026-10-27T00:00:00Z"}], h.session);
  await h.inspect.refreshUnread(h.session);
  const button = h.el("konusma-listesi").children[0];
  assert.equal(button.children[0].hidden, false);
  assert.equal(button.children[1].children[0].children[1].hidden, true);
});

test("okunmamış özeti günlükten önce gelse de çizgi satırlar gelince konur", async (t) => {
  const h = await harness(async () => ({ok: true, status: 200, json: async () => ({threads: [{room: "r1", thread: "T-1", title: "m1", total: 2, last_id: 2, last_incoming_id: 2, read_id: 1, unread: 1}]})}));
  t.after(() => h.inspect.stop());
  await h.inspect.refreshUnread(h.session);
  h.inspect.addMessages([1, 2].map((id) => ({id, thread: "T-1", room: "r1", from: "other", type: "bilgi", body: "m" + id, created_at: "2026-10-27T00:00:00Z"})), h.session);
  assert.equal(h.el("mesajlar").children.findIndex((n) => n.className === "yeni-ayirac"), 1);
});
