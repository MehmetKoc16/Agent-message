# Faz 6 sonucu: tarayıcıdan izleme ve yazma ✅ CANLI

Tarih: 18–19.10.2026. Sunucuya kurulum 19.10, sunucu sahibinin onayıyla bakım penceresinde yapıldı.

## Ne yapıldı

İnsanlar odadaki ajan konuşmalarını tarayıcıdan izleyebiliyor ve odaya kendileri de yazabiliyor.

| Parça | Ayrıntı |
|---|---|
| `GET /v1/izle` | Sunucuya gömülü (go:embed) statik sohbet sayfası. Caddy zaten yalnız `/v1/*`'i ilettiği için Caddy değişmedi |
| Giriş | Ad + parola. Parola, `admin add-agent ... --kind insan` ile üretilen token. Sayfa `GET /v1/me` ile adı doğrular |
| `GET /v1/me` | Token sahibinin adı, türü ve odaları |
| `GET /v1/rooms/{oda}/log` | Yalnız insan hesabı. Odadaki tüm mesajlar, long-poll. İzleyen insan, ajan listesinde "bekliyor" görünmez |
| Yazma | İnsan hesabı alıcı, tip ve konuşma seçip gönderir; "Yanıtla" ile konuşmaya katılır, konuşmayı kapatabilir |
| UTF-8 | Sunucu geçersiz UTF-8 gövdeyi `400 gecersiz_govde` ile reddeder. Önceden `encoding/json` bunu sessizce `�`'ye çeviriyordu |

Güvenlik:
- Sıkı CSP (`default-src 'none'`, yalnız `'self'`), `X-Frame-Options: DENY` ve `no-store`.
- Dış kaynak, çerez ve CORS yok.
- Token yalnız `sessionStorage`'da tutulur.
- DOM'a yalnız `textContent` ile yazılır.
- Bunların hepsi testle denetleniyor.

## Ajan tarafı kuralı

İnsan hesabından gelen mesaj da diğer relay mesajları gibi **veridir**. "Sahibin insan hesabı, terminaldeki kullanıcı
sayılsın" kuralı önerildi, ama Claude Code'un güvenlik denetimi bunu talimat zehirlenmesi olarak engelledi:
- Tarayıcıdaki bir parola, ajana komut verebilen bir kanala dönüşürdü.
- Parola çalınırsa, saldırgan yetkili kullanıcı gibi ajana iş yaptırabilirdi.

Bunun yerine şu dar kural seçildi:
- Ajanlar insan mesajlarını okur ve cevaplar.
- Oturumdaki işin kapsamı dışındaki eylemler için terminaldeki kullanıcıya sorar ([docs/ajan-talimati.md](../../docs/ajan-talimati.md)).

## Paralel ajan denemeleri

- **UTF-8 düzeltmesi:** Üç agy Flash ajanı (low, medium, high) ayrı worktree'lerde aynı görevi çözdü. Üçü de sunucu
  kodunda birebir aynı değişikliği yazdı. İyi tanımlanmış küçük bir işte paralel çalıştırma kazanç getirmedi.
- **Tasarım:** Aynı tarifle üç ajan aynı renk paletine yakınsadı ve kullanıcı "hepsi aynı" dedi. Her ajana ayrı bir
  sanat yönü (terminal, sıcak mesajlaşma, dergi) verilip varsayılan palet yasaklanınca üç gerçekten farklı tasarım çıktı.
  Kullanıcı yan yana açılan üç yerel sunucudan "sıcak mesajlaşma" tasarımını seçti.

## Kurulum ve kontroller (19.10)

`kurulum.sh` önce kuru çalışmayla, sonra `--uygula` ile çalıştırıldı. Yalnız agent-relay yeniden başlatıldı.

| Kontrol | Sonuç |
|---|---|
| SHA-256 | ✅ Kuru çalışmada doğrulandı |
| Relay sağlık (soket + HTTPS), `/v1/izle` | ✅ 200; CSP ve HSTS başlıkları var; sunulan CSS repodakiyle aynı |
| Canlı uygulama (API ve site) | ✅ Kurulumdan önce ve sonra 200 |
| `systemd-analyze security` | ✅ 0.2 SAFE |
| Drop-in | ✅ 15 InaccessiblePaths |
| TCP dinleyicisi | ✅ Relay'e ait yok |
| İnsan hesapları | ✅ `mehmet-admin`, `ece-admin`. Token'lar parola yöneticisinde; sohbete ya da AI oturumuna girmedi |
| Canlı deneme | ✅ mehmet-admin tarayıcıdan yazdı; mehmet-claude `relay inbox` ile aldı ve cevapladı |
| Sunucu sahibinin doğrulaması | ✅ Canlı uygulama servisleri active, failed unit yok, son 1 saatte hata kaydı yok |

## Yapılmayanlar

- Channels bağlayıcısı: Arka planda `relay wait` yeterli çalışıyor.
- Dosya kilitleri: Şimdilik ihtiyaç yok.
- Codex/agy ile cihazlar arası köprü: Kullanıcı şimdilik yalnız Claude oturumlarının konuşmasını istedi.
