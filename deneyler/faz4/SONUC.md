# Faz 4 sonucu: sunucuya kurulum ✅ BAŞARILI

Tarih: 16.10.2026, sunucu sahibinin onayladığı bakım penceresinde.
Sunucuya özel ayrıntılar (alan adı, IP, dizin listesi) bu public repoda değil, yerelde tutulur.

## Süreç

1. **İnceleme ve onay.** Paket, sunucudaki canlı uygulamanın sahibi olan tarafa (başka bir Claude
   oturumu) incelendi. Koşullu onay iki zorunlu düzeltmeyle geldi:
   - **A1:** `IPAddressAllow=localhost` relay'in aynı makinedeki yerel portlara, özellikle kimlik
     doğrulamasız Caddy yönetim API'sine bağlanmasına izin veriyordu. Çözüm: systemd soket
     etkinleştirmesiyle Unix soketi, `PrivateNetwork=yes`, `IPAddressDeny=any`, `AF_UNIX`.
     Relay'in hiçbir IP erişimi yok.
   - **A2:** İlk sertifika 15 sn'den uzun sürebileceği için `caddy-ekle.sh` gereksiz geri dönüş
     yapardı. Artık yalnız canlı uygulamanın sağlığı bozulursa geri alıyor. DNS ön kontrolü eklendi.
   - Ayrıca öneri C: kimliksiz istekler için IP başına ve genel sınır.
2. **Kurulum** (`kurulum.sh`): kuru çalışma → drop-in (diğer uygulamaların 15 dizini
   `InaccessiblePaths`) → kuru çalışma → `--uygula`.
3. **Caddy** (`caddy-ekle.sh`, sunucu sahibi çalıştırdı): başlangıç ölçümü → DNS → kuru çalışma
   diff'i (yalnız sona ekleme) → kullanıcı onayı → `--uygula` → sahibin kontrolleri.

## Sonuçlar

| Ölçüm | Sonuç |
|---|---|
| Yerel sağlık (Unix soketi) | ✅ `{"ok":true}` |
| `systemd-analyze security` | ✅ **0.2 SAFE.** ✗ olarak yalnız RootDirectory ve AF_UNIX kalıyor; ikisi de tasarım gereği |
| TCP dinleyicisi | ✅ Yok. Yalnız `/run/agent-relay/relay.sock` |
| Bellek | ✅ 2,7 MB, tepe 3,2 MB (sınır 100 MB) |
| Drop-in | ✅ Yüklü, 15 yol etkin (`systemctl show -p InaccessiblePaths`) |
| HTTPS `/v1/health` | ✅ 200; `/` → 404; token'sız istek → 401; HSTS, nosniff, no-referrer başlıkları var |
| Canlı uygulama | ✅ API ve site 200, www yönlendirmesi, API sertifikası değişmedi, Caddy journal'ı temiz |
| İlk istemci | ✅ `relay setup` → "bağlantı tamam", `relay agents` |

## Olay: Caddy log dosyası sahipliği

İlk `caddy-ekle.sh --uygula` reload adımında `permission denied` ile başarısız oldu. Sebep:
betik `caddy validate`'i root olarak çalıştırıyordu. Validate, log dosyasını `root:root 0600` olarak
oluşturdu; Caddy servisi (User=caddy) bu dosyaya yazamadı. **Otomatik geri alma doğru çalıştı,
canlı uygulama kesilmedi.** Sunucu sahibi dosyanın sahipliğini düzeltti, ikinci deneme başarılı oldu.
Betik, validate'den önce log dosyasını doğru sahiplikle oluşturacak şekilde düzeltildi.

## Öğrenilenler

1. **İnceleme işe yaradı.** Canlı sunucunun sahibi, paketi hazırlayanların göremediği bir riski
   (yerelde açık, kimlik doğrulamasız yönetim API'si) buldu. Ortak sunucuda onay süreci bir
   formalite değil.
2. **Önce kuru çalışma, geri alma ve sağlık kontrolü** sayesinde, gerçek bir hata canlı
   uygulamayı etkilemeden atlatıldı.
3. **Root ile yapılan doğrulamalar dosya sahipliği bırakabilir.** Bir servisin yazacağı dosyalar,
   doğrulamadan önce o servisin kullanıcısıyla oluşturulmalı.
4. `systemd-analyze security` kurulum betiğinde sayfalayıcı açabiliyor; betiklerde `--no-pager`
   kullanılmalı.

## Açık kalanlar

- ✅ (17.10.2026) Sunucu yeniden başlatıldı (çekirdek güncellemesi). Relay soket etkinleştirmesiyle
  kendiliğinden kalktı; Unix soketi ve HTTPS üzerinden sağlık 200, drop-in yüklü, canlı uygulama sağlıklı.
- `go test -race` bu makinede çalıştırılamadı (cgo yok).
