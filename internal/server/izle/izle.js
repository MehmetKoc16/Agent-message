"use strict";
(() => {
  const el = (id) => document.getElementById(id);
  const form = el("giris"), view = el("izleme"), status = el("durum");
  const messages = el("mesajlar"), text = el("metin"), recipient = el("alici");
  const tokenKey = "relay-izle-token", nameKey = "relay-izle-ad";
  let active = null, filter = "", target = "", targetExplicit = false, reply = null, rows = [], ids = new Set();
  const preferredRoom = new URLSearchParams(location.search).get("oda");
  const threads = new Map();
  const narrow = matchMedia("(max-width: 1150px)");
  function panel(open, focus = false) {
    el("katilimci-panel").hidden = !open;
    view.classList.toggle("bilgi-kapali", !open);
    el("katilimcilar-ac").setAttribute("aria-expanded", String(open));
    if (focus) (open ? el("katilimcilar-kapat") : el("katilimcilar-ac")).focus();
  }
  function menu(open, focus = false) {
    view.classList.toggle("menu-acik", open);
    el("menu-perde").hidden = !open;
    el("menu-ac").setAttribute("aria-expanded", String(open));
    if (focus) (open ? el("menu-kapat") : el("menu-ac")).focus();
  }
  el("menu-ac").addEventListener("click", () => { panel(false); menu(true, true); });
  el("menu-kapat").addEventListener("click", () => menu(false, true));
  el("menu-perde").addEventListener("click", () => menu(false, true));
  el("katilimcilar-ac").addEventListener("click", () => { menu(false); panel(el("katilimci-panel").hidden, true); });
  el("katilimcilar-kapat").addEventListener("click", () => panel(false, true));
  narrow.addEventListener("change", () => { if (active && active.room) panel(!narrow.matches); });
  matchMedia("(max-width: 760px)").addEventListener("change", () => menu(false));
  document.addEventListener("keydown", (event) => {
    const drawer = view.classList.contains("menu-acik") ? el("gezinme") :
      narrow.matches && !el("katilimci-panel").hidden ? el("katilimci-panel") : null;
    if (event.key === "Tab" && drawer) {
      const controls = [...drawer.querySelectorAll("button:not(:disabled), select, a[href], [tabindex='0']")]
        .filter((n) => n.getClientRects().length > 0);
      const first = controls[0], last = controls[controls.length - 1];
      if (!controls.includes(document.activeElement) || (!event.shiftKey && document.activeElement === last)) {
        event.preventDefault(); first?.focus();
      } else if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    }
    if (event.key !== "Escape") return;
    if (view.classList.contains("menu-acik")) menu(false, true);
    else if (narrow.matches && !el("katilimci-panel").hidden) panel(false, true);
  });
  try {
    const theme = sessionStorage.getItem("relay-izle-tema");
    if (theme === "acik" || theme === "koyu") document.documentElement.dataset.tema = theme;
  } catch (_) { /* Tema depolama olmadan da değişir. */ }
  el("tema").addEventListener("click", () => {
    const current = document.documentElement.dataset.tema;
    const dark = current ? current === "koyu" : matchMedia("(prefers-color-scheme: dark)").matches;
    const theme = dark ? "acik" : "koyu";
    document.documentElement.dataset.tema = theme;
    try { sessionStorage.setItem("relay-izle-tema", theme); } catch (_) { /* Bellekte kalır. */ }
  });
  function node(tag, content, className) {
    const n = document.createElement(tag);
    if (content !== undefined) n.textContent = content;
    if (className) n.className = className;
    return n;
  }
  function accountAvatar(name, className) {
    const avatar = node("span", name.slice(0, 2).toLocaleUpperCase("tr-TR"), className);
    avatar.setAttribute("aria-hidden", "true");
    avatar.dataset.renk = String(Array.from(name).reduce((sum, c) => sum + c.charCodeAt(0), 0) % 2);
    return avatar;
  }
  function remember(token, name) {
    try {
      if (token) {
        sessionStorage.setItem(tokenKey, token); sessionStorage.setItem(nameKey, name);
      } else {
        sessionStorage.removeItem(tokenKey); sessionStorage.removeItem(nameKey);
      }
    } catch (_) { /* Depolama kapalıysa oturum bellekte kalır. */ }
  }
  function stop() {
    status.classList.remove("canli");
    clearYonetim(); el("yonetim").hidden = true; el("yonetim-ac").hidden = true;
    if (active) {
      active.controller.abort();
      if (active.roomController) active.roomController.abort();
      active.token = "";
    }
    active = null;
    menu(false); panel(false);
  }
  function resetRoom() {
    rows = []; ids = new Set(); target = ""; targetExplicit = false; messages.replaceChildren(); el("ajanlar").replaceChildren();
    threads.clear(); el("konusma-listesi").replaceChildren();
    el("konusma-sayisi").textContent = "0"; el("konusma-bos").hidden = false;
    el("katilimci-sayisi").textContent = "0";
    recipient.replaceChildren(node("option", "herkes")); recipient.firstChild.value = "";
    text.value = ""; count(); showFilter(""); el("yeni").hidden = true;
    el("gonderim-durum").textContent = ""; el("gonder").disabled = false; el("kapat").disabled = false;
  }
  function logout(message) {
    stop(); remember("", ""); el("token").value = ""; resetRoom();
    view.hidden = true; form.hidden = false; el("giris-dugme").disabled = false;
    status.textContent = message;
    el("giris-ekrani").hidden = false; document.body.classList.remove("oturum-acik");
  }
  el("cikis").addEventListener("click", () => logout("Çıkış yapıldı."));
  function bottom() { return messages.scrollHeight - messages.scrollTop - messages.clientHeight < 48; }
  function scrollDown() { messages.scrollTop = messages.scrollHeight; el("yeni").hidden = true; }
  el("yeni").addEventListener("click", scrollDown);
  messages.addEventListener("scroll", () => { if (bottom()) el("yeni").hidden = true; });
  function clearReply() { reply = null; el("yanit").hidden = true; }
  function currentTarget() {
    if (targetExplicit) return "";
    if (filter) return filter;
    if (target) return target;
    return rows.length ? rows[rows.length - 1].thread : "";
  }
  function updateBaglam() {
    const t = currentTarget();
    el("baglam").textContent = t ? t + " konuşmasına yazıyorsun" : "Yeni konuşma başlatıyorsun";
    el("yeni-konusma").hidden = false;
    el("yeni-konusma").disabled = !active || !active.room;
  }
  function showFilter(thread) {
    filter = thread; targetExplicit = false; clearReply();
    el("filtre").textContent = thread ? "Konuşma: " + thread : "Tüm konuşmalar";
    el("kapat").hidden = !thread;
    for (const row of rows) row.element.hidden = !!thread && row.thread !== thread;
    refreshContext();
    if (view.classList.contains("menu-acik")) { menu(false); el("menu-ac").focus(); }
    updateBaglam();
    scrollDown();
  }
  function refreshContext() {
    const visible = rows.filter((row) => !filter || row.thread === filter);
    const count = visible.length + " mesaj";
    el("mesaj-sayisi").textContent = count; el("ozet-mesaj").textContent = count;
    el("bos-durum").hidden = visible.length > 0;
    el("ozet-baslik").textContent = filter || "Tüm konuşmalar";
    el("ozet-aciklama").textContent = filter && threads.has(filter) ? threads.get(filter).title :
      "Odadaki mesajları bir arada görüyorsun. Bir konuşma seçerek odağını daralt.";
    el("tumu").classList.toggle("secili", !filter);
    el("tumu").setAttribute("aria-pressed", String(!filter));
    for (const [id, thread] of threads) {
      thread.button.classList.toggle("secili", id === filter);
      thread.button.setAttribute("aria-pressed", String(id === filter));
    }
  }
  function updateThread(m) {
    let thread = threads.get(m.thread);
    if (!thread) {
      const button = node("button", undefined, "konusma-satir"); button.type = "button";
      const top = node("span", undefined, "konusma-satir-ust");
      const count = node("span"); top.append(node("span", m.thread), count);
      const title = m.body.split("\n").find((s) => s.trim()) || "Yeni konuşma";
      button.append(top, node("span", title, "konusma-satir-baslik"));
      button.title = title; button.addEventListener("click", () => showFilter(m.thread));
      thread = {button, count, title, total: 0}; threads.set(m.thread, thread);
      el("konusma-listesi").prepend(button);
    }
    thread.total++;
    thread.count.textContent = m.type === "kapat" ? "Kapalı" : thread.total + " mesaj";
    el("konusma-sayisi").textContent = String(threads.size);
    el("konusma-bos").hidden = true;
  }
  el("tumu").addEventListener("click", () => showFilter(""));
  el("yeni-konusma").addEventListener("click", () => {
    showFilter(""); target = ""; targetExplicit = true; updateBaglam(); text.focus();
  });
  el("yanit-sil").addEventListener("click", clearReply);
  function addMessages(list, session) {
    let wasBottom = bottom();
    let visibleAdded = false;
    for (const m of list) {
      if (ids.has(m.id)) continue;
      ids.add(m.id);
      if (m.from === session.name) wasBottom = true;
      const row = node("article", undefined, "mesaj" + (m.from === session.name ? " kendi" : ""));
      const avatar = accountAvatar(m.from, "mesaj-avatar");
      const meta = node("div", undefined, "meta");
      meta.append(node("span", m.from, "gonderen"), node("span", "→ " + (m.to || "herkes"), "alici-bilgi"));
      const types = ["soru", "gorev", "cevap", "sonuc", "bilgi", "kapat"];
      const typeNames = {soru: "Soru", gorev: "Görev", cevap: "Cevap", sonuc: "Sonuç", bilgi: "Bilgi", kapat: "Kapalı"};
      meta.append(node("span", typeNames[m.type] || m.type, "etiket " + (types.includes(m.type) ? m.type : "bilgi")));
      const stamp = new Date(m.created_at);
      const time = node("time", stamp.toLocaleString("tr-TR", {day: "numeric", month: "short", hour: "2-digit", minute: "2-digit"}), "zaman");
      time.dateTime = m.created_at; time.title = stamp.toLocaleString("tr-TR"); meta.append(time);
      const thread = node("button", m.thread + " · " + m.turn + "/" + m.max_turns, "konusma");
      thread.type = "button"; thread.addEventListener("click", () => showFilter(m.thread));
      const answer = node("button", "Yanıtla", "yanitla"); answer.type = "button";
      answer.addEventListener("click", () => {
        target = m.thread; targetExplicit = false;
        showFilter(m.thread); reply = m.id;
        if (m.from !== session.name) {
          if (!Array.from(recipient.options).some((o) => o.value === m.from)) {
            const option = node("option", m.from); option.value = m.from; recipient.append(option);
          }
          recipient.value = m.from;
        }
        if (m.type === "soru") el("tip").value = "cevap";
        if (m.type === "gorev") el("tip").value = "sonuc";
        el("yanit-bilgi").textContent = "#" + m.id + " yanıtlanıyor";
        el("yanit").hidden = false; text.focus();
      });
      const actions = node("div", undefined, "mesaj-islemler"); actions.append(thread, answer);
      row.append(avatar, meta, node("div", m.body, "govde"), actions);
      row.hidden = !!filter && filter !== m.thread;
      visibleAdded = visibleAdded || !row.hidden;
      rows.push({thread: m.thread, element: row}); messages.append(row);
      updateThread(m);
    }
    if (rows.length && !targetExplicit && !filter) target = rows[rows.length - 1].thread;
    updateBaglam();
    refreshContext();
    if (wasBottom) scrollDown(); else if (visibleAdded) el("yeni").hidden = false;
  }
  function pause(ms, signal) {
    return new Promise((resolve, reject) => {
      const cancel = () => { clearTimeout(timer); reject(new Error("durdu")); };
      const timer = setTimeout(() => { signal.removeEventListener("abort", cancel); resolve(); }, ms);
      signal.addEventListener("abort", cancel, {once: true});
      if (signal.aborted) cancel();
    });
  }
  async function api(session, path, signal, options = {}) {
    const response = await fetch(path, {
      ...options, headers: {Authorization: "Bearer " + session.token, ...(options.headers || {})},
      cache: "no-store", credentials: "omit", signal, redirect: "error"
    });
    const data = await response.json();
    if (signal.aborted) throw new Error("durdu");
    return {response, data};
  }
  function roomPath(session) { return "/v1/rooms/" + encodeURIComponent(session.room); }
  async function request(session, suffix, signal) {
    let backoff = 2;
    while (!signal.aborted) {
      try {
        const {response, data} = await api(session, roomPath(session) + suffix, signal);
        if (response.status === 401) { logout("Ad ya da parola hatalı."); throw new Error("durdu"); }
        if (response.status === 403) { logout("Bu odaya erişimin yok."); throw new Error("durdu"); }
        if (response.status === 429) {
          status.classList.remove("canli");
          status.textContent = "İstek sınırı: 30 saniye bekleniyor.";
          await pause(30000, signal); continue;
        }
        if (!response.ok) throw new Error("istek");
        return data;
      } catch (error) {
        if (signal.aborted) throw error;
        status.classList.remove("canli");
        status.textContent = "Bağlantı kesildi; " + backoff + " saniye sonra yeniden denenecek.";
        await pause(backoff * 1000, signal); backoff = Math.min(60, backoff * 2);
      }
    }
    throw new Error("durdu");
  }
  async function logLoop(session, signal) {
    let after = 0, history = true;
    while (!signal.aborted) {
      const data = await request(session, "/log?after=" + after + "&wait=" + (history ? 0 : 25), signal);
      if (signal.aborted) return;
      addMessages(data.messages, session);
      for (const m of data.messages) after = Math.max(after, m.id);
      if (history && data.messages.length < 100) history = false;
      status.textContent = history ? "Geçmiş yükleniyor…" : "Canlı izleniyor";
      status.classList.toggle("canli", !history);
    }
  }
  async function agentsLoop(session, signal) {
    while (!signal.aborted) {
      const data = await request(session, "/agents", signal);
      if (signal.aborted) return;
      el("ajanlar").replaceChildren();
      const selected = recipient.value;
      recipient.replaceChildren(node("option", "herkes")); recipient.firstChild.value = "";
      for (const a of data.agents) {
        const li = node("li", undefined, "katilimci");
        const avatar = accountAvatar(a.name, "mini-avatar");
        const content = node("div", undefined, "katilimci-icerik");
        const seen = a.last_seen ? new Date(a.last_seen).toLocaleString("tr-TR") : "Henüz görülmedi";
        const label = node("span", a.waiting ? "Mesaj bekliyor" : (a.name === session.name ? "Sen" : "Beklemede değil"), "katilimci-durum" + (a.waiting ? " bekliyor" : ""));
        label.title = "Son görülme: " + seen;
        content.append(node("span", a.name, "katilimci-isim"), label);
        li.append(avatar, content, node("span", a.kind === "insan" ? "İnsan" : "Ajan", "katilimci-tur"));
        el("ajanlar").append(li);
        if (a.name !== session.name) {
          const option = node("option", a.name); option.value = a.name; recipient.append(option);
        }
      }
      el("katilimci-sayisi").textContent = String(data.agents.length);
      if (Array.from(recipient.options).some((o) => o.value === selected)) recipient.value = selected;
      await pause(30000, signal);
    }
  }
  function openRoom(session, room) {
    if (session.roomController) session.roomController.abort();
    status.classList.remove("canli"); status.textContent = "Oda yükleniyor…";
    resetRoom(); session.room = room; session.roomController = new AbortController();
    el("oda-baslik").textContent = room; el("tek-oda-ad").textContent = room;
    el("ozet-oda").textContent = room; el("odasiz-durum").hidden = true;
    menu(false); panel(!narrow.matches);
    el("oda-icerik").hidden = false;
    const signal = session.roomController.signal;
    logLoop(session, signal).catch(() => {}); agentsLoop(session, signal).catch(() => {});
  }
  el("oda").addEventListener("change", () => { if (active) openRoom(active, el("oda").value); });
  async function login(name, token) {
    stop(); remember("", "");
    const session = {name, token, controller: new AbortController()}; active = session;
    const signal = session.controller.signal;
    el("giris-dugme").disabled = true; status.textContent = "Giriş denetleniyor…";
    try {
      const {response, data} = await api(session, "/v1/me", signal);
      if (response.status === 401 || (response.ok && data.name !== name)) {
        logout("Ad ya da parola hatalı."); return;
      }
      if (!response.ok) { logout("Giriş yapılamadı; biraz sonra tekrar dene."); return; }
      if (data.kind !== "insan") { logout("Bu sayfa yalnız insan hesapları içindir."); return; }
      remember(token, name); el("token").value = "";
      el("yonetim-ac").hidden = !data.yonetici;
      form.hidden = true; view.hidden = false; el("giris-dugme").disabled = false;
      el("giris-ekrani").hidden = true; document.body.classList.add("oturum-acik");
      el("kullanici-ad").textContent = name;
      el("kullanici-avatar").textContent = name.slice(0, 2).toLocaleUpperCase("tr-TR");
      el("oda-baslik").textContent = name;
      el("oda").replaceChildren(); el("oda-secimi").hidden = data.rooms.length < 2;
      el("tek-oda").hidden = data.rooms.length !== 1;
      el("oda-icerik").hidden = true;
      for (const room of data.rooms) { const option = node("option", room); option.value = room; el("oda").append(option); }
      el("odasiz-durum").hidden = data.rooms.length > 0;
      if (!data.rooms.length) { status.textContent = "Bu hesabın odası yok."; return; }
      el("oda").value = data.rooms.includes(preferredRoom) ? preferredRoom : data.rooms[0];
      openRoom(session, el("oda").value);
    } catch (_) { if (!signal.aborted) logout("Giriş yapılamadı; bağlantını denetle."); }
  }
  form.addEventListener("submit", (event) => {
    event.preventDefault(); login(el("ad").value.trim(), el("token").value.trim());
  });
  // Yönetim anahtarı yalnız bu kapanımda yaşar; tarayıcı depolarına aktarılmaz.
  let managementSession = "", managementExpiry = 0, managementTimer = null;
  function clearYonetim() {
    managementSession = ""; managementExpiry = 0; clearTimeout(managementTimer);
    el("yonetim-kod").value = ""; el("yonetim-parola").replaceChildren();
    el("yonetim-hesaplar").replaceChildren(); el("yonetim-kayit").replaceChildren();
    el("yonetim-kod-form").hidden = false; el("yonetim-icerik").hidden = true;
  }
  const managementErrors = {
    yalniz_yonetici: "Bu işlem yalnız yönetici hesabına açık.",
    kod_gecersiz: "Kod geçersiz veya daha önce kullanılmış. Yeni kodu bekle.",
    cok_fazla_deneme: "Çok fazla hatalı kod. 15 dakika sonra tekrar dene.",
    yonetim_oturumu_gerekli: "Yönetim oturumu sona erdi. Yeni kod gir.",
    yoneticiye_dokunulamaz: "Yönetici hesabı yalnız SSH ile değiştirilebilir.",
    zaten_var: "Bu adla bir hesap zaten var.", hesap_yok: "Hesap yok veya iptal edilmiş.",
    gecersiz_ad: "Geçersiz hesap adı.", gecersiz_oda: "Geçersiz oda adı; en az bir oda gerekli.",
    gecersiz_tur: "Tür ajan veya insan olmalı.", gecersiz_son: "Kayıt sayısı 1–1000 olmalı.",
    gecersiz_json: "İstek biçimi geçersiz.", gecersiz_govde: "İstek metni geçersiz.",
    govde_cok_buyuk: "İstek çok büyük.", hiz_siniri: "İstek sınırına ulaşıldı; biraz bekle.",
    cok_fazla_baglanti: "Çok fazla eşzamanlı bağlantı; yeniden dene.",
    kimliksiz_hiz_siniri: "Giriş sınırına ulaşıldı; biraz bekle.", yetkisiz: "Parola geçersiz veya hesap iptal edilmiş.",
    sunucu_hatasi: "Sunucu işlemi tamamlayamadı."
  };
  async function managementAPI(path, method = "GET", body) {
    const session = active;
    const key = managementSession;
    if (!session) throw new Error("Önce giriş yap.");
    if (path !== "/oturum" && Date.now() >= managementExpiry) {
      clearYonetim(); throw new Error(managementErrors.yonetim_oturumu_gerekli);
    }
    const {response, data} = await api(session, "/v1/yonetim" + path, session.controller.signal, {
      method, headers: {"Content-Type": "application/json", "X-Yonetim-Oturum": managementSession},
      ...(body === undefined ? {} : {body: JSON.stringify(body)})
    });
    if (active !== session) throw new Error("Oturum değişti.");
    if (path !== "/oturum" && (key !== managementSession || Date.now() >= managementExpiry)) {
      throw new Error(managementErrors.yonetim_oturumu_gerekli);
    }
    if (!response.ok) {
      if (data.error === "yonetim_oturumu_gerekli" || data.error === "yalniz_yonetici") clearYonetim();
      if (data.error === "yetkisiz") logout("Hesap iptal edilmiş veya parola geçersiz.");
      throw new Error(managementErrors[data.error] || "İşlem tamamlanamadı.");
    }
    return data;
  }
  function managementAction(fn) {
    return async () => { try { await fn(); } catch (error) { el("yonetim-durum").textContent = error.message; } };
  }
  function managementButton(label, fn) {
    const b = node("button", label); b.type = "button"; b.addEventListener("click", managementAction(async () => {
      b.disabled = true; try { await fn(); } finally { b.disabled = false; }
    })); return b;
  }
  async function refreshManagement() {
    const accounts = await managementAPI("/hesaplar");
    const list = el("yonetim-hesaplar"); list.replaceChildren();
    for (const a of accounts) {
      const card = node("article", undefined, "yonetim-hesap");
      card.append(node("h4", a.name + " · " + a.kind + (a.yonetici ? " · yönetici" : "")),
        node("p", (a.revoked ? "İptal edilmiş" : "Aktif") + " · Son görülme: " +
          (a.last_seen ? new Date(a.last_seen).toLocaleString("tr-TR") : "Henüz görülmedi")));
      const base = "/hesaplar/" + encodeURIComponent(a.name);
      const rooms = node("ul");
      for (const room of a.rooms) {
        const li = node("li", room + " ");
        if (!a.yonetici) li.append(managementButton("çıkar", async () => {
          await managementAPI(base + "/odalar/" + encodeURIComponent(room), "DELETE"); await refreshManagement();
        })); rooms.append(li);
      }
      card.append(rooms);
      if (!a.yonetici && !a.revoked) {
        card.append(managementButton("Oda ekle", async () => {
          const room = window.prompt(a.name + " için oda adı"); if (room === null) return;
          await managementAPI(base + "/odalar", "POST", {oda: room.trim()}); await refreshManagement();
        }), managementButton("İptal et", async () => {
          if (!window.confirm(a.name + " hesabı iptal edilsin mi?")) return;
          await managementAPI(base + "/iptal", "POST"); await refreshManagement();
        }));
      }
      list.append(card);
    }
    const records = await managementAPI("/kayit?son=100");
    el("yonetim-kayit").replaceChildren();
    for (const k of records) el("yonetim-kayit").append(node("li",
      new Date(k.zaman).toLocaleString("tr-TR") + " · " + k.yapan + " · " + k.islem + " · " + k.hedef + " · " + k.ayrinti));
    el("yonetim-durum").textContent = "Yönetim oturumu 10 dakika geçerlidir.";
  }
  el("yonetim-ac").addEventListener("click", managementAction(async () => {
    view.hidden = true; el("yonetim").hidden = false;
    if (managementSession && Date.now() < managementExpiry) await refreshManagement();
    else { clearYonetim(); el("yonetim-kod").focus(); }
  }));
  el("sohbete-don").addEventListener("click", () => {
    el("yonetim").hidden = true; view.hidden = false; el("yonetim-parola").replaceChildren();
  });
  el("yonetim-yenile").addEventListener("click", managementAction(refreshManagement));
  el("yonetim-kod-form").addEventListener("submit", (event) => {
    event.preventDefault(); const submit = event.submitter; submit.disabled = true;
    managementAction(async () => {
      try {
      const kod = el("yonetim-kod").value; el("yonetim-kod").value = "";
      const data = await managementAPI("/oturum", "POST", {kod});
      managementSession = data.oturum; managementExpiry = Date.parse(data.bitis);
      managementTimer = setTimeout(() => {
        clearYonetim(); el("yonetim-durum").textContent = managementErrors.yonetim_oturumu_gerekli;
      }, Math.max(0, managementExpiry - Date.now()));
      el("yonetim-kod-form").hidden = true; el("yonetim-icerik").hidden = false; await refreshManagement();
      } finally { submit.disabled = false; }
    })();
  });
  el("yonetim-yeni").addEventListener("submit", (event) => {
    event.preventDefault(); const submit = event.submitter;
    managementAction(async () => {
      submit.disabled = true;
      try {
        const data = await managementAPI("/hesaplar", "POST", {ad: el("yeni-ad").value.trim(),
          tur: el("yeni-tur").value, odalar: el("yeni-odalar").value.split(",").map((s) => s.trim())});
        const box = node("section", undefined, "yonetim-token");
        box.append(node("p", data.ad + ": Bu parola bir daha gösterilmeyecek; kişiye güvenli kanaldan ilet"));
        const secret = node("code", data.token); data.token = "";
        box.append(secret, managementButton("Kopyala", async () => {
          await navigator.clipboard.writeText(secret.textContent);
        }), managementButton("Kapat", async () => { secret.textContent = ""; box.remove(); }));
        el("yonetim-parola").replaceChildren(box); el("yonetim-yeni").reset(); await refreshManagement();
      } finally { submit.disabled = false; }
    })();
  });
  function count() { el("sayac").textContent = text.value.length + " / 16000"; }
  text.addEventListener("input", count);
  text.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.isComposing) {
      if (event.ctrlKey || event.metaKey || !event.shiftKey) {
        event.preventDefault(); el("gonderme").requestSubmit();
      }
    }
  });
  function errorText(response, data) {
    if (response.status === 400 && /^gecersiz_/.test(data.error)) return "Geçersiz istek (" + data.error + ")";
    if (response.status === 403) return "Bu konuşmaya yazma yetkin yok";
    if (response.status === 404 && data.error === "konusma_yok") return "Konuşma bulunamadı";
    if (response.status === 409) return "Konuşma kapalı ya da tur sınırına ulaştı";
    if (response.status === 413) return "Mesaj çok uzun";
    if (response.status === 429) return "Çok hızlı gönderiyorsun; biraz bekle";
    return "İstek tamamlanamadı (" + response.status + ")";
  }
  el("gonderme").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!active || !active.room || el("gonder").disabled || !text.value.trim()) return;
    const session = active, signal = session.roomController.signal;
    const curTarget = currentTarget(), original = text.value, originalReply = reply;
    const canRetry = !filter && !targetExplicit && reply === null;
    const body = {to: recipient.value, type: el("tip").value, body: original};
    if (curTarget) body.thread = curTarget;
    if (reply !== null) body.reply_to = reply;
    el("gonder").disabled = true; el("gonderim-durum").textContent = "Gönderiliyor…";
    try {
      let {response, data} = await api(session, roomPath(session) + "/messages", signal, {
        method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(body)
      });
      if (response.status === 401) { logout("Ad ya da parola hatalı."); return; }
      let retried = false;
      if (response.status === 409 && canRetry) {
        const retryBody = {to: recipient.value, type: el("tip").value, body: original};
        const retry = await api(session, roomPath(session) + "/messages", signal, {
          method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(retryBody)
        });
        response = retry.response;
        data = retry.data;
        retried = true;
        if (response.status === 401) { logout("Ad ya da parola hatalı."); return; }
      }
      if (response.status !== 201) { el("gonderim-durum").textContent = errorText(response, data); return; }
      if (text.value === original) { text.value = ""; count(); }
      if (reply === originalReply) clearReply();
      // Süzülmüş görünümde "Yeni konuşma" ile yazılan mesaj gizli kalmasın.
      if (filter && filter !== data.thread) showFilter(data.thread);
      target = data.thread; targetExplicit = false;
      updateBaglam(); scrollDown();
      el("gonderim-durum").textContent = retried ? "Önceki konuşma kapalıydı; yeni konuşma açıldı." : "Mesaj gönderildi.";
    } catch (_) {
      if (!signal.aborted) el("gonderim-durum").textContent = "Gönderim doğrulanamadı; tekrar denemeden önce geçmişi denetle.";
    } finally { if (!signal.aborted) el("gonder").disabled = false; }
  });
  el("kapat").addEventListener("click", async () => {
    if (!active || !filter || el("kapat").disabled || !confirm(filter + " konuşması kapatılsın mı?")) return;
    const session = active, signal = session.roomController.signal, thread = filter;
    el("kapat").disabled = true;
    try {
      const {response, data} = await api(session, roomPath(session) + "/threads/" + encodeURIComponent(thread) + "/close", signal, {method: "POST"});
      if (response.status === 401) { logout("Ad ya da parola hatalı."); return; }
      el("gonderim-durum").textContent = response.ok ? thread + " konuşması kapatıldı." : errorText(response, data);
    } catch (_) { if (!signal.aborted) el("gonderim-durum").textContent = "Konuşma kapatılamadı; bağlantını denetle."; }
    finally { if (!signal.aborted) el("kapat").disabled = false; }
  });
  let savedToken = "", savedName = "";
  try { savedToken = sessionStorage.getItem(tokenKey) || ""; savedName = sessionStorage.getItem(nameKey) || ""; } catch (_) { /* Bellekte kalır. */ }
  if (savedToken && savedName) { el("ad").value = savedName; login(savedName, savedToken); }
  else remember("", "");
})();
