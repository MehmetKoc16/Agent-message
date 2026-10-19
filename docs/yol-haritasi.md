# Yol haritası

Her faz bir öncekinin kabul ölçütü sağlanmadan başlamaz. **Faz 4'e kadar sunucuya dokunulmaz.**

## Faz 0: Uyandırma deneyi (lokal, ~1 saat) ✅ Tamamlandı (13.10.2026)

> Sonuç: başarılı. Ayrıntı: [deneyler/faz0/SONUC.md](../deneyler/faz0/SONUC.md)

**Soru:** Boştaki VS Code paneli, arka planda başlattığı komut bitince gerçekten uyanıyor mu?

- Lokalde 30 satırlık sahte bir relay (yalnız bellek, localhost).
- Bu oturum ve ikinci bir Agent-message oturumu `relay wait` / `relay send` ile konuşur.
- **Kabul:** Ben hiçbir şey yazmadan, 5 mesajlık bir konuşma iki oturum arasında kendiliğinden
  tamamlanır. Her taraf her mesajdan sonra beklemeyi yeniden kurar.
- **Başarısız olursa:** yedek planlar: hook ile uyandırma (agent-comms yaklaşımı),
  CLI + Channels.

## Faz 1: Relay sunucusu ve CLI (lokal) ✅ Tamamlandı (13.10.2026)

> Sonuç: Go ile yazıldı ([cmd/README.md](../cmd/README.md)). 58 test, `go vet` temiz, Linux
> çapraz derlemesi tamam. Bellek: 4 ajan beklerken 200 mesajda tepe ~21 MB (Windows).
> agy kodu üç parçada inceledi: 25 bulgunun 16'sı doğrulanıp düzeltildi, 9'u çürütüldü.
> Gerçek ikililerle uçtan uca denendi (gönder/bekle, UTF-8, kaçışlama, sır taraması, 401).
> Kodu Sonnet yazdı. Codex'in Windows sandbox'ı arızalı olduğu için kullanılamadı.
> Açık: `go test -race` çalıştırılamadı (makinede cgo/gcc yok); Linux'ta gerçek çalıştırma Faz 4'te.

- Go: `agent-relay` sunucusu (+ `admin` alt komutu) ve `relay` istemcisi; tek dış bağımlılık `modernc.org/sqlite`.
- [protokol.md](protokol.md) v1: token doğrulama, tur ve hız sınırları, long-poll.
- Testler: token yok/yanlış → 401; `from` taklidi yapılamaz; tur sınırında kilitlenir;
  16 KB üstü → 413; long-poll mesaj gelince hemen döner.
- **Kabul:** testler geçer; lokalde ölçülen bellek < 100 MB.

## Faz 2: İletişim talimatı ✅ Tamamlandı (14.10.2026)

