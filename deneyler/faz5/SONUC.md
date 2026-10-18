# Faz 5 sonucu: arkadaşın bağlanması ✅ BAŞARILI

Tarih: 18.10.2026

## Kurulum

- Arkadaşın makinesi: Windows 11 Pro + Claude Code (masaüstü uygulaması). Ajan adı `ece-claude`.
- Ona gönderilenler: `relay-windows-amd64.exe` ve `SHA256SUMS` (Faz 4 sürümü, commit 3a3c94f)
  ile adım adım bir kurulum notu. Notta işlemci türü kontrolü, SHA-256 doğrulaması, PATH,
  `relay setup` ve CLAUDE.md'ye eklenecek kısa bir relay bölümü vardı. Relay bölümü bu repodaki
  [docs/ajan-talimati.md](../../docs/ajan-talimati.md) dosyasına bağlantı veriyor.
- Token ayrı ve güvenli bir kanaldan gönderildi. Sunucuda yalnız SHA-256 özeti tutuluyor.
- İlk bağlantı: `relay agents` listesinde `ece-claude` göründü (son görülme 17.10 23:31 UTC).

## Kabul testi

Kabul ölçütü: iki cihazdaki yönetici ajanlar bir `soru → cevap` ve bir `gorev → sonuc` döngüsünü
tamamlar. Her iki konuşmayı da `mehmet-claude` başlattı. Karşı taraf, kullanıcısı
"relay'i dinle" dedikten sonra `relay wait` ile bekliyordu.

| # | Konuşma | Sonuç |
|---|---|---|
| 1 | T-1 `soru`: hangi işletim sistemi ve araç, `relay version` çıktısı ne | ✅ `cevap` geldi, `--reply-to` ile. "Windows 11 Pro + Claude Code", sürüm `3a3c94f`, go1.27.0. Sürüm, gönderilen derlemeyle aynı |
| 2 | T-1 görgü | ✅ Cevaba "teşekkürler" ya da ek mesaj gelmedi. Konuşma `close` ile kapatıldı |
| 3 | T-2 `gorev`: `relay agents` çalıştır, odadaki ajanları bildir (salt okuma) | ✅ `sonuc` geldi: "Ece onayladı, relay agents çalıştırıldı. Odada 2 ajan var...". Bildirilen liste o anki durumla tutarlı (mehmet-claude o sırada bekliyordu) |
| 4 | T-2 onay | ✅ Karşı ajan, talimata uyarak görevi yapmadan önce kendi kullanıcısına sordu. Bu bilgi karşı ajanın kendi bildirimine dayanıyor, kullanıcı tarafından teyit edilmesi beklenir |

İki konuşma da 2/20 turda kapandı. Döngü oluşmadı, mesajlarda sır yoktu.

## Öğrenilenler

1. **Kurulum notu yeterli oldu.** Arkadaş, notu izleyerek yardım almadan kurdu ve bağlandı.
2. **Mesajlar sunucuda bekliyor.** `gorev`, karşı ajan dinlemeye başlamadan gönderildi. Ajan
   dinlemeye başlayınca mesajı aldı, yani iki tarafın aynı anda çevrimiçi olması gerekmiyor.
3. **Talimat başka bir makinede de tuttu.** Talimat CLAUDE.md'den GitHub bağlantısıyla
   veriliyordu. Karşı Claude doğru mesaj tiplerini kullandı, `--reply-to` verdi, cevaba
   karşılık yazmadı ve `gorev` için kullanıcısına sordu.

## Açık kalanlar

- `bridge` (Codex/agy) ile cihazlar arası konuşma henüz denenmedi. Faz 3'te yerelde çalışıyordu.
- Yol haritasındaki S3 açık soru: her `relay send` için izin sorulsun mu, yoksa izin listesine mi
  eklensin? Her kullanıcı bunu kendi izin ayarında karar verir.
