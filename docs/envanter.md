# Envanter: elimizde ne var?

Son güncelleme: 11.10.2026

## 1. Lokal makine (benim PC, Windows 11 Pro)

| Araç | Sürüm | Konum / not | Durum |
|---|---|---|---|
| Claude Code | 2.1.276 | VS Code eklentisi (sohbet paneli) olarak kullanılıyor | ✅ |
| Codex CLI | codex-cli 0.159.2 | `/c/Users/pc/AppData/Local/OpenAI/Codex/bin/c6fe824d725f02d7/codex.exe` | ✅ |
| agy (Antigravity CLI) | 1.3.3 | `/c/Users/pc/AppData/Local/agy/bin/agy.exe` | ✅ |
| Python | 3.12.6 | | ✅ |
| Node.js | 22.15.0 | | ✅ |
| Bun | 1.3.14 | | ✅ |
| Go | 1.27.0 | `C:\Program Files\Go\bin\go.exe` (13.10.2026'da winget ile kuruldu) | ✅ |

> ⚠️ **prompt.md'deki Codex yolu eskidi.** prompt.md'de `.../bin/cdef5aaf3e41ab53/codex.exe`
> yazıyor, güncel klasör `c6fe824d725f02d7`. Uygulama güncellendikçe bu klasör adı değişiyor.
> Kodda yol sabit yazılmamalı, `find` ile bulunmalı.

> ⚠️ **Codex sandbox arızası (13.10.2026, codex-cli 0.159.2):** `--sandbox workspace-write`
> ile hiçbir komut başlamıyor; boş bir klasörde `echo` bile çalışmıyor. Hata:
> `Failed to create unified exec process: helper_unknown_error: apply deny-read ACLs`.
> Codex dosya düzenleyemiyor ve test çalıştıramıyor. Codex güncellenene ya da düzeltilene
> kadar kod yazımı Sonnet'te. `danger-full-access` kullanılmıyor (güvenlik kuralı).

### Ajanların bu iş için önemli yetenekleri (yardım çıktılarından ✅)

**Codex CLI**
- `codex exec "<prompt>"`: tek seferlik, başsız çalışma.
- `codex exec resume <SESSION_ID> "<prompt>"` / `--last`: **önceki Codex konuşmasını sürdürür.**
  Karşı ajandan gelen her mesajı aynı Codex konuşmasına ekleyebiliriz, bağlam kaybolmaz.
- `codex app-server`: deneysel; uzun yaşayan sunucu ve konuşma protokolü. İlk sürümde gerek yok.

**agy**
- `-p/--print`: tek seferlik, başsız çalışma.
- `--conversation <ID>` / `-c/--continue`: önceki konuşmayı sürdürür.
- `--input-format stream-json --output-format stream-json`: stdin'den satır satır mesaj alıp
  her biri için bir tur çalıştırır. Tek bir uzun süreçle sürekli konuşma mümkün olabilir. ⚠️ denenmedi
- `agy agentapi send-message <recipient_id> <content>`: çalışan bir agy konuşmasına dışarıdan
  mesaj gönderme. EcoRelay bunu kullanıyor. ⚠️ `recipient_id`'nin ne olduğu denenmedi.
- `--remote-control`, `remote-control` daemon: kendi uzaktan kontrol özelliği. ⚠️ incelenmedi.

**Claude Code**
- Aynı makinedeki oturumlar birbirine mesaj atabiliyor ve boştaki VS Code paneli gelen mesajla
  **kendiliğinden uyanıyor.** ✅ Gözlendi: 11.10'da bu oturum, başka iki projedeki
  oturumların mesajlarıyla uyandı. Bu yerel ve dahili bir mekanizma, makineler arası çalışmaz.
- Arka planda başlatılan bir komut (`run_in_background`) bittiğinde oturum uyanıyor.
  ✅ Faz 0'da (13.10.2026) VS Code panelinde boştayken uyandığı doğrulandı
  ([SONUC.md](../deneyler/faz0/SONUC.md)).
- Channels: MCP sunucusunun çalışan oturuma mesaj itmesi. Araştırma önizlemesinde; kendi
  channel'ımız için `--dangerously-load-development-channels` bayrağı gerekiyor. Ayrıntı:
  [arastirma.md](arastirma.md#claude-code-channels).

## 2. Sunucu

Relay, **başka canlı uygulamaların da çalıştığı** bir VPS'e kurulacak. Sunucudaki uygulamalardan
biri kişisel veri tutuyor. Bu yüzden sunucunun ayrıntıları (servisler, iç portlar, dosya yolları,
deploy ve yedek düzeni) **bu public repoda tutulmaz.** Ayrıntılar yalnız lokalde,
`local/sunucu.md` dosyasında durur; bu dosya `.gitignore`'da.

Relay tasarımını etkileyen genel bilgiler:

| Konu | Bilgi |
|---|---|
| İşletim sistemi | Ubuntu 24.04 LTS |
| Kaynak | Küçük VPS (2 vCPU, ~4 GB RAM); RAM'in çoğu diğer uygulamalarda |
| Servis yönetimi | **Docker yok**, her şey systemd |
| Reverse proxy | Caddy 2.x, TLS'i Let's Encrypt'ten otomatik alıyor |
| Dışa açık portlar | Yalnız 22, 80, 443 |
| SSH | Yalnız anahtarla giriş |

### Relay'e bıraktığı alan

- RAM bütçesi: **≤ 100 MB hedef**, systemd `MemoryMax=150M` ile sınırlı.
- Port: dışarı yeni port **açılmaz**. Relay `127.0.0.1:<port>`'ta dinler, dışarıya Caddy açar.
- Alan adı: 💡 `relay.<domain>`. DNS A kaydı ve Caddyfile'a yeni bir site bloğu gerekir.
- Python: Ubuntu 24.04'te python3.12 sistemde hazır.

### Değişiklik protokolü

Caddyfile, sunucudaki canlı uygulamanın da yapılandırması. Hatalı bir yapılandırma o
uygulamayı düşürür.

1. Caddyfile değişikliği, servis yeniden başlatma, paket kurulumu ya da sshd değişikliğinden
   önce **sunucudaki diğer uygulamaların sahiplerine** (onların ajan oturumlarına, kullanıcı
   üzerinden) haber verilir. Kimlere haber verileceği `local/sunucu.md`'de yazıyor.
2. Caddyfile değişikliği: yedek al → `caddy validate` → `systemctl reload caddy`.
   **restart kullanılmaz.**
3. Sunucu kararlarının sahibi sunucudaki ana uygulamanın tarafıdır. Son mimari kuruluma
   geçmeden önce ona onaylatılır.

## 3. Arkadaşın makinesi

❓ Bilinmiyor: işletim sistemi, kullandığı araçlar (Claude Code panel/CLI, Codex, agy, Cursor…).
