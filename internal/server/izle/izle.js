"use strict";
(() => {
  const el = (id) => document.getElementById(id);
  const form = el("giris"), view = el("izleme"), status = el("durum");
  const messages = el("mesajlar"), text = el("metin"), recipient = el("alici");
  const tokenKey = "relay-izle-token", nameKey = "relay-izle-ad";
  let active = null, filter = "", reply = null, rows = [], ids = new Set();
  const preferredRoom = new URLSearchParams(location.search).get("oda");
  function node(tag, content, className) {
    const n = document.createElement(tag);
    if (content !== undefined) n.textContent = content;
    if (className) n.className = className;
    return n;
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
    if (active) {
      active.controller.abort();
      if (active.roomController) active.roomController.abort();
      active.token = "";
    }
    active = null;
  }
  function resetRoom() {
    rows = []; ids = new Set(); messages.replaceChildren(); el("ajanlar").replaceChildren();
    recipient.replaceChildren(node("option", "herkes")); recipient.firstChild.value = "";
    text.value = ""; count(); showFilter(""); el("yeni").hidden = true;
    el("gonderim-durum").textContent = ""; el("gonder").disabled = false; el("kapat").disabled = false;
  }
  function logout(message) {
    stop(); remember("", ""); el("token").value = ""; resetRoom();
    view.hidden = true; form.hidden = false; el("giris-dugme").disabled = false;
    status.textContent = message;
  }
  el("cikis").addEventListener("click", () => logout("Çıkış yapıldı."));
  function bottom() { return messages.scrollHeight - messages.scrollTop - messages.clientHeight < 48; }
  function scrollDown() { messages.scrollTop = messages.scrollHeight; el("yeni").hidden = true; }
  el("yeni").addEventListener("click", scrollDown);
  messages.addEventListener("scroll", () => { if (bottom()) el("yeni").hidden = true; });
  function clearReply() { reply = null; el("yanit").hidden = true; }
  function showFilter(thread) {
    filter = thread; clearReply();
    el("filtre").textContent = thread ? "Konuşma: " + thread : "Tüm konuşmalar";
    el("baglam").textContent = thread ? thread + " konuşmasına yazıyorsun ·" : "Yeni konuşma";
    el("yeni-konusma").hidden = !thread; el("kapat").hidden = !thread;
    for (const row of rows) row.element.hidden = !!thread && row.thread !== thread;
  }
  el("tumu").addEventListener("click", () => showFilter(""));
  el("yeni-konusma").addEventListener("click", () => showFilter(""));
  el("yanit-sil").addEventListener("click", clearReply);
  function addMessages(list, session) {
    const wasBottom = bottom();
    let visibleAdded = false;
    for (const m of list) {
      if (ids.has(m.id)) continue;
      ids.add(m.id);
      const row = node("article", undefined, "mesaj" + (m.from === session.name ? " kendi" : ""));
      const meta = node("div", undefined, "meta");
      meta.append(node("time", new Date(m.created_at).toLocaleString("tr-TR")));
      meta.append(node("span", m.from + " → " + (m.to || "herkes")));
      const types = ["soru", "gorev", "cevap", "sonuc", "bilgi", "kapat"];
      meta.append(node("span", m.type, "etiket " + (types.includes(m.type) ? m.type : "bilgi")));
      const thread = node("button", m.thread + " · " + m.turn + "/" + m.max_turns);
      thread.type = "button"; thread.addEventListener("click", () => showFilter(m.thread));
      const answer = node("button", "Yanıtla"); answer.type = "button";
      answer.addEventListener("click", () => {
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
      meta.append(thread, answer); row.append(meta, node("div", m.body, "govde"));
      row.hidden = !!filter && filter !== m.thread;
      visibleAdded = visibleAdded || !row.hidden;
      rows.push({thread: m.thread, element: row}); messages.append(row);
    }
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
          status.textContent = "İstek sınırı: 30 saniye bekleniyor.";
          await pause(30000, signal); continue;
        }
        if (!response.ok) throw new Error("istek");
        return data;
      } catch (error) {
        if (signal.aborted) throw error;
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
        const li = node("li", a.name + " · " + a.kind + " · " +
          (a.last_seen ? new Date(a.last_seen).toLocaleString("tr-TR") : "henüz görülmedi") +
          (a.waiting ? " · bekliyor" : ""));
        if (a.waiting) li.className = "bekliyor";
        el("ajanlar").append(li);
        if (a.name !== session.name) {
          const option = node("option", a.name); option.value = a.name; recipient.append(option);
        }
      }
      if (Array.from(recipient.options).some((o) => o.value === selected)) recipient.value = selected;
      await pause(30000, signal);
    }
  }
  function openRoom(session, room) {
    if (session.roomController) session.roomController.abort();
    resetRoom(); session.room = room; session.roomController = new AbortController();
    el("oda-baslik").textContent = session.name + " · " + room;
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
      form.hidden = true; view.hidden = false; el("giris-dugme").disabled = false;
      el("oda-baslik").textContent = name;
      el("oda").replaceChildren(); el("oda-secimi").hidden = data.rooms.length < 2;
      el("oda-icerik").hidden = true;
      for (const room of data.rooms) { const option = node("option", room); option.value = room; el("oda").append(option); }
      if (!data.rooms.length) { status.textContent = "Bu hesabın odası yok."; return; }
      el("oda").value = data.rooms.includes(preferredRoom) ? preferredRoom : data.rooms[0];
      openRoom(session, el("oda").value);
    } catch (_) { if (!signal.aborted) logout("Giriş yapılamadı; bağlantını denetle."); }
  }
  form.addEventListener("submit", (event) => {
    event.preventDefault(); login(el("ad").value.trim(), el("token").value.trim());
  });
  function count() { el("sayac").textContent = text.value.length + " / 16000"; }
  text.addEventListener("input", count);
  text.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault(); el("gonderme").requestSubmit();
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
    const thread = filter, original = text.value, originalReply = reply;
    const body = {to: recipient.value, type: el("tip").value, body: original};
    if (thread) body.thread = thread;
    if (reply !== null) body.reply_to = reply;
    el("gonder").disabled = true; el("gonderim-durum").textContent = "Gönderiliyor…";
    try {
      const {response, data} = await api(session, roomPath(session) + "/messages", signal, {
        method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(body)
      });
      if (response.status === 401) { logout("Ad ya da parola hatalı."); return; }
      if (response.status !== 201) { el("gonderim-durum").textContent = errorText(response, data); return; }
      if (text.value === original) { text.value = ""; count(); }
      if (reply === originalReply) clearReply();
      if (!thread && filter === thread) showFilter(data.thread);
      el("gonderim-durum").textContent = "Mesaj gönderildi.";
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
