"use strict";
(() => {
  const el = (id) => document.getElementById(id);
  const form = el("giris"), view = el("izleme"), status = el("durum");
  const messages = el("mesajlar"), text = el("metin"), recipient = el("alici");
  const tokenKey = "relay-izle-token", nameKey = "relay-izle-ad";
  let active = null, filter = "", target = "", targetExplicit = false, reply = null, rows = [], ids = new Set();
  const preferredRoom = new URLSearchParams(location.search).get("oda");
  const threads = new Map();
  const maxRows = 500, encoder = new TextEncoder();
  const narrow = matchMedia("(max-width: 1150px)");
  function navigationButton() { return el("oda-icerik").hidden ? el("odasiz-menu-ac") : el("menu-ac"); }
  function drawerBackdrop() {
    el("menu-perde").hidden = !view.classList.contains("menu-acik") &&
      !(narrow.matches && !el("katilimci-panel").hidden);
  }
  function panel(open, focus = false) {
    el("katilimci-panel").hidden = !open;
    view.classList.toggle("bilgi-kapali", !open);
    el("katilimcilar-ac").setAttribute("aria-expanded", String(open));
    drawerBackdrop();
    if (focus) (open ? el("katilimcilar-kapat") : el("katilimcilar-ac")).focus();
  }
  function menu(open, focus = false) {
    view.classList.toggle("menu-acik", open);
    drawerBackdrop();
    el("menu-ac").setAttribute("aria-expanded", String(open));
    el("odasiz-menu-ac").setAttribute("aria-expanded", String(open));
    if (focus) (open ? el("menu-kapat") : navigationButton()).focus();
  }
  el("menu-ac").addEventListener("click", () => { panel(false); menu(true, true); });
  el("odasiz-menu-ac").addEventListener("click", () => { panel(false); menu(true, true); });
  el("menu-kapat").addEventListener("click", () => menu(false, true));
  el("menu-perde").addEventListener("click", () => {
    if (view.classList.contains("menu-acik")) menu(false, true);
    else panel(false, true);
  });
  el("katilimcilar-ac").addEventListener("click", () => { menu(false); panel(el("katilimci-panel").hidden, true); });
  el("katilimcilar-kapat").addEventListener("click", () => panel(false, true));
  narrow.addEventListener("change", () => { if (active && active.room) panel(!narrow.matches); });
  matchMedia("(max-width: 760px)").addEventListener("change", () => menu(false));
  document.addEventListener("keydown", (event) => {
    if (view.hidden) return;
    const drawer = view.classList.contains("menu-acik") ? el("gezinme") :
      narrow.matches && !el("katilimci-panel").hidden ? el("katilimci-panel") : null;
    if (event.key === "Tab" && drawer) {
      const controls = [...drawer.querySelectorAll("button:not(:disabled), select, a[href], [tabindex='0']")]
        .filter((n) => n.getClientRects().length > 0);
      const first = controls[0], last = controls[controls.length - 1];
      if (controls.length && (!controls.includes(document.activeElement) || (!event.shiftKey && document.activeElement === last))) {
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
  function accountAvatar(name, className, avatar = node("span")) {
    const parts = name.split("-").filter(Boolean);
    const initials = parts.length > 1 ? parts[0][0] + parts[parts.length - 1][0] : name.slice(0, 2);
    const tone = Array.from(name).reduce((sum, c) => sum + c.codePointAt(0), 0) % 8;
    avatar.textContent = initials.toLocaleUpperCase("tr-TR");
    avatar.className = className + " avatar-ton-" + tone;
    avatar.setAttribute("aria-hidden", "true");
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
    el("oda-dialog").close(); el("uyelik-dialog").close(); el("uyeleri-yonet").hidden = true; el("oda-ekle").hidden = true;
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
    selectRecipient("");
    text.value = ""; count(); showFilter(""); el("yeni").hidden = true;
    el("gonderim-durum").textContent = ""; el("gonder").disabled = false; el("kapat").disabled = false;
    el("gecmis").hidden = true; el("guncel").hidden = true; el("gecmis").disabled = false;
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
  el("yeni").addEventListener("click", () => {
    if (active?.history?.browsing) openRoom(active, active.room, true); else scrollDown();
  });
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
    el("kapat").hidden = !thread || thread === "T-0";
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
      const thread = node("button", m.thread + " · " + m.turn + "/" + (m.max_turns || "∞"), "konusma");
      thread.type = "button"; thread.addEventListener("click", () => showFilter(m.thread));
      const answer = node("button", "Yanıtla", "yanitla"); answer.type = "button";
      answer.addEventListener("click", () => {
        target = m.thread; targetExplicit = false;
        showFilter(m.thread); reply = m.id;
        if (m.from !== session.name) {
          selectRecipient(m.from);
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
      rows.push({thread: m.thread, element: row, message: m}); messages.append(row);
    }
    if (rows.length > maxRows) {
      for (const row of rows.splice(0, rows.length - maxRows)) { row.element.remove(); ids.delete(row.message.id); }
      if (session.history) session.history.hasMore = true;
    }
    threads.clear(); el("konusma-listesi").replaceChildren();
    for (const row of rows) updateThread(row.message);
    if (session.history && rows.length) {
      session.history.oldest = rows[0].message.id;
      el("gecmis").hidden = !session.history.hasMore;
    }
    if (rows.length && !targetExplicit && !filter && !text.value) target = rows[rows.length - 1].thread;
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
    const history = session.history;
    const initial = await request(session, "/log?tail=1", signal);
    if (signal.aborted) return;
    history.hasMore = initial.has_more;
    addMessages(initial.messages, session);
    let after = Math.max(0, ...initial.messages.map((m) => m.id));
    while (!signal.aborted) {
      status.textContent = "Canlı izleniyor"; status.classList.add("canli");
      const data = await request(session, "/log?after=" + after + "&wait=25", signal);
      if (signal.aborted) return;
      if (!history.browsing) addMessages(data.messages, session);
      else if (data.messages.length) { el("yeni").textContent = "↓ Güncel mesajlara dön"; el("yeni").hidden = false; }
      for (const m of data.messages) after = Math.max(after, m.id);
    }
  }
  el("guncel").addEventListener("click", () => { if (active) openRoom(active, active.room, true); });
  el("gecmis").addEventListener("click", async () => {
    const session = active, history = session?.history;
    if (!session || !history || !history.oldest || el("gecmis").disabled) return;
    const signal = session.roomController.signal;
    el("gecmis").disabled = true;
    // Canlı döngü yükleme sırasında sayfa imlecini değiştirmesin.
    history.browsing = true; el("guncel").hidden = false;
    try {
      const data = await request(session, "/log?before=" + history.oldest, signal);
      if (signal.aborted) return;
      history.hasMore = data.has_more;
      if (data.messages.length) {
        rows = []; ids = new Set(); messages.replaceChildren(); threads.clear();
        el("konusma-listesi").replaceChildren(); showFilter("");
        addMessages(data.messages, session);
        messages.scrollTop = 0;
      }
      el("gecmis").hidden = !history.hasMore;
    } catch (_) { /* Oda değişimi ve çıkış bekleyen sayfayı iptal eder. */ }
    finally { if (!signal.aborted) el("gecmis").disabled = false; }
  });
  function validateRecipient() {
    const unavailable = Array.from(recipient.options).some((o) => o.value === recipient.value && o.disabled);
    recipient.setCustomValidity(unavailable ? "Seçtiğin alıcı artık bu odada yok. Yeni bir alıcı seç." : "");
    el("alici-uyari").hidden = !unavailable;
    return !unavailable;
  }
  function selectRecipient(name, unavailable = true) {
    if (name && !Array.from(recipient.options).some((o) => o.value === name)) {
      const option = node("option", name + (unavailable ? " (odada değil)" : ""));
      option.value = name; option.disabled = unavailable; recipient.append(option);
    }
    recipient.value = name;
    validateRecipient();
  }
  function refreshRecipients(agents) {
    const selected = recipient.value;
    recipient.replaceChildren(node("option", "herkes")); recipient.firstChild.value = "";
    for (const a of agents) {
      if (a.name !== active?.name) {
        const option = node("option", a.name); option.value = a.name; recipient.append(option);
      }
    }
    selectRecipient(selected);
  }
  recipient.addEventListener("change", validateRecipient);
  async function agentsLoop(session, signal) {
    while (!signal.aborted) {
      const data = await request(session, "/agents", signal);
      if (signal.aborted) return;
      el("ajanlar").replaceChildren();
      const participants = [...data.agents].sort((a, b) => Number(b.waiting) - Number(a.waiting) || a.name.localeCompare(b.name, "tr-TR"));
      for (const a of participants) {
        const li = node("li", undefined, "katilimci");
        const avatar = accountAvatar(a.name, "mini-avatar");
        const content = node("div", undefined, "katilimci-icerik");
        content.append(node("span", a.name, "katilimci-isim"));
        if (a.waiting || a.name === session.name) {
          content.append(node("span", a.name === session.name ? "Sen" : "Dinliyor", "katilimci-durum" + (a.waiting ? " bekliyor" : "")));
        } else if (a.last_seen && Number.isFinite(Date.parse(a.last_seen))) {
          const seen = managementTime(a.last_seen); seen.className = "katilimci-durum"; content.append(seen);
        }
        li.append(avatar, content, node("span", a.kind === "insan" ? "İnsan" : "Ajan", "katilimci-tur"));
        el("ajanlar").append(li);
      }
      refreshRecipients(data.agents);
      el("katilimci-sayisi").textContent = String(data.agents.length);
      await pause(30000, signal);
    }
  }
  function readDraft() {
    return {body: text.value, to: recipient.value, type: el("tip").value, filter,
      recipientMissing: Array.from(recipient.options).some((o) => o.value === recipient.value && o.disabled),
      target: currentTarget(), explicit: targetExplicit, reply, replyLabel: el("yanit-bilgi").textContent};
  }
  function restoreDraft(draft) {
    text.value = draft.body; count(); selectRecipient(draft.to, draft.recipientMissing); el("tip").value = draft.type;
    showFilter(draft.filter); target = draft.target; targetExplicit = draft.explicit; reply = draft.reply;
    el("yanit-bilgi").textContent = draft.replyLabel; el("yanit").hidden = reply === null; updateBaglam();
  }
  function openRoom(session, room, preserveDraft = false) {
    const draft = preserveDraft && session.room === room ? readDraft() : null;
    if (session.roomController) session.roomController.abort();
    status.classList.remove("canli"); status.textContent = "Oda yükleniyor…";
    resetRoom(); session.room = room; session.roomController = new AbortController();
    session.history = {oldest: 0, hasMore: false, browsing: false};
    el("yeni").textContent = "↓ Yeni mesajlar";
    const title = session.rooms?.find((r) => r.name === room)?.title || room;
    el("oda-baslik").textContent = title; el("tek-oda-ad").textContent = title;
    el("ozet-oda").textContent = title; el("odasiz-durum").hidden = true;
    menu(false); panel(!narrow.matches);
    el("oda-icerik").hidden = false;
    if (draft) restoreDraft(draft);
    const signal = session.roomController.signal;
    logLoop(session, signal).catch(() => {}); agentsLoop(session, signal).catch(() => {});
  }
  el("oda").addEventListener("change", () => { if (active) openRoom(active, el("oda").value); });
  async function reloadRooms(session, selected = "", preserveDraft = false) {
    const {response, data} = await api(session, "/v1/rooms", session.controller.signal);
    if (!response.ok || !Array.isArray(data.rooms)) throw new Error("Odalar yüklenemedi.");
    if (active !== session) throw new Error("Oturum değişti.");
    session.rooms = data.rooms;
    el("oda").replaceChildren(); el("oda-secimi").hidden = data.rooms.length < 2;
    el("tek-oda").hidden = data.rooms.length !== 1;
    for (const room of data.rooms) { const option = node("option", room.title); option.value = room.name; el("oda").append(option); }
    el("odasiz-durum").hidden = data.rooms.length > 0;
    if (!data.rooms.length) {
      session.roomController?.abort(); session.room = ""; resetRoom();
      el("oda-icerik").hidden = true; status.textContent = "Bu hesabın odası yok."; return;
    }
    const room = data.rooms.some((r) => r.name === selected) ? selected : data.rooms[0].name;
    el("oda").value = room;
    openRoom(session, room, preserveDraft);
  }
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
      session.maxBody = Number.isInteger(data.max_body) && data.max_body > 0 && data.max_body <= 1048576 ? data.max_body : 16384;
      session.yonetici = data.yonetici === true;
      text.maxLength = session.maxBody; count();
      remember(token, name); el("token").value = "";
      el("yonetim-ac").hidden = !data.yonetici;
      el("oda-ekle").hidden = !data.yonetici; el("uyeleri-yonet").hidden = !data.yonetici;
      form.hidden = true; view.hidden = false; el("giris-dugme").disabled = false;
      el("giris-ekrani").hidden = true; document.body.classList.add("oturum-acik");
      el("kullanici-ad").textContent = name;
      accountAvatar(name, "mini-avatar", el("kullanici-avatar"));
      el("oda-baslik").textContent = name;
      el("oda-icerik").hidden = true;
      await reloadRooms(session, preferredRoom);
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
    resetManagementView();
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
    gecersiz_baslik: "Oda başlığı 1–80 karakter olmalı; satır sonu içeremez.",
    gecersiz_uye: "Üye seçimi geçersiz; yalnız aktif hesapları seçebilirsin.",
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
  function acceptManagementSession(data) {
    clearTimeout(managementTimer);
    managementSession = data.oturum; managementExpiry = Date.parse(data.bitis);
    managementTimer = setTimeout(() => {
      clearYonetim();
      if (el("oda-dialog").open) showRoomVerification();
      if (el("uyelik-dialog").open) showMembershipVerification();
      el("yonetim-durum").textContent = managementErrors.yonetim_oturumu_gerekli;
    }, Math.max(0, managementExpiry - Date.now()));
  }
  let roomCreateReady = false, roomCreateBusy = false;
  function showRoomVerification() {
    roomCreateReady = false;
    el("oda-dogrulama").hidden = false; el("oda-kod").disabled = false;
    el("oda-bilgiler").hidden = true; el("oda-bilgiler").disabled = true;
    el("oda-kaydet").textContent = "Doğrula ve devam et";
  }
  async function prepareRoomCreation(session) {
    const accounts = await managementAPI("/hesaplar");
    if (active !== session || !el("oda-dialog").open) return;
    el("oda-uyeler").replaceChildren();
    for (const a of accounts.filter((a) => !a.revoked)) {
      const label = node("label", undefined, "oda-uye"), input = node("input");
      input.type = "checkbox"; input.value = a.name; input.checked = a.name === session.name;
      input.disabled = a.name === session.name;
      label.append(input, accountAvatar(a.name, "mini-avatar"), node("span", a.name),
        node("small", a.name === session.name ? "Sen" : a.yonetici ? "Yönetici" : a.kind === "insan" ? "İnsan" : "Ajan"));
      el("oda-uyeler").append(label);
    }
    roomCreateReady = true;
    el("oda-kod").value = ""; el("oda-kod").disabled = true; el("oda-dogrulama").hidden = true;
    el("oda-bilgiler").disabled = false; el("oda-bilgiler").hidden = false;
    el("oda-kaydet").textContent = "Odayı oluştur"; el("oda-baslik-girdi").focus();
  }
  el("oda-ekle").addEventListener("click", async () => {
    const session = active;
    if (!session?.yonetici || roomCreateBusy) return;
    menu(false); showRoomVerification(); el("oda-hata").textContent = "";
    el("oda-dialog").showModal();
    if (managementSession && Date.now() < managementExpiry) {
      roomCreateBusy = true; el("oda-kaydet").disabled = true;
      try { await prepareRoomCreation(session); } catch (error) { el("oda-hata").textContent = error.message; }
      finally { roomCreateBusy = false; el("oda-kaydet").disabled = false; }
    } else el("oda-kod").focus();
  });
  el("oda-vazgec").addEventListener("click", () => { if (!roomCreateBusy) el("oda-dialog").close(); });
  el("oda-dialog").addEventListener("cancel", (event) => { if (roomCreateBusy) event.preventDefault(); });
  el("oda-dialog").addEventListener("close", () => {
    el("oda-kod").value = ""; el("oda-baslik-girdi").value = ""; el("oda-uyeler").replaceChildren();
    el("oda-hata").textContent = ""; showRoomVerification();
    if (active?.yonetici) (matchMedia("(max-width: 760px)").matches ? navigationButton() : el("oda-ekle")).focus();
  });
  el("oda-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const session = active;
    if (!session?.yonetici || roomCreateBusy) return;
    roomCreateBusy = true; el("oda-kaydet").disabled = true; el("oda-vazgec").disabled = true;
    el("oda-hata").textContent = "";
    try {
      if (!roomCreateReady) {
        const kod = el("oda-kod").value; el("oda-kod").value = "";
        const data = await managementAPI("/oturum", "POST", {kod});
        acceptManagementSession(data); await prepareRoomCreation(session); return;
      }
      const baslik = el("oda-baslik-girdi").value.trim();
      const uyeler = [...el("oda-uyeler").querySelectorAll("input:checked")].map((input) => input.value);
      const room = await managementAPI("/odalar", "POST", {baslik, uyeler});
      await reloadRooms(session, room.name); el("oda-dialog").close();
      status.textContent = baslik + " odası oluşturuldu.";
    } catch (error) {
      if (active === session && el("oda-dialog").open) {
        el("oda-hata").textContent = error.message;
        if (!managementSession || Date.now() >= managementExpiry) showRoomVerification();
      }
    } finally { roomCreateBusy = false; el("oda-kaydet").disabled = false; el("oda-vazgec").disabled = false; }
  });
  let membershipRoom = "", membershipBusy = false;
  function showMembershipVerification() {
    el("uyelik-form").hidden = false; el("uyelik-hesaplar").replaceChildren();
    el("uyelik-kod").value = "";
  }
  async function renderMembership() {
    const session = active, room = membershipRoom;
    const accounts = await managementAPI("/hesaplar");
    if (active !== session || !el("uyelik-dialog").open || room !== membershipRoom) return;
    el("uyelik-form").hidden = true; el("uyelik-kod").value = "";
    el("uyelik-hesaplar").replaceChildren();
    for (const a of accounts.filter((a) => !a.revoked)) {
      const member = a.rooms.includes(room), row = node("div", undefined, "uyelik-satir");
      const label = node("span", a.name + (a.yonetici ? " · Yönetici" : a.kind === "ajan" ? " · Ajan" : " · İnsan"));
      const button = node("button", member ? "Çıkar" : "Ekle"); button.type = "button";
      button.setAttribute("aria-label", a.name + (member ? " hesabını odadan çıkar" : " hesabını odaya ekle"));
      button.addEventListener("click", async () => {
        if (membershipBusy) return;
        membershipBusy = true; button.disabled = true; el("uyelik-durum").textContent = "";
        try {
          const base = "/hesaplar/" + encodeURIComponent(a.name) + "/odalar";
          await managementAPI(member ? base + "/" + encodeURIComponent(room) : base, member ? "DELETE" : "POST", member ? undefined : {oda: room});
          if (active !== session) return;
          if (a.name === session.name && member) {
            el("uyelik-dialog").close(); await reloadRooms(session); return;
          }
          await renderMembership();
          el("uyelik-durum").textContent = a.name + (member ? " odadan çıkarıldı." : " odaya eklendi.");
        } catch (error) {
          if (active === session) {
            el("uyelik-durum").textContent = error.message;
            if (!managementSession) showMembershipVerification();
          }
        } finally { membershipBusy = false; button.disabled = false; }
      });
      row.append(accountAvatar(a.name, "mini-avatar"), label, node("small", member ? "Üye" : "Üye değil"), button);
      el("uyelik-hesaplar").append(row);
    }
  }
  el("uyeleri-yonet").addEventListener("click", async () => {
    if (!active?.yonetici || !active.room) return;
    membershipRoom = active.room; showMembershipVerification(); el("uyelik-durum").textContent = "";
    el("uyelik-baslik").textContent = el("oda-baslik").textContent + " · Üyeleri yönet";
    el("uyelik-dialog").showModal();
    if (managementSession && Date.now() < managementExpiry) {
      try { await renderMembership(); } catch (error) { el("uyelik-durum").textContent = error.message; }
    } else el("uyelik-kod").focus();
  });
  el("uyelik-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!active?.yonetici || membershipBusy) return;
    membershipBusy = true; el("uyelik-durum").textContent = "";
    try {
      const kod = el("uyelik-kod").value; el("uyelik-kod").value = "";
      acceptManagementSession(await managementAPI("/oturum", "POST", {kod}));
      await renderMembership();
    } catch (error) { el("uyelik-durum").textContent = error.message; }
    finally { membershipBusy = false; }
  });
  el("uyelik-kapat").addEventListener("click", () => { if (!membershipBusy) el("uyelik-dialog").close(); });
  el("uyelik-dialog").addEventListener("cancel", (event) => { if (membershipBusy) event.preventDefault(); });
  el("uyelik-dialog").addEventListener("close", () => { membershipRoom = ""; showMembershipVerification(); });
  function managementAction(fn) {
    return async () => { try { await fn(); } catch (error) { clearTimeout(managementToastTimer); el("yonetim-durum").textContent = error.message; } };
  }
  function managementButton(label, fn) {
    const b = node("button", label); b.type = "button"; b.addEventListener("click", managementAction(async () => {
      b.disabled = true; try { await fn(); } finally { b.disabled = false; }
    })); return b;
  }
  let managementAccounts = [], managementRecords = [], managementRooms = [], managementEditor = "";
  let managementClock = null, managementToastTimer = null, managementRevoke = null;
  let managementRoomInput = null;
  const managementTabs = ["ozet", "hesaplar", "yeni", "kayit"];
  const managementMobile = window.matchMedia("(max-width: 760px)");
  function managementTabOrientation() {
    el("sekme-ozet").parentElement.setAttribute("aria-orientation", managementMobile.matches ? "horizontal" : "vertical");
  }
  managementMobile.addEventListener("change", managementTabOrientation);
  managementTabOrientation();
  function managementTab(name, focus = false) {
    for (const key of managementTabs) {
      const selected = key === name, tab = el("sekme-" + key);
      tab.setAttribute("aria-selected", String(selected)); tab.tabIndex = selected ? 0 : -1;
      el("panel-" + key).hidden = !selected;
      if (selected && focus) tab.focus();
    }
  }
  for (const [index, key] of managementTabs.entries()) {
    el("sekme-" + key).addEventListener("click", () => managementTab(key));
    el("sekme-" + key).addEventListener("keydown", (event) => {
      let next;
      if (event.key === (managementMobile.matches ? "ArrowRight" : "ArrowDown")) next = (index + 1) % managementTabs.length;
      if (event.key === (managementMobile.matches ? "ArrowLeft" : "ArrowUp")) next = (index + managementTabs.length - 1) % managementTabs.length;
      if (event.key === "Home") next = 0;
      if (event.key === "End") next = managementTabs.length - 1;
      if (next !== undefined) { event.preventDefault(); managementTab(managementTabs[next], true); }
    });
  }
  function resetManagementView() {
    clearInterval(managementClock); clearTimeout(managementToastTimer);
    managementAccounts = []; managementRecords = []; managementRooms = []; managementEditor = ""; managementRevoke = null; managementRoomInput = null;
    for (const id of ["yonetim-metrikler", "yonetim-son-etkinlik", "yonetim-odalar", "yonetim-uyarilar", "yonetim-durum"]) el(id).replaceChildren();
    el("yonetim-iptal-dialog").close(); el("yonetim-iptal-ad").value = "";
    el("yonetim-iptal-hata").textContent = "";
    el("yonetim-iptal-aciklama").textContent = "";
    el("yonetim-iptal-onay").textContent = "Hesabı iptal et"; el("yonetim-iptal-onay").disabled = true;
    el("yonetim-sure").hidden = true; el("yonetim-yenile").hidden = true;
    el("yonetim-arama").value = ""; el("yonetim-tur").value = "";
    el("yonetim-filtre-durum").value = "aktif";
    renderManagementOperationFilter();
    managementTab("ozet");
  }
  function managementTime(value) {
    const time = node("time"), date = new Date(value), ms = date.getTime();
    if (!value || !Number.isFinite(ms)) { time.textContent = "Henüz görülmedi"; return time; }
    time.dateTime = date.toISOString(); time.title = date.toLocaleString("tr-TR");
    const minutes = Math.max(0, Math.floor((Date.now() - ms) / 60000));
    time.textContent = minutes < 1 ? "Az önce" : minutes < 60 ? minutes + " dk önce" :
      minutes < 1440 ? Math.floor(minutes / 60) + " saat önce" : Math.floor(minutes / 1440) + " gün önce";
    return time;
  }
  function managementCountdown() {
    const seconds = Math.max(0, Math.ceil((managementExpiry - Date.now()) / 1000));
    el("yonetim-sayac").textContent = String(Math.floor(seconds / 60)).padStart(2, "0") + ":" + String(seconds % 60).padStart(2, "0");
    el("yonetim-ilerleme").value = Math.min(600, seconds);
  }
  function managementToast(message, undo) {
    clearTimeout(managementToastTimer);
    const area = el("yonetim-durum"); area.replaceChildren(node("span", message));
    if (undo) area.append(managementButton("Geri al", async () => {
      await undo(); managementToast("Oda üyeliği geri alındı.");
    }));
    area.append(managementButton("Kapat", async () => { clearTimeout(managementToastTimer); area.replaceChildren(); }));
    managementToastTimer = setTimeout(() => area.replaceChildren(), 8000);
  }
  function managementCell(row, label, content) {
    const cell = node("td"); cell.dataset.label = label;
    cell.append(typeof content === "string" ? node("span", content) : content); row.append(cell); return cell;
  }
  function managementTable(headers, caption) {
    const table = node("table", undefined, "yonetim-tablo"), head = node("thead"), row = node("tr");
    table.append(node("caption", caption, "sr-only"));
    for (const title of headers) { const th = node("th", title); th.scope = "col"; row.append(th); }
    head.append(row); const body = node("tbody"); table.append(head, body); return {table, body};
  }
  function managementChips(rooms) {
    const chips = node("div", undefined, "yonetim-chipler");
    for (const room of rooms) chips.append(node("span", room, "yonetim-chip"));
    if (!rooms.length) chips.append(node("span", "Oda yok")); return chips;
  }
  function renderManagementEditor(a, body) {
    const row = node("tr", undefined, "yonetim-duzenleme"), cell = node("td"); cell.colSpan = 5;
    const box = node("div", undefined, "yonetim-oda-editor"), chips = node("div", undefined, "yonetim-chipler");
    const base = "/hesaplar/" + encodeURIComponent(a.name);
    box.append(node("strong", a.name + " · Odaları düzenle"));
    for (const room of a.rooms) {
      const chip = node("span", room, "yonetim-chip");
      const remove = managementButton("×", async () => {
        await managementAPI(base + "/odalar/" + encodeURIComponent(room), "DELETE");
        await refreshManagement();
        managementToast(a.name + " · " + room + " odasından çıkarıldı", async () => {
          await managementAPI(base + "/odalar", "POST", {oda: room}); await refreshManagement();
        });
      });
      remove.setAttribute("aria-label", a.name + " hesabını " + room + " odasından çıkar"); chip.append(remove); chips.append(chip);
    }
    if (!a.rooms.length) chips.append(node("span", "Oda yok"));
    const controls = node("div", undefined, "yonetim-oda-kontroller"), input = node("select");
    input.id = "yonetim-oda-girdi";
    const placeholder = node("option", "Oda seç"); placeholder.value = ""; input.append(placeholder);
    for (const room of managementRooms.filter((room) => !a.rooms.includes(room.name))) {
      const option = node("option", room.title); option.value = room.name; input.append(option);
    }
    managementRoomInput = input;
    const label = node("label", "Oda adı"); label.htmlFor = input.id;
    const add = managementButton("Ekle", async () => {
      await managementAPI(base + "/odalar", "POST", {oda: input.value.trim()}); await refreshManagement();
      managementRoomInput?.focus();
    }); add.disabled = true;
    input.addEventListener("change", () => { add.disabled = !input.value.trim(); });
    input.addEventListener("keydown", (event) => { if (event.key === "Enter" && !add.disabled) { event.preventDefault(); add.click(); } });
    controls.append(label, input, add, managementButton("Kapat", async () => {
      managementEditor = ""; renderManagementAccounts(); el("yonetim-hesaplar").querySelectorAll("button[data-account]").forEach((button) => {
        if (button.dataset.account === a.name) button.focus();
      });
    })); box.append(chips, controls); cell.append(box); row.append(cell); body.append(row);
  }
  function managementAccountTable(accounts) {
    const {table, body} = managementTable(["Hesap", "Odalar", "Son görülme", "Durum", "İşlemler"], "Hesaplar");
    for (const a of accounts) {
      const row = node("tr", undefined, a.revoked ? "yonetim-iptal-satir" : "");
      const identity = node("div", undefined, "yonetim-kimlik"), info = node("div");
      info.append(node("strong", a.name), node("span", a.kind === "ajan" ? "Ajan" : "İnsan", "yonetim-rozet"));
      if (a.yonetici) info.append(node("span", "Yönetici", "yonetim-rozet"));
      identity.append(accountAvatar(a.name, "mini-avatar"), info); managementCell(row, "Hesap", identity);
      managementCell(row, "Odalar", managementChips(a.rooms)); managementCell(row, "Son görülme", managementTime(a.last_seen));
      managementCell(row, "Durum", node("span", a.revoked ? "İptal edilmiş" : "Aktif", "yonetim-durum-etiket" + (a.revoked ? " iptal" : "")));
      const actions = node("div", undefined, "yonetim-satir-islemler");
      if (!a.revoked) {
        const edit = managementButton("Odalar", async () => {
          managementEditor = managementEditor === a.name ? "" : a.name; renderManagementAccounts();
          if (managementEditor) managementRoomInput?.focus();
          else el("yonetim-hesaplar").querySelectorAll("button[data-account]").forEach((b) => { if (b.dataset.account === a.name) b.focus(); });
        });
        edit.dataset.account = a.name; edit.setAttribute("aria-expanded", String(managementEditor === a.name));
        const menu = node("details", undefined, "yonetim-islem-menu"), summary = node("summary", "⋯");
        summary.setAttribute("aria-label", a.name + " hesabı işlemleri");
        const revoke = managementButton("Hesabı iptal et", async () => { menu.open = false; openManagementRevoke(a, summary); });
        revoke.className = "yonetim-tehlike"; menu.append(summary, revoke); actions.append(edit); if (!a.yonetici) actions.append(menu);
      } else actions.append(node("span", "—"));
      managementCell(row, "İşlemler", actions); body.append(row);
      if (!a.revoked && managementEditor === a.name) renderManagementEditor(a, body);
    }
    if (!accounts.length) { const row = node("tr"), cell = node("td", "Eşleşen hesap yok", "yonetim-bos"); cell.colSpan = 5; row.append(cell); body.append(row); }
    return table;
  }
  function renderManagementAccounts() {
    managementRoomInput = null;
    const query = el("yonetim-arama").value.toLocaleLowerCase("tr-TR").trim(), kind = el("yonetim-tur").value;
    const accounts = managementAccounts.filter((a) => a.name.toLocaleLowerCase("tr-TR").includes(query) && (!kind || kind === a.kind));
    const list = el("yonetim-hesaplar"), revoked = accounts.filter((a) => a.revoked);
    const wasOpen = list.querySelector(".yonetim-iptaller")?.open || false;
    list.replaceChildren();
    if (el("yonetim-filtre-durum").value === "aktif") list.append(managementAccountTable(accounts.filter((a) => !a.revoked)));
    const details = node("details", undefined, "yonetim-iptaller");
    details.open = wasOpen || el("yonetim-filtre-durum").value === "iptal";
    details.append(node("summary", "İptal edilmiş (" + revoked.length + ")"), managementAccountTable(revoked)); list.append(details);
  }
  function managementOperationLabel(value) {
    const labels = {
      hesaplar_listelendi: "Hesaplar görüntülendi", kayit_listelendi: "Kayıt görüntülendi",
      oturum_acildi: "Yönetim oturumu açıldı", oturum_reddedildi: "Yönetim oturumu reddedildi",
      hesap_acildi: "Hesap açıldı", hesap_iptal: "Hesap iptal edildi", iptal: "Hesap iptal edildi", revoke: "Hesap iptal edildi",
      odaya_eklendi: "Odaya eklendi", "odaya-ekle": "Odaya eklendi",
      oda_olusturuldu: "Oda oluşturuldu",
      odadan_cikarildi: "Odadan çıkarıldı", "odadan-cikar": "Odadan çıkarıldı",
      "yonetici-yap": "Yönetici yetkisi verildi", "yonetici-al": "Yönetici yetkisi kaldırıldı"
    };
    return Object.prototype.hasOwnProperty.call(labels, value) ? labels[value] : String(value || "—");
  }
  function managementReadOperation(value) {
    return String(value || "").endsWith("_listelendi");
  }
  function renderManagementOperationFilter(selected = "degisiklikler") {
    const select = el("yonetim-islem"), changes = node("option", "Değişiklikler"), all = node("option", "Tümü");
    changes.value = "degisiklikler"; all.value = ""; select.replaceChildren(changes, all);
    for (const type of [...new Set(managementRecords.map((k) => k.islem))].sort()) {
      const option = node("option", managementOperationLabel(type)); option.value = type; select.append(option);
    }
    select.value = [...select.options].some((o) => o.value === selected) ? selected : "degisiklikler";
  }
  function managementOperation(value) {
    const code = String(value || ""), tone = /iptal|revoke|reddedildi|sil|cikar|yonetici-al/.test(code) ? "tehlike" : /ekle|olustur|ac/.test(code) ? "olumlu" : "bilgi";
    return node("span", managementOperationLabel(value), "yonetim-islem " + tone);
  }
  function renderManagementRecords() {
    const {table, body} = managementTable(["Zaman", "Yapan", "İşlem", "Hedef", "Ayrıntı"], "Denetim kaydı");
    const selected = el("yonetim-islem").value;
    const records = managementRecords.filter((k) => selected === "degisiklikler" ? !managementReadOperation(k.islem) : !selected || k.islem === selected);
    for (const k of records) {
      const row = node("tr"); managementCell(row, "Zaman", managementTime(k.zaman));
      managementCell(row, "Yapan", String(k.yapan || "—")); managementCell(row, "İşlem", managementOperation(k.islem));
      managementCell(row, "Hedef", String(k.hedef || "—")); managementCell(row, "Ayrıntı", String(k.ayrinti || "—")); body.append(row);
    }
    if (!records.length) { const row = node("tr"), cell = node("td", "Kayıt bulunamadı", "yonetim-bos"); cell.colSpan = 5; row.append(cell); body.append(row); }
    // Eski ol öğesinin kimliğini ve türünü koru; tabloyu tek liste öğesinde taşı.
    const item = node("li"); item.append(table); el("yonetim-kayit").replaceChildren(item);
  }
  function renderManagementOverview() {
    const accounts = managementAccounts, live = accounts.filter((a) => !a.revoked), metrics = el("yonetim-metrikler"); metrics.replaceChildren();
    for (const [label, total] of [["Toplam hesap", accounts.length], ["Ajan", live.filter((a) => a.kind === "ajan").length], ["İnsan", live.filter((a) => a.kind === "insan").length], ["İptal edilmiş", accounts.length - live.length]]) {
      const card = node("div", undefined, "yonetim-metrik"); card.append(node("span", label), node("strong", String(total)));
      metrics.append(card);
    }
    const activity = el("yonetim-son-etkinlik"); activity.replaceChildren();
    const recent = managementRecords.filter((k) => !managementReadOperation(k.islem)).slice(0, 5);
    for (const k of recent) {
      const target = String(k.hedef || "").trim();
      const item = node("li"); item.append(managementOperation(k.islem), node("span", (k.yapan || "—") + (target ? " → " + target : "")), managementTime(k.zaman)); activity.append(item);
    }
    if (!recent.length) activity.append(node("li", "Henüz etkinlik yok."));
    const rooms = new Map();
    for (const a of live) for (const room of new Set(a.rooms)) { if (!rooms.has(room)) rooms.set(room, []); rooms.get(room).push(a); }
    const roomList = el("yonetim-odalar"); roomList.replaceChildren();
    for (const [name, members] of [...rooms].sort(([a], [b]) => a.localeCompare(b, "tr"))) {
      const item = node("li"), stack = node("span", undefined, "yonetim-avatar-yigini");
      for (const a of members.slice(0, 4)) { const avatar = accountAvatar(a.name, "mini-avatar"); avatar.title = a.name; stack.append(avatar); }
      if (members.length > 4) stack.append(node("span", "+" + (members.length - 4), "mini-avatar"));
      stack.setAttribute("aria-label", members.map((a) => a.name).join(", "));
      item.append(node("strong", name), node("span", members.length + " üye"), stack); roomList.append(item);
    }
    if (!rooms.size) roomList.append(node("li", "Henüz oda yok."));
    const warnings = el("yonetim-uyarilar"); warnings.replaceChildren();
    for (const a of live.filter((a) => a.kind === "ajan" && (!a.last_seen || !Number.isFinite(Date.parse(a.last_seen)) || Date.now() - Date.parse(a.last_seen) >= 86400000))) {
      const item = node("li"); item.append(accountAvatar(a.name, "mini-avatar"), node("strong", a.name), managementTime(a.last_seen)); warnings.append(item);
    }
    if (!warnings.childElementCount) warnings.append(node("li", "Tüm aktif ajanlar son 24 saatte görüldü."));
  }
  function openManagementRevoke(account, trigger) {
    managementRevoke = {account, trigger}; el("yonetim-iptal-ad").value = "";
    el("yonetim-iptal-aciklama").textContent = "Bu işlem geri alınamaz. Onaylamak için hesabın adını yaz: " + account.name;
    el("yonetim-iptal-hata").textContent = ""; el("yonetim-iptal-onay").textContent = account.name + " hesabını iptal et";
    el("yonetim-iptal-onay").disabled = true; el("yonetim-iptal-dialog").showModal(); el("yonetim-iptal-ad").focus();
  }
  el("yonetim-iptal-ad").addEventListener("input", () => {
    el("yonetim-iptal-onay").disabled = !managementRevoke || el("yonetim-iptal-ad").value !== managementRevoke.account.name;
  });
  el("yonetim-iptal-vazgec").addEventListener("click", () => el("yonetim-iptal-dialog").close());
  el("yonetim-iptal-dialog").addEventListener("close", () => {
    managementRevoke?.trigger?.focus(); managementRevoke = null; el("yonetim-iptal-ad").value = "";
  });
  el("yonetim-iptal-onay").addEventListener("click", async () => {
    if (!managementRevoke || el("yonetim-iptal-onay").disabled) return;
    const account = managementRevoke.account, button = el("yonetim-iptal-onay"); button.disabled = true;
    el("yonetim-iptal-ad").disabled = true;
    try {
      await managementAPI("/hesaplar/" + encodeURIComponent(account.name) + "/iptal", "POST");
      el("yonetim-iptal-dialog").close(); await refreshManagement();
      el("yonetim-arama").focus(); managementToast(account.name + " hesabı iptal edildi.");
    } catch (error) { el("yonetim-iptal-hata").textContent = error.message; }
    finally { el("yonetim-iptal-ad").disabled = false; button.disabled = !managementRevoke || el("yonetim-iptal-ad").value !== managementRevoke.account.name; }
  });
  for (const id of ["yonetim-arama", "yonetim-tur", "yonetim-filtre-durum"]) el(id).addEventListener(id === "yonetim-arama" ? "input" : "change", renderManagementAccounts);
  el("yonetim-islem").addEventListener("change", renderManagementRecords);
  async function refreshManagement() {
    const accounts = await managementAPI("/hesaplar");
    const records = await managementAPI("/kayit?son=100");
    managementRooms = (await managementAPI("/odalar")).rooms;
    managementAccounts = accounts; managementRecords = [...records].sort((a, b) => Date.parse(b.zaman) - Date.parse(a.zaman));
    renderManagementOperationFilter(el("yonetim-islem").value);
    renderManagementAccounts(); renderManagementRecords(); renderManagementOverview();
    el("yonetim-sure").hidden = false; el("yonetim-yenile").hidden = false;
    managementCountdown(); clearInterval(managementClock); managementClock = setInterval(managementCountdown, 1000);
  }
  el("yonetim-ac").addEventListener("click", managementAction(async () => {
    menu(false); panel(false);
    view.hidden = true; el("yonetim").hidden = false;
    if (managementSession && Date.now() < managementExpiry) await refreshManagement();
    else { clearYonetim(); el("yonetim-kod").focus(); }
  }));
  el("sohbete-don").addEventListener("click", () => {
    el("yonetim").hidden = true; view.hidden = false; el("yonetim-parola").replaceChildren();
    if (active) reloadRooms(active, active.room, true).catch(() => { status.textContent = "Oda listesi yenilenemedi."; });
  });
  el("yonetim-yenile").addEventListener("click", managementAction(refreshManagement));
  el("yonetim-kod-form").addEventListener("submit", (event) => {
    event.preventDefault(); const submit = event.submitter; submit.disabled = true;
    managementAction(async () => {
      try {
      const kod = el("yonetim-kod").value; el("yonetim-kod").value = "";
      const data = await managementAPI("/oturum", "POST", {kod});
      acceptManagementSession(data);
      // Önceki hatalı kodun bildirimi oturum açılınca kalmasın.
      clearTimeout(managementToastTimer); el("yonetim-durum").replaceChildren();
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
  function count() {
    const size = encoder.encode(text.value).length, limit = active?.maxBody || 16384;
    el("sayac").textContent = size + " / " + limit + " bayt";
    text.setCustomValidity(size > limit ? "Mesajın UTF-8 boyutu sınırı aşıyor." : "");
  }
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
    if (!validateRecipient()) { recipient.reportValidity(); return; }
    count();
    if (!text.checkValidity()) { text.reportValidity(); return; }
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
        const retryBody = {...body}; delete retryBody.thread; delete retryBody.reply_to;
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
