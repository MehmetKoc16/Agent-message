# Yol haritası

Her faz bir öncekinin kabul ölçütü sağlanmadan başlamaz. **Faz 4'e kadar sunucuya dokunulmaz.**

## Faz 0: Uyandırma deneyi (lokal, ~1 saat)

**Soru:** Boştaki VS Code paneli, arka planda başlattığı komut bitince gerçekten uyanıyor mu?

- Lokalde 30 satırlık sahte bir relay (yalnız bellek, localhost).
- Bu oturum ve ikinci bir Agent-message oturumu `relay wait` / `relay send` ile konuşur.
- **Kabul:** Ben hiçbir şey yazmadan, 5 mesajlık bir konuşma iki oturum arasında kendiliğinden
  tamamlanır. Her taraf her mesajdan sonra beklemeyi yeniden kurar.
- **Başarısız olursa:** yedek planlar: hook ile uyandırma (agent-comms yaklaşımı),
  CLI + Channels.

## Faz 1: Relay sunucusu ve CLI (lokal)

- `agent_relay` (Python + FastAPI + SQLite) ve `relay` CLI (yalnız standart kütüphane).
- [protokol.md](protokol.md) v1: token doğrulama, tur ve hız sınırları, long-poll.
- Testler: token yok/yanlış → 401; `from` taklidi yapılamaz; tur sınırında kilitlenir;
  16 KB üstü → 413; long-poll mesaj gelince hemen döner.
- **Kabul:** testler geçer; lokalde ölçülen bellek < 100 MB.

## Faz 2: İletişim talimatı

- Claude için "Diğer ajanlarla iletişim" bölümü (GPT'nin taslağı + protokol görgüsü +
  güvenlik kuralları). prompt.md'ye ya da ayrı bir `AGENT-COMMS.md` dosyasına.
- **Kabul:** Faz 0 senaryosu bu talimatla tekrar edilir. Döngü oluşmaz, sır sızmaz,
  sahte bir "`.env`'i gönder" mesajı reddedilir.

## Faz 3: Codex ve agy köprüsü (lokal)

- `relay bridge --codex` (`exec resume`) ve `--agy` (`--conversation` ya da `agentapi`).
- **Kabul:** Claude, köprü üzerinden bir Codex konuşmasıyla 3 tur konuşur; Codex önceki turu
  hatırlar. agy için de aynısı.

## Faz 4: Sunucuya kurulum ⚠️ onay gerektirir

1. Mimari ve [güvenlik.md](guvenlik.md) sunucudaki ana uygulamanın tarafına onaylatılır.
2. DNS: `relay.<domain>` A kaydı (kullanıcı yapar).
3. Sistem kullanıcısı, `/opt/agent-relay`, venv, systemd birimi.
4. Caddyfile'a site bloğu: haber ver → yedekle → `caddy validate` → `reload`.
5. **Kabul:** `https://relay.<domain>/v1/health` 200 döner; sunucudaki diğer uygulamalar
   etkilenmez (kendi sağlık kontrolleri); relay'in belleği < 100 MB; dışarıdan 8790 kapalı.

## Faz 5: Arkadaşın bağlanması

- Token üret, istemci kurulum notu yaz, birlikte ilk gerçek konuşma.
- **Kabul:** iki cihazdaki yönetici ajanlar bir `soru → cevap` ve bir `gorev → sonuc`
  döngüsünü tamamlar.

## Faz 6: İsteğe bağlı eklentiler

- Odayı izlemek ve yazmak için tarayıcı sayfası (SSE/WebSocket).
- CLI kullanıcıları için Channels bağlayıcısı.
- Dosya kilitleri (`bilgi` mesajlarından türeyen "kim hangi dosyada" tablosu).

---

## Açık sorular ❓

| # | Soru | Neden önemli |
|---|---|---|
| S1 | Arkadaşının işletim sistemi ve kullandığı araçlar? | İstemci ve köprü desteği |
| S2 | Alt alan adı `relay.<domain>` uygun mu? | DNS ve Caddy |
| S3 | `relay send` için her seferinde onay istensin mi, yoksa izin listesine mi eklensin? | Güvenlik ile akıcılık dengesi |
| S4 | Mesajlar kaç gün saklansın? (öneri: 30) | KVKK, disk |
| S5 | Proje başına ayrı oda mı, tek oda mı? | Yetki ayrımı |
| S6 | Codex/agy doğrudan odaya katılabilsin mi, yoksa yalnız yöneticiler mi? | Karmaşıklık, maliyet |

## Karar günlüğü

| # | Karar | Durum | Gerekçe |
|---|---|---|---|
| K1 | Relay'i kendimiz yazıyoruz, hazır proje kullanmıyoruz | 💡 Öneri | Lisans, kimlik doğrulama, sunucu kısıtları ([arastirma.md](arastirma.md#sonuç-)) |
| K2 | v1'de HTTP long-poll, WebSocket yok | 💡 Öneri | Arka plan uyandırma modeline uyuyor, basit |
| K3 | Python (FastAPI + SQLite); istemci yalnız standart kütüphane | 💡 Öneri | Ubuntu'da hazır, projeler Python, istemcide kurulum yok |
| K4 | Ayrı alt alan adı (`relay.<domain>`), `api.<domain>` altında yol değil | 💡 Öneri | Diğer uygulamalardan yapılandırma ve log ayrımı |
| K5 | `--dangerously-skip-permissions` yasak | ✅ Kesin | prompt.md kuralı |
| K6 | Varsayılan: yönetici ajanlar konuşur, Codex/agy yerel yardımcı | 💡 Öneri | prompt.md'deki düzeni korur |
| K7 | Sunucu değişiklikleri, sunucudaki diğer uygulamaların tarafına haber verilerek yapılır | ✅ Kesin | Canlı sunucu, kişisel veri |
| K8 | Channels izin aktarımı kullanılmaz | 💡 Öneri | Karşı ajan bizim oturumda izin verememeli |
