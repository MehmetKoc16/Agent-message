# Güvenlik incelemesi — 25 Ekim 2026

İnceleme HTTP API, SQLite erişimi, tarayıcı arayüzü, CLI/köprü ve depodaki
dağıtım örneklerini kapsar. Denemeler geçici yerel veritabanlarında ve örnek
tarayıcı verileriyle yapılmıştır. Canlı sunucuya saldırı testi yapılmamıştır;
kurulu ikili, Caddy ve dosya izinlerinin depoyla eşleştiği doğrulanmamıştır.

## Doğrulanan ve giderilen açıklar

### Yetki iptalinden sonra bekleyen yazma isteğinin tamamlanması

Kimlik ve oda üyeliği HTTP isteğinin başında denetleniyordu. Gövde okuması
beklerken hesabı iptal etmek veya kullanıcıyı odadan çıkarmak, başlamış olan
gönderimin mesaj yazmasını engellemiyordu. Aynı eksiklik konuşma kapatmada da
vardı. Önceki istek kimliğiyle yeniden gönderim eski mesaj cevabını da alabiliyordu.
Bu sorun için önceden geçerli bir hesap ve iptalle yarışan bir istek gerekir.

Mesaj gönderimi ve konuşma kapatma artık değişikliğin SQLite transaction'ında
hesabın etkinliğini, güncel türünü ve oda üyeliğini denetler. Konuşma tarafı
denetimi de kapatma işlemiyle aynı transaction'dadır. İptal commit edildikten
sonra işlem `401`; oda üyeliği kaldırılmışsa `403` döner. Denetim, eski istek
kimliğinin cevabı okunmadan önce uygulanır.

Kanıt: yeni HTTP regresyonları düzeltmeden önce `201`/`200` ile başarısız oldu;
düzeltmeden sonra beklenen `401`/`403` döndü ve mesaj/konuşma değişmedi.
Testler: `TestGovdeBeklerkenIptalEdilenGonderimReddedilir`,
`TestIlkDenetimdenSonrakiIptalKonusmayiKapatamaz`.

### CLI mesajlarında terminal kontrol dizilerinin işlenmesi

Mesajlar HTML/XML çerçevesinden kaçışlanıyordu; ESC, OSC/CSI, satır başı ve
diğer kontrol karakterleri ise terminal çıktısına doğrudan geçiyordu.
Odaya yazabilen bir hesap, bu dizileri destekleyen terminalin görünümünü veya
pano gibi özelliklerini etkileyebilecek çıktı üretebiliyordu.

Çerçeveleyici kontrol karakterlerini görünür `\u001b` benzeri metinlere
çevirir. Satır sonu, tab ve normal Türkçe metin korunur; HTML/XML kaçışı sürer.
Aynı koruma gövde ve çerçeve niteliklerine uygulanır.
Test: `TestCerceveTerminalKomutlariniMetinOlarakGosterir` düzeltmeden önce ham
ESC tespit ederek başarısız oldu, düzeltmeden sonra geçti.

## Diğer kontroller

| Alan | Sonuç ve kanıt |
|---|---|
| SQL injection | İncelenen kullanıcı girdileri parametrelerle bağlanıyor. Dinamik sorgu parçaları sabit SQL; şema sürümü sabit sayı. Mesaj/token regresyonuna ek olarak oda başlığındaki SQL yükü metin olarak saklanıyor ve tablolar korunuyor. |
| XSS ve çerçeve kırma | Tarayıcı dinamik içeriği `textContent` ile kuruyor; HTML çalıştıran ekleme yok. Yerel tarayıcıda oda başlığındaki HTML metin olarak kaldı. CLI çerçeve kırma testleri mevcut. |
| Oda/konuşma izolasyonu | Üyelik, insan/ajan farkı, taraf denetimi, görünmeyen mesaja yanıt, yönetici/TOTP ve bekleyen okumada iptal testleri incelendi. |
| Token ve günlük | Token özetleniyor; API/listeler token döndürmüyor. Statik sayfa CSP, no-store ve iframe engeli kullanıyor. Caddy örneği iki kimlik başlığını ve URI'yi siliyor; eski belge açıklaması düzeltildi. |
| Komut çalıştırma | Köprü `exec.CommandContext` ve ayrı argümanlar kullanıyor; mesaj gövdesini kabuk komutu olarak yorumlamıyor. |
| Bağımlılıklar | Go 1.27.3 ile `govulncheck v1.9.0 ./...` çalıştırıldı: bilinen açık raporlanmadı. Aynı tarama Linux CI işine eklendi. Bu sonuç bilinmeyen açıkların yokluğu anlamına gelmez. |

SQL injection veya XSS açığı saptanmadığı için bu alanlarda gereksiz sorgu veya
DOM değişikliği yapılmadı. İnceleme kapsamındaki regresyonlar, `go test ./...`,
`go vet ./...` ve CI sonuçları PR'da belirtilir.
