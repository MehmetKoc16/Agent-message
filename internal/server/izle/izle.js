"use strict";
(() => {
  const el = (id) => document.getElementById(id);
  const form = el("giris"), view = el("izleme"), status = el("durum");
  const roomInput = el("oda"), tokenInput = el("token"), messages = el("mesajlar");
  const storageKey = "relay-izle-token";
  let active = null, filter = "", rows = [];
  roomInput.value = new URLSearchParams(location.search).get("oda") || "";
  try { tokenInput.value = sessionStorage.getItem(storageKey) || ""; } catch (_) { /* Bellekte kalır. */ }

  function remember(token) {
    try {
      if (token) sessionStorage.setItem(storageKey, token);
      else sessionStorage.removeItem(storageKey);
    } catch (_) { /* Depolama kapalıysa yalnız bellek kullanılır. */ }
  }
  function node(tag, text, className) {
    const n = document.createElement(tag);
    if (text !== undefined) n.textContent = text;
    if (className) n.className = className;
    return n;
  }
  function stop() {
    if (active) { active.controller.abort(); active.token = ""; }
    active = null;
  }
  function logout(text) {
    stop(); remember(""); tokenInput.value = "";
    rows = []; messages.replaceChildren(); el("ajanlar").replaceChildren();
    view.hidden = true; form.hidden = false; status.textContent = text;
  }
  el("cikis").addEventListener("click", () => logout("Çıkış yapıldı."));
  function bottom() { return messages.scrollHeight - messages.scrollTop - messages.clientHeight < 48; }
  function scrollDown() { messages.scrollTop = messages.scrollHeight; el("yeni").hidden = true; }
  el("yeni").addEventListener("click", scrollDown);
  messages.addEventListener("scroll", () => { if (bottom()) el("yeni").hidden = true; });
  function showFilter(thread) {
    filter = thread;
    el("filtre").textContent = thread ? "Konuşma: " + thread : "Tüm konuşmalar";
    for (const row of rows) row.element.hidden = !!thread && row.thread !== thread;
  }
  el("tumu").addEventListener("click", () => showFilter(""));
  function addMessages(list) {
    const wasBottom = bottom();
    let visibleAdded = false;
    for (const m of list) {
      const row = node("article", undefined, "mesaj");
      const meta = node("div", undefined, "meta");
      meta.append(node("time", new Date(m.created_at).toLocaleString("tr-TR")));
      meta.append(node("span", m.from + " → " + (m.to || "herkes")));
      const types = ["soru", "gorev", "cevap", "sonuc", "bilgi", "kapat"];
      meta.append(node("span", m.type, "etiket " + (types.includes(m.type) ? m.type : "bilgi")));
      const thread = node("button", m.thread + " · " + m.turn + "/" + m.max_turns);
      thread.type = "button";
      thread.addEventListener("click", () => showFilter(m.thread));
      meta.append(thread);
      row.append(meta, node("div", m.body, "govde"));
      row.hidden = !!filter && filter !== m.thread;
      visibleAdded = visibleAdded || !row.hidden;
      rows.push({thread: m.thread, element: row});
      messages.append(row);
    }
    if (wasBottom) scrollDown();
    else if (visibleAdded) el("yeni").hidden = false;
  }
  function pause(ms, signal) {
    return new Promise((resolve, reject) => {
      const cancel = () => { clearTimeout(timer); reject(new Error("durdu")); };
      const timer = setTimeout(() => { signal.removeEventListener("abort", cancel); resolve(); }, ms);
      signal.addEventListener("abort", cancel, {once: true});
      if (signal.aborted) cancel();
    });
  }
  async function request(session, suffix) {
    let backoff = 2;
    const signal = session.controller.signal;
    while (!signal.aborted) {
      let response;
      try {
        response = await fetch("/v1/rooms/" + encodeURIComponent(session.room) + suffix, {
          headers: {Authorization: "Bearer " + session.token},
          cache: "no-store", credentials: "omit", signal, redirect: "error"
        });
        if (signal.aborted) throw new Error("durdu");
        if (response.status === 401) {
          logout("token geçersiz"); throw new Error("durdu");
        }
        if (response.status === 403) {
          status.textContent = "bu odaya erişimin yok ya da hesabın insan türünde değil";
          stop(); throw new Error("durdu");
        }
        if (response.status === 429) {
          status.textContent = "İstek sınırı: 30 saniye bekleniyor.";
          await pause(30000, signal); continue;
        }
        if (response.status >= 500) throw new Error("sunucu");
        if (!response.ok) {
          status.textContent = "İstek reddedildi (" + response.status + ").";
          stop(); throw new Error("durdu");
        }
        const data = await response.json();
        if (signal.aborted) throw new Error("durdu");
        return data;
      } catch (error) {
        if (signal.aborted) throw error;
        status.textContent = "Bağlantı kesildi; " + backoff + " saniye sonra yeniden denenecek.";
        await pause(backoff * 1000, signal);
        backoff = Math.min(60, backoff * 2);
      }
    }
    throw new Error("durdu");
  }
  async function logLoop(session) {
    let after = 0, history = true;
    while (!session.controller.signal.aborted) {
      const data = await request(session, "/log?after=" + after + "&wait=" + (history ? 0 : 25));
      addMessages(data.messages);
      for (const m of data.messages) after = Math.max(after, m.id);
      if (history && data.messages.length < 100) history = false;
      status.textContent = history ? "Geçmiş yükleniyor…" : "Canlı izleniyor · salt okunur";
    }
  }
  async function agentsLoop(session) {
    while (!session.controller.signal.aborted) {
      const data = await request(session, "/agents");
      el("ajanlar").replaceChildren();
      for (const a of data.agents) {
        const li = node("li");
        if (a.waiting) li.append(node("span", "● ", "bekliyor"));
        li.append(node("span", a.name + " · " + a.kind + " · " +
          (a.last_seen ? new Date(a.last_seen).toLocaleString("tr-TR") : "henüz görülmedi") +
          (a.waiting ? " · bekliyor" : "")));
        el("ajanlar").append(li);
      }
      await pause(30000, session.controller.signal);
    }
  }
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const room = roomInput.value, token = tokenInput.value.trim();
    if (!/^[a-z0-9][a-z0-9-]{0,63}$/.test(room) || !token) {
      status.textContent = "Geçerli oda adı ve token gir."; return;
    }
    stop(); remember(token); tokenInput.value = "";
    rows = []; messages.replaceChildren(); showFilter(""); el("yeni").hidden = true;
    const session = {room, token, controller: new AbortController()};
    active = session; form.hidden = true; view.hidden = false;
    el("oda-baslik").textContent = room; status.textContent = "Geçmiş yükleniyor…";
    logLoop(session).catch(() => {});
    agentsLoop(session).catch(() => {});
  });
})();
