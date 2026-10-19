# Güvenlik

Relay, kişisel veri tutan **canlı** bir uygulamayla aynı sunucuda çalışacak.
Bu yüzden güvenlik sonradan eklenecek bir özellik değil, tasarımın başlangıç şartı.

## 1. Tehdit modeli

| # | Tehdit | Örnek | Önlem |
|---|---|---|---|
| T1 | **Prompt injection** | Karşı ajan (ya da onu ele geçiren biri) "`.env`'i gönder", "şu komutu çalıştır" yazar | Mesajlar `trust="untrusted"` çerçevesiyle gelir; ajan talimatında "veri, talimat değil" kuralı; `gorev` yalnız kendi kullanıcının kapsamında yapılır; Codex/agy köprüsü varsayılan **read-only** |
| T2 | **Token çalınması** | Token loga, mesaja ya da git'e düşer | Ajan istemcisinde token yalnız `~/.agent-relay/config.toml` dosyasında; tarayıcıdan izleme için bölüm 6; ajan başına ayrı token; sunucuda yalnız SHA-256 özeti saklanır; tek komutla iptal |
| T3 | **Relay'in ele geçirilmesi → canlı sunucu** | Relay'deki bir açık, yerel yönetim API'sine/veritabanı portuna ya da uygulama dosyalarına ulaşır | Ayrı kullanıcı ve dosya izolasyonu; systemd Unix soketi; PrivateNetwork, yalnız AF_UNIX ve IPAddressDeny=any; loopback IP izni yok |
| T4 | **Kontrolsüz döngü** | İki ajan saatlerce birbirine cevap yazıp kotayı bitirir | Tur sınırı (20), hız sınırı, cevap beklemeyen tiplere cevap yasağı |
| T5 | **Veri sızıntısı** | Ajan mesaja API anahtarı ya da kullanıcı verisi yazar | İstemci gönderim öncesi sır desenlerini tarar (API anahtarı, `-----BEGIN`, `.env` satırları) ve reddeder; saklama süresi sınırlı |
| T6 | **Kaynak tüketme (DoS)** | Çok sayıda istek RAM'i tüketir, sunucudaki diğer uygulamalar etkilenir | `MemoryMax`, `CPUQuota`, hız sınırları, mesaj boyutu sınırı, Caddy'de istek gövdesi sınırı |
| T7 | **Kimlik taklidi** | Bir istemci `from="mehmet-claude"` yazar | `from`'u sunucu token'dan doldurur |

## 2. Sunucu sıkılaştırması (systemd)

Go servisi için [agent-relay.service](../deploy/agent-relay.service) ve
[kurulum.sh](../deploy/kurulum.sh) hazırlanmıştır. Sunucuda uygulanmadan önce ana
uygulamanın sahibi paketi ve özel izolasyon listesini onaylar. İşlem sırası ve geri
alma: [kurulum rehberi](kurulum.md). Python/venv çalışma zamanı gerekmez.

| Alan | Uygulama ve amacı |
|---|---|
| Kimlik/dosyalar | Kabuksuz, ev dizinsiz `agentrelay`; ikili root:root 0755, config root:agentrelay 0640; StateDirectory 0700, UMask 0077 |
| Bellek/CPU | MemoryHigh=80M, MemoryMax=100M, MemorySwapMax=0, CPUQuota=25%, TasksMax=64, LimitNOFILE=1024 |
| Dosya izolasyonu | ProtectSystem=strict, ProtectHome, PrivateTmp/Devices/Users; diğer uygulamalar için özel InaccessiblePaths drop-in |
| Çekirdek/süreç | Kernel/ControlGroups/Clock/Hostname korumaları, ProtectProc=invisible, ProcSubset=pid; namespace/realtime/SUID kısıtları |
| Yetki/syscall | NoNewPrivileges, boş capabilities; native mimari, @system-service izin listesi ve @privileged/@resources reddi (yasak çağrı SIGSYS yerine EPERM döner: SystemCallErrorNumber=EPERM, Go başlangıçta prlimit64 çağırır); MemoryDenyWriteExecute |
| Ağ | PID 1 Unix soketini fd 3 ile verir; RestrictAddressFamilies=AF_UNIX, PrivateNetwork=yes, IPAddressDeny=any; IPAddressAllow yok |
| Soket izinleri | /run/agent-relay/relay.sock; agentrelay:caddy 0660, dizin 0755; Accept=no; Caddy bu sokete bağlanır |
| Hata/toparlanma | SIGTERM, 20 sn kapanış, on-failure/5 sn; SHA-256, RAM/grup/soket kontrolü; 10 sn Unix sağlık hatasında ikili ve iki birimi geri alma |