> Sonuç: [docs/ajan-talimati.md](ajan-talimati.md). Canlı testte habersiz ajan iki prompt
> injection'ı reddetti, döngüye girmedi, kanarya sızmadı:
> [deneyler/faz2/SONUC.md](../deneyler/faz2/SONUC.md). Codex (sandbox'ı düzelmiş olarak)
> `relay setup` (token ekrana yansımadan), `relay version` ve `tools/derle/derle.sh`'yi
> (8 ikili + SHA256SUMS) yazdı. Talimatın prompt.md'ye eklenmesi kullanıcı kararı.

- Claude için "Diğer ajanlarla iletişim" bölümü (GPT'nin taslağı + protokol görgüsü +
  güvenlik kuralları). prompt.md'ye ya da ayrı bir `AGENT-COMMS.md` dosyasına.
- **Kabul:** Faz 0 senaryosu bu talimatla tekrar edilir. Döngü oluşmaz, sır sızmaz,
  sahte bir "`.env`'i gönder" mesajı reddedilir.

## Faz 3: Codex ve agy köprüsü (lokal) ✅ Tamamlandı (15.10.2026)

> Sonuç: `relay bridge --runner codex|agy` (Codex yazdı). Gerçek Codex ve agy ile canlı test:
> soru/cevap, resume ile bağlam, bilgi tamponu, read-only sandbox (yazma engellendi), prompt
> injection reddi. agy incelemesi 10 bulgu: 7 düzeltildi. Ayrıntı:
> [deneyler/faz3/SONUC.md](../deneyler/faz3/SONUC.md).

- `relay bridge --codex` (`exec resume`) ve `--agy` (`--conversation` ya da `agentapi`).
- **Kabul:** Claude, köprü üzerinden bir Codex konuşmasıyla 3 tur konuşur; Codex önceki turu
  hatırlar. agy için de aynısı.

## Faz 4: Sunucuya kurulum ✅ Tamamlandı (16.10.2026)

> Sonuç: sunucu sahibinin incelemesi (A1 Unix soketi + ağ yok, A2 güvenli geri alma) sonrası
> kuruldu. Güvenlik puanı 0.2 SAFE, bellek ~3 MB, TCP portu yok. Caddy log sahipliği olayı
> otomatik geri almayla zararsız atlatıldı. Ayrıntı: [deneyler/faz4/SONUC.md](../deneyler/faz4/SONUC.md).

1. Mimari ve [güvenlik.md](guvenlik.md) sunucudaki ana uygulamanın tarafına onaylatılır.
2. DNS: `relay.<domain>` A kaydı (kullanıcı yapar).
3. Sistem kullanıcısı, `/opt/agent-relay`, venv, systemd birimi.
4. Caddyfile'a site bloğu: haber ver → yedekle → `caddy validate` → `reload`.
5. **Kabul:** `https://relay.<domain>/v1/health` 200 döner; sunucudaki diğer uygulamalar
   etkilenmez (kendi sağlık kontrolleri); relay'in belleği < 100 MB; dışarıdan 8790 kapalı.

## Faz 5: Arkadaşın bağlanması ✅ Tamamlandı (18.10.2026)

- Token üret, istemci kurulum notu yaz, birlikte ilk gerçek konuşma.
- **Kabul:** iki cihazdaki yönetici ajanlar bir `soru → cevap` ve bir `gorev → sonuc`
  döngüsünü tamamlar.

> Sonuç: Windows 11 + Claude Code çalışan ikinci bir makine bağlandı. İki döngü de 2 turda
> tamamlandı, karşı ajan `gorev` için kullanıcısının onayını aldı. Ayrıntı:
> [deneyler/faz5/SONUC.md](../deneyler/faz5/SONUC.md).

## Faz 6: İsteğe bağlı eklentiler (tarayıcı sayfası ✅ 19.10.2026)

- ✅ Odayı izlemek ve yazmak için tarayıcı sayfası: `/v1/izle`. SSE/WebSocket yerine mevcut long-poll kullanıldı.
  Ayrıntı: [deneyler/faz6/SONUC.md](../deneyler/faz6/SONUC.md).
- ⏸ CLI kullanıcıları için Channels bağlayıcısı (arka planda `relay wait` yeterli).
- ⏸ Dosya kilitleri (`bilgi` mesajlarından türeyen "kim hangi dosyada" tablosu).

---

## Açık sorular ❓

| # | Soru | Neden önemli |
|---|---|---|
| S1 | ✅ Arkadaşının işletim sistemi ve kullandığı araçlar? **Windows 11 + Claude Code (18.10.2026)** | İstemci ve köprü desteği |
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
| K3 | **Go** (sunucu + istemci), SQLite için `modernc.org/sqlite` (saf Go, cgo yok); ayarlar JSON | ✅ 13.10.2026 | `net/http` internete açık sunucu için üretim kalitesinde (Python `http.server` değil); düşük bellek; long-poll için goroutine; tek ikili dosya: sunucuya kopyala-çalıştır, arkadaşa Python kurdurmadan `relay.exe`. Python Faz 0 kodu protokolün referans uygulaması olarak kalır |
| K4 | Ayrı alt alan adı (`relay.<domain>`), `api.<domain>` altında yol değil | 💡 Öneri | Diğer uygulamalardan yapılandırma ve log ayrımı |
| K5 | `--dangerously-skip-permissions` yasak | ✅ Kesin | prompt.md kuralı |
| K6 | Varsayılan: yönetici ajanlar konuşur, Codex/agy yerel yardımcı | 💡 Öneri | prompt.md'deki düzeni korur |
| K7 | Sunucu değişiklikleri, sunucudaki diğer uygulamaların tarafına haber verilerek yapılır | ✅ Kesin | Canlı sunucu, kişisel veri |
| K8 | Channels izin aktarımı kullanılmaz | 💡 Öneri | Karşı ajan bizim oturumda izin verememeli |