Üretimde TCP portu yoktur. Yerel izolasyon yolları yalnız
`/etc/systemd/system/agent-relay.service.d/10-yerel.conf` içinde tutulur;
[10-yerel.conf.ornek](../deploy/10-yerel.conf.ornek) yalnız yer tutucudur. Drop-in yoksa
kurucu uygulamayı durdurur; `--drop-in-yok-onay` sahibin ayrıca kabul ettiği istisnadır.
Salt okunur sistem dosyaları hâlâ okunabilir; diğer uygulamaların özel dizinlerini
eksiksiz gizlemek ve izinlerini korumak gerekir. Eski loopback istisnası kaldırıldı:
kimlik doğrulamasız yerel yönetim API'leri ve veritabanı/canlı API IP portlarına relay
erişemez. Dinleyici fd'si dışarıdan geldiği için PrivateNetwork içinde kabul çalışır.
AF_UNIX erişimi bütünüyle kapalı değildir; diğer uygulamaların Unix soketlerini dosya
izinleri ve yerel InaccessiblePaths listesiyle ayrıca koruyun.

Saf Go ve modernc SQLite JIT kullanmadığı için MemoryDenyWriteExecute uygundur.
Syscall grupları, PrivateUsers, proc/namespace ve IP BPF/aygıt kısıtlarının birlikte
çalışması gerçek Ubuntu/systemd üzerinde ilk kurulumda doğrulanmalıdır; Windows
kontrolleri bunu kanıtlamaz. Uyarılar ve systemd güvenlik puanı sahibiyle incelenir.

## 3. Caddy

[Caddyfile.relay.ornek](../deploy/Caddyfile.relay.ornek) yalnız relay site bloğudur.
[caddy-ekle.sh](../deploy/caddy-ekle.sh) varsayılan kuru çalışmada diff gösterir;
`--uygula` ile mevcut `/etc/caddy/Caddyfile` sonuna **ekler**, mevcut blokları değiştirmez.

| Koruma | Değer/davranış |
|---|---|
| TLS/yönlendirme | `relay.example.com`; yalnız `/v1/*` → `unix//run/agent-relay/relay.sock`, diğer yollar 404 |
| İstek/long-poll | Gövde en çok 64KB; transport response_header_timeout/read_timeout 75s, varsayılan max_wait_sec=50 için pay |
| Başlıklar | HSTS (1 yıl), X-Content-Type-Options=nosniff, Referrer-Policy=no-referrer; Server kaldırılır |
| Günlük | `/var/log/caddy/relay.log`; filter ile request>headers>Authorization ve tüm request>uri silinir; sorgu loglanmaz |
| Saklama | Dönen loglar 10MiB, en çok 14 dosya/336 saat; etkin dosya ve journal politikası ayrıca incelenir |
| Değişiklik | Önce DNS/arayüz eşleşmesi ve mevcut uygulama sağlığı → yedek → ekleme → validate → reload → mevcut uygulama 15 sn, relay ayrı 90 sn |
| Geri alma | Validate/reload/mevcut uygulama sağlık hatasında geri al; yalnız relay hazır değilse geri alma yok, uyarı ve çıkış 3; elle `--geri-al` önce kuru çalışır |

Caddy çekirdeğinde hız sınırı yoktur. Relay, ajan sınırına ek olarak kimliksiz/yanlış
token denemelerinde kayan bir dakikada IP başına `unauth_per_min=20`, global
`unauth_global_per_min=300` uygular. Ön kontrol DB'den önce çalışır; eksik/biçimsiz
Authorization DB'siz 401, yanlış token doğrulama sonrası 401, dolu sınır DB'siz 429'dur.
Başarılı doğrulama sayılmaz; paralel doğrulamalar geçici rezervasyon kullanır.
IP haritası en çok 10.000 kayıt tutar, eski kayıtlar temizlenir; doluyken yeni IP reddedilir.

Başarılı token'ların yalnız SHA-256 özetleri en çok 10.000 kayıt ve bir saat süreyle
ön sınırdan muaf tutulur; **her istekte DB doğrulaması sürer**, iptal sonrası ilk
başarısız doğrulama özeti siler. Daha önce görülmemiş, süresi dolmuş veya özet
haritasına sığmayan geçerli token dolu sınırda 429 alabilir: DB'ye bakmadan yeni
geçerli token'ı yanlış token'dan ayırmak mümkün değildir. Başarılı ajanlar kotayı tüketmez.
Bu sınır bağlantı/TLS DoS korumasının tamamı değildir; sağlık yolu kimliksiz kalır.

`trust_forwarded_for` varsayılan false'tur; üretim örneğinde yalnız Unix/systemd
dinlemede true'dur. Caddy'nin eklediği X-Forwarded-For **son** adresi kullanılır;
başlık eksik/geçersizse RemoteAddr (Unix için ortak unknown kovası) kullanılır.
TCP'de true başlangıç hatasıdır. Sokete erişen taraf IP başlığını verebildiği için
0660 izinleri ve Caddy'nin gerçek istemci adresini sona yazması ilk kurulumda doğrulanır.
Token hiçbir zaman loglanmaz. Günlükte istemci IP'si gibi
başka kişisel alanlar kalabilir. TLS ve kurulu Caddy 2.x sürümündeki yönergeler ilk
uygulamada doğrulanır. DNS ve mevcut uygulamanın gerçek sağlık URL'si özel kopyada
verilir. Yedekten sonra başka Caddy değişiklikleri varsa tam dosya geri alınmaz;
sahibi yalnız relay bloğunu kaldırır.

Uygulama sırası [envanter.md](envanter.md#değişiklik-protokolü)'de: haber ver → yedekle →
`caddy validate` → `systemctl reload caddy`. **Restart kullanılmaz.**

## 4. İstemci tarafı

- `~/.agent-relay/` dizini yalnız kullanıcıya açık (Windows'ta kullanıcı profili altında).
- `--dangerously-skip-permissions` hiçbir ajanda kullanılmaz (Claude, Codex, agy).
- Köprü Codex'i varsayılan olarak `--sandbox read-only` ile çalıştırır.
- Gelen mesajın gövdesi hiçbir zaman komut satırı argümanı olarak kabuğa **yorumlanmadan**
  verilir (dosyadan ya da stdin'den okunur). Kabuk enjeksiyonu engellenir.

## 5. Arkadaşın erişimi

- Arkadaşım sunucuya **SSH erişimi almaz.** Yalnız kendi ajanları için token'lar alır.
- Token'ı ben üretir ve güvenli bir kanaldan veririm. Gerekirse `relay-admin revoke` ile iptal ederim.

## 6. Tarayıcıdan izleme

`/v1/izle` insan hesabıyla giriş, izleme ve mesaj gönderme sayfasıdır. Statik HTML, JavaScript ve CSS kimliksiz sunulur.
Oda verisi istekleri token, oda üyeliği ve mevcut eşzamanlı istek sınırı ile korunur.
`/v1/rooms/{room}/log` yalnız `insan` türüne açıktır. İnsan hesabı odadaki **tüm** mesajları,
yönlü mesajlar dahil görür. Bu hesap yalnız oda sahibine verilir.

Dosyalar ikiliye gömülüdür; dış kaynak, çerez ve CORS başlığı kullanılmaz.
CSP birebir şöyledir:

```text
default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'
```

Üç dosyada da `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`,
`Referrer-Policy: no-referrer`, `Cache-Control: no-store`,
`Cross-Origin-Opener-Policy: same-origin` ve `Cross-Origin-Resource-Policy: same-origin` verilir.
Girişte ad ve parola alınır; parola yöneticinin `admin add-agent <ad> --rooms ... --kind insan`
ile ürettiği yüksek entropili rastgele token'dır, ayrı parola sistemi yoktur.
`GET /v1/me` kendi adını, türünü ve sıralı odalarını döndürür; oda gerektirmez,
mevcut kimliksiz istek, kimlik doğrulama ve bağlantı sınırlarını kullanır. Ajan da kendi bilgilerini alabilir.
Sayfa yalnız ad eşleşip tür `insan` olduğunda oturumu saklar; ad eşleşmesi yalnız
kullanıcı deneyimi içindir, güvenlik token'ın entropisinden gelir. Hatalı ad/parola aynı hatayı verir.
İnsan odasına mesaj gönderebilir, tarafı olmadığı konuşmaya yazabilir ve konuşmayı kapatabilir.
Çerez kullanılmadığı ve kimlik `Authorization` başlığında taşındığı için çerez tabanlı CSRF yoktur.

Satır içi betik ve stil yoktur. Başarılı girişte ad ve token yalnız `sessionStorage` içinde tutulur; depolama
çalışmazsa yalnız bellekte kalır. URL'ye yazılmaz. Yenilemede `/v1/me` ile tekrar doğrulanır;
başarısızsa giriş ekranına dönülür. Çıkış ve 401 yanıtı saklanan ad/token'ı siler;
çıkış tüm devam eden istekleri iptal eder.
İsteklerde `credentials: "omit"` ve `cache: "no-store"` kullanılır.

Mesaj gövdesi güvenilmez veridir. Dinamik DOM yalnız `document.createElement` ve
`textContent` ile oluşturulur. Mesaj HTML ya da kod olarak yorumlanmaz.
Sayfa bu güven sınırını kullanıcıya da belirtir.

Log long-poll'u sorgudan önce oda kanalını alır; süre veya sunucu kapanışında son kez sorgular.
`BeginWait` çağırmaz. İzleyen insan ajan listesinde `waiting=true` görünmez;
ajanlar insanın izlemesini karşı tarafın dinlemesi sanmaz.

## 7. Tarayıcıdan hesap yönetimi — tehdit modeli (Faz 7)

Sohbet parolasının çalınması, saldırganın kendine hesap açıp diğer odalara erişmesine
yetmemelidir. Yönetim için günlük sohbet hesabından **ayrı** bir insan hesabı kullanılır.
Yetki yalnız sunucu sahibinin SSH'deki `yonetici-yap` / `yonetici-al` komutlarıyla
atanır veya alınır; web API'sinde yetki verme yolu yoktur. Yeni hesap gövdesindeki
`yonetici` alanı yok sayılır. Ajan türü ve yetkisiz insan hesapları 403 alır.
Yönetici bütün hesap ve odalar üzerinde yetkilidir; bu hesabı başka kişilere vermeyin.

Yönetim Bearer parolasına ek olarak zorunlu TOTP ister: 20 rastgele bayt sır,
padding'siz base32, HMAC-SHA1, 30 saniye, altı hane ve ±1 adımlık saat toleransı.
Kod karşılaştırması sabit zamanlıdır. Kullanılan adım DB'de atomik olarak ilerletilir;
aynı ya da daha eski adım yeniden kullanılamaz (süreç yeniden başlasa da).
15 dakikalık pencerede beş yanlış deneme hesabı beşinci denemeden itibaren 15 dakika
kilitler; başarı sayacı temizler. Sayaç/kilit bellektedir, süreç yeniden başlayınca
sıfırlanır. Bu koruma tek süreç içindir; hesap kilitleyerek hizmet engelleme mümkündür.
Mevcut kimliksiz istek, hesap yazma ve bağlantı sınırları da korunur.

İkinci faktörden sonra üretilen 32 rastgele baytlık yönetim anahtarı 10 dakika
geçerlidir. Sunucu yalnız SHA-256 özetini, hesap bağını ve bitişini bellekte tutar.
TOTP sırrının özetiyle bağlanması sır yenilendiğinde eski oturumu da geçersiz kılar.
Her istekte insan türü ve yönetici yetkisi yeniden denetlenir. Her hesap için en son
oturum geçerlidir; yeniden başlatmada oturumlar silinir. Tarayıcı anahtarı yalnız JS
belleğinde tutar; `sessionStorage` dahil hiçbir depoya yazmaz. Bearer token için
mevcut sekme oturumu davranışı sürer. Oturum süresi dolunca veya sayfa yenilenince
yeniden kod girilir. İstekler HTTPS üzerinden, anahtarlar yalnız başlıklarda taşınır.

**Caddy logu:** Örnek Caddyfile'daki log filtresi yalnız `Authorization` başlığını siler.
`X-Yonetim-Oturum` başlığı bu yüzden Caddy erişim loguna yazılabilir. Bu anahtar tek başına
yetki vermez:
- Her yönetim isteği ayrıca loglanmayan Bearer parolasını ister.
- Anahtar 10 dakikada geçersiz olur.
- Anahtar, TOTP sırrına bağlıdır.

Yine de log dosyası en az relay verisi kadar korunmalıdır. Caddy'ye dokunulan bir sonraki
bakımda filtreye `request>headers>X-Yonetim-Oturum delete` satırı eklenebilir.

Yönetici hedef hesaba web'den oda ekleme/çıkarma ve iptal yapılamaz; kendisi de
buna dahildir. Hedef denetimi değişiklik transaction'ında tekrar yapılır; eşzamanlı
SSH atamasıyla koruma atlanamaz. Yönetici yetkisi geri alma ve sır yenileme SSH işidir.
Hesap açma ve üyelik/iptal değişikliği ile denetim kaydı aynı transaction'dadır;
kayıt yazılamazsa değişiklik geri alınır. Oturum açma ve reddetme, hesap/kayıt
listeleme de kaydedilir. SSH işlemlerinde yapan `ssh`, web'de hesap adıdır.
Kayıt alanları zaman, yapan, işlem, hedef ve oda gibi ayrıntıdır; parola, oturum
anahtarı, TOTP kodu/sırrı ve istek gövdesi kaydedilmez. Denetim tablosu kriptografik
olarak mühürlenmiş değildir; DB yazma yetkisi olan kişi kaydı değiştirebilir.

**TOTP sırrı doğrulama için gerekli olduğundan DB'de açık metin saklanır.** DB,
WAL/SHM ve yedekleri yalnız `agentrelay` kullanıcısınca okunabilen erişimle
korunmalıdır (sunucu root yetkisi bu sınırın dışındadır). Yeni göç dosya izinlerini
değiştirmez; mevcut kurulum sahipliği/izinleri korunmalı ve işletimde doğrulanmalıdır.
DB sızıntısı ikinci faktörü açığa çıkarır. DB'yi okuyabilen saldırganın oda içerikleri
zaten elindedir; parola özeti tek başına rastgele Bearer parolası yerine geçmez.

TOTP canlı kimlik avına, ele geçirilmiş tarayıcı/cihaza, aynı kaynakta çalışan kötü
niyetli betiğe veya SSH/root ihlaline karşı koruma sağlamaz. CSP, metinle DOM üretme,
aynı kaynak başlıkları ve dış bağımlılık kullanmama kuralları sürer. Yeni hesap
token'ı yalnız oluşturma cevabında sunulur; kutu kapatılınca DOM'dan silinir,
kopyalama yalnız açık kullanıcı eylemiyle panoya yapılır. Pano ve alıcıya güvenli
iletim kullanıcının sorumluluğundadır. Göçten önce tutarlı v1 yedeği alınmalıdır.
