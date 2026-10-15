# Linux VPS kurulum ve geri alma rehberi

15.10.2026 tarihli kullanıcı onayı **repoda paket hazırlama** kapsamındadır. Bu paketle
sunucuya bağlanılmadı. Sunucuda uygulamadan önce ana uygulamanın sahibi olan taraf
paketi, yerel drop-in listesini ve değişiklik penceresini inceleyip onaylamalıdır.
İletişim kullanıcı üzerinden yürür. Gerçek alan adı, SSH hedefi, sağlık yolu ve diğer
uygulamaların dizinleri yalnız sunucudaki/yerel özel kopyalarda düzenlenir; repoya konmaz.

## Ön koşullar ve değişiklik penceresi

- Ubuntu 24.04, PID 1 olarak systemd, amd64 CPU, cgroup v2 ve Caddy 2.x; Docker gerekmez.
- En az 300 MiB `MemAvailable`, ikili/veri/yedekler için yeterli boş disk. Windows'taki
  yaklaşık 21 MB tepe ölçümü Linux'ta aynı tüketimin garantisi değildir.
- Bash, coreutils (`sha256sum`, `install`, `diff`), `awk`, `grep`, `ss`, `curl`, `flock`,
  `useradd`, `systemd-analyze`, `python3` hazır olmalı. Python yalnız kurucunun JSON/ELF
  denetimi içindir; servis saf Go ikilisidir, venv yoktur. Otomatik paket kurulmaz.
- Caddy hizmeti sağlıklı ve `/etc/caddy/Caddyfile` normal dosya olmalı. Mevcut uygulamanın
  kimlik doğrulama gerektirmeyen, yönlendirmesiz **HTTPS 200** sağlık URL'sini sahibi verir.
  URL sorgu veya kimlik bilgisi içeremez; 443 portu kullanılır. Yanlış URL otomatik geri aldırır.
- Dışa açık yalnız 22/80/443 kalır; UFW/sshd değiştirilmez, 8790 açılmaz. SSH erişimi
  yalnız mevcut yetkili operatörde kalır. DNS/TLS doğrulaması için mevcut 80/443 erişimi gerekir.
- Önce diğer uygulamaların sahiplerine haber ver → yedeği ve dönüş planını doğrula →
  `validate` → `reload`. **Caddy için restart yoktur.** Diğer servisler, veritabanları ve
  zamanlanmış işler değiştirilmez. Kuru çalışma çıktısı onaydan önce incelenir.

Paket: [systemd birimi](../deploy/agent-relay.service), [JSON](../deploy/config.example.json),
[yerel drop-in örneği](../deploy/10-yerel.conf.ornek), [Caddy bloğu](../deploy/Caddyfile.relay.ornek),
[kurucu](../deploy/kurulum.sh), [Caddy aracı](../deploy/caddy-ekle.sh),
[kaldırıcı](../deploy/kaldir.sh). Betikleri `bash` ile çalıştırmak yürütme izni gerektirmez;
`sudo ./kurulum.sh` biçimi için yalnız bu üç giriş betiğine `chmod 0755` verilebilir.

## 1. Yerelde derle ve aktar

Proje kökünde Git Bash ile, Go 1.27.0 ve vendored bağımlılıklarla:

```bash
GOFLAGS=-mod=vendor GOPROXY=off GOCACHE="$(cygpath -w "$(pwd)/.gocache")" bash tools/derle/derle.sh
```

Linux'ta aynı komutta `GOCACHE="$(pwd)/.gocache"` kullanılır. Betik sekiz ikili ve
`dist/SHA256SUMS` üretir. Bu kurucu `agent-relay-linux-amd64` içindir; arm64 sunucuda
uygulamayın. Manifesti ve ikiliyi güvenilen aynı derlemeden alın; SHA-256 kimlik
doğrulayan imza değildir. SHA256SUMS içindeki **dosya adıyla** eşleşme zorunludur;
aktarımda ikilinin adını değiştirmeyin.

Mevcut SSH erişimiyle `scp` üzerinden `deploy/` dizininin tamamını,
`dist/agent-relay-linux-amd64` ve `dist/SHA256SUMS` dosyalarını sahibin belirlediği
özel bir aktarım dizinine gönderin. SSH hedefi/yolu bu rehbere eklenmez. İkili ve
manifest aktarım dizininin kökünde, betikler `deploy/` altında olacak şekilde devam edin.
Aktarım SSH ile yapılır; paket hazırlayan ajan bunu yürütmez.

## 2. Kurulumun kuru çalışması

Sunucuda, aktarım dizininden:

```bash
sudo bash deploy/kurulum.sh --ikili agent-relay-linux-amd64 --ozet SHA256SUMS --port 8790
```

Varsayılan yol hiçbir dosya, kullanıcı, kilit, yedek veya servis değiştirmez. Root,
Ubuntu/systemd, dosya adı ve SHA-256, `ss -ltn` ile boş port, RAM kontrol edilir.
Wildcard/IPv6 dahil aynı portu kullanan bir dinleyici varsa durur. Mevcut config varsa
localhost/port/veri yolu eşleşmesi denetlenir; dosya içeriği değiştirilmez.
`/etc/caddy/Caddyfile` bu kurucuda okunmaz veya değiştirilmez.

## 3. Yerel izolasyon drop-in'i

Sunucu sahibi [örneği](../deploy/10-yerel.conf.ornek) inceler; gerçek uygulama, ayar,
veritabanı, yedek ve zamanlanmış iş dizinlerini belirler. Yalnız sunucuda root olarak
`/etc/systemd/system/agent-relay.service.d/10-yerel.conf` dosyasına koyar:

```bash
sudo install -d -o root -g root -m 0755 /etc/systemd/system/agent-relay.service.d
# Örneği özel bir kopyada düzenleyip yer tutucuları gerçek, var olan yollarla değiştirin.
# Sonra bu özel dosyayı root:root 0644 ile 10-yerel.conf konumuna kurun.
```

Örneği olduğu gibi kurmayın: var olmayan `InaccessiblePaths` hedefleri servisi
başlatmayabilir. Başına `-` koyup hataları sessizce geçmeyin. Relay'in kendi yollarını
engellemeyin; `/proc`, temel kütüphaneler ve çalışma zamanı dosyaları için geniş
engeller eklemeyin. Drop-in public repoya/çıktılara taşınmaz. Servis zaten okuyabildiği
başka uygulama dosyalarından bu liste ve dosya izinleriyle ayrılır; salt okunur olmak
okunamayacağı anlamına gelmez.

Kurucu bu dosyada dolu bir `InaccessiblePaths=` satırı arar. Bu kontrol listenin
eksiksizliğini ispatlamaz; sahibi incelemelidir. Dosya/satır yoksa `--uygula` değişiklik
yapmadan durur. `--drop-in-yok-onay` yalnız sahibin açıkça kabul ettiği istisnadır;
kişisel veri tutan bu sunucuda normal kurulum yolu değildir.

## 4. Relay'i uygula

Kuru çalışmayı yeniden inceleyin, ardından:

```bash
sudo bash deploy/kurulum.sh --ikili agent-relay-linux-amd64 --ozet SHA256SUMS --port 8790 --uygula
curl --noproxy '*' --fail http://127.0.0.1:8790/v1/health
```

Kurucu ayrı `agentrelay` sistem hesabını oluşturur (kabuksuz, ev dizinsiz, ek grup yok).
İkili root:root 0755, config root:agentrelay 0640 olur. Veri dizinini systemd 0700 ile
oluşturur. Eski ikili `agent-relay.onceki` olarak korunur; mevcut config değiştirilmez.
Portu değiştirirseniz **özel Caddy kopyasındaki upstream'i de** aynı porta ayarlayın.

Birim kurulur, `systemd-analyze verify`, `daemon-reload`, `enable` ve `restart agent-relay` çalışır (yalnız relay; Caddy ve diğer servisler değil).
10 saniyelik sağlık penceresinde 200 alınmazsa servis durdurulur, önceki ikili ve birim
geri konur; relay kapalı bırakılır ve komut hata döndürür. İlk kurulumda yeni birim
devre dışı bırakılıp kaldırılır. Geri alma komutlarının hatası ACİL uyarısıyla bildirilir;
inceleme dosyaları korunur. Oluşmuş config, veri ve hesap korunur. systemd güvenlik
puanı gösterilir; puan tek başına kabul kararı değildir. Port boşluğu şartı nedeniyle
bu betik çalışan sürümü yerinde güncellemez; güncellemede sahibin onayladığı relay
kesintisi, veri yedeği ve `stop` adımı gerekir. Caddy'ye dokunulmaz.

## 5. DNS ve Caddy

Kullanıcı DNS'te `relay.example.com` yerine seçilen özel alanı sunucuya yönlendirir.
A kaydını ve varsa AAAA kaydını doğrular; yanlış/eski AAAA bırakmaz. Caddy TLS'i alır;
DNS yayılımını bekleyin. Sertifika edinimi 15 saniyeyi aşarsa Caddy adımı güvenli
olarak geri döner; günlükleri inceleyip daha sonra yeniden deneyin.

`deploy/Caddyfile.relay.ornek` dosyasının **özel kopyasında** alanı ve portu düzenleyin.
Yalnız relay site bloğu eklenir. Mevcut site blokları değiştirilmez. Caddy'nin kurulu
2.x sürümünde `request_body`, transport zaman aşımı ve log filtresi desteği gerçek
`caddy validate` ile kontrol edilir. Caddy güncellemesi bu paketin kapsamı değildir.
`/var/log/caddy` için mevcut Caddy hesabının yazma yetkisini sahibi doğrular; başka
uygulamaların log izinlerini değiştirmeyin.

```bash
sudo bash deploy/caddy-ekle.sh --blok Caddyfile.relay --alan relay.example.com --mevcut-saglik https://api.example.com/saglik
sudo bash deploy/caddy-ekle.sh --blok Caddyfile.relay --alan relay.example.com --mevcut-saglik https://api.example.com/saglik --uygula
```

İlk komut yalnız diff gösterir; ağ isteği/validate/reload yapmaz. Diff mevcut Caddyfile
içeriğini içerebilir; çıktıyı public repoya veya paylaşılan günlüklere koymayın.
İkinci komut mevcut uygulamanın ön sağlık kontrolünü yapar, zaman damgalı
`/etc/caddy/Caddyfile.yedek-<zaman>` yedeği alır, sona ekler, validate ve **reload** yapar.
Aynı alan Caddyfile'da site adresi olarak (tek başına, virgüllü/çok satırlı listede, `https://` öneki veya `:port` ekiyle, büyük/küçük harf fark etmeksizin) geçiyorsa hem kuru çalışma hem `--uygula` durur; yorum satırları, blok gövdesindeki metinler ve başka alanın alt dizesi (`xrelay.example.com`) eşleşme sayılmaz.
`import` ile gelen dosyalardaki alan çakışmalarını sahibi ayrıca
inceler; ana dosyadaki basit tarama bunları çözümlemez.

Reload sonrasında 15 saniyelik ortak pencerede hem mevcut uygulama hem relay HTTPS
sağlığı 200 olmalıdır. Başarısızlıkta yedek geri konur, validate ve reload yapılır,
komut hata verir. Otomatik geri alma başarısızlığı ayrıca ACİL uyarısıyla belirtilir;
sunucu sahibi hemen müdahale eder. Betikler aynı `flock` kilidini kullanır; başka
operatörlerin Caddy düzenlemeleriyle çakışmamak için değişiklik penceresi yine gereklidir.

## 6. Doğrulama ve kabul

- [ ] Yerelde `curl http://127.0.0.1:8790/v1/health`, dışarıdan
  `curl https://relay.example.com/v1/health`: HTTP 200 ve `{"ok":true}`.
- [ ] Mevcut uygulamanın sahibi `https://api.example.com/saglik` ve kendi kullanıcı
  akışları/veritabanı/zamanlanmış işleri üzerinden etkilenmediğini doğruladı.
- [ ] Relay alanında `/` 404, `/v1/*` API'ye gider; yanıt başlıklarında HSTS,
  `nosniff`, `no-referrer` var ve Server başlığı yok. 64 KB üstü gövde reddedilir.
- [ ] `sudo systemctl status agent-relay` ve `sudo systemd-cgtop` ile boşta ve gerçek
  long-poll yükünde bellek/CPU izlendi; 100M sınırı aşılmıyor, OOM/yeniden başlama yok.
  `sudo systemctl show agent-relay -p MemoryCurrent -p MemoryPeak -p MemoryMax -p NRestarts`
  ek kanıttır. 100M systemd birimi MiB tabanlıdır.
- [ ] `ss -ltn` yalnız `127.0.0.1:8790` gösteriyor. Dışarıdaki yetkili başka bir
  makineden 8790 bağlantısı başarısız; UFW'de yalnız mevcut 22/80/443 açık.
- [ ] `sudo systemd-analyze security agent-relay` sahibi tarafından incelendi.
  `systemctl status`/journal çıktısında bilinmeyen yönerge, seccomp, namespace veya
  BPF/cgroup filtreleme uyarısı yok; varsa güvenlik kuralını kaldırarak geçmeyin.
- [ ] Yalnız sentetik sorgu/kimlik bilgisiyle log filtresi kontrol edildi:
  `/var/log/caddy/relay.log` içinde Authorization ve URI/sorgu alanı yok. Gerçek token
  ile test yapmayın; loglarda diğer kişisel alanlar (ör. istemci IP'si) kalabilir.

## 7. Ajan ve istemci

Güvenli, kaydedilmeyen bir yönetim terminalinde, her ajan için ayrı:

```bash
sudo -u agentrelay /opt/agent-relay/agent-relay admin --config /etc/agent-relay/config.json add-agent ornek-ajan --rooms ornek-proje
sudo -u agentrelay /opt/agent-relay/agent-relay admin --config /etc/agent-relay/config.json list
```

Token bir kez stdout'a çıkar. Oturum kaydı, ekran paylaşımı, kabuk izleme (`set -x`),
pipe/tee veya public dosya kullanmayın. Token'ı yalnız ilgili kişiye güvenli kanaldan
verin. İstemcide (Windows'ta PowerShell/cmd; Git Bash token girişini gizlemeyebilir):

```text
relay setup --url https://relay.example.com --room ornek-proje --agent ornek-ajan
relay agents
```

Token'ı gizli giriş istemine yapıştırın; argümana veya ortam değişkenine koymayın.
Kurulum ve köprü kullanımı: [cmd/README.md](../cmd/README.md).

## Geri alma ve kaldırma

Önce haber verin. Caddy yedeğini, alındığından beri yapılan **bütün** değişikliklerle
karşılaştırın. Eski tam yedeği geri koymak sonradan eklenen canlı site değişikliklerini
silebilir; böyle bir durumda sahibi yalnız relay bloğunu çıkarmalı, yeni yedek alıp
validate/reload yapmalıdır.

```bash
sudo bash deploy/caddy-ekle.sh --geri-al /etc/caddy/Caddyfile.yedek-<zaman>
sudo bash deploy/caddy-ekle.sh --geri-al /etc/caddy/Caddyfile.yedek-<zaman> --uygula
```

`<zaman>` gerçek yedek sonekiyle değiştirilir; örnek satırları olduğu gibi çalıştırmayın.
Geri alma da varsayılan kuru çalışır. Uygulamada önce kaynak validate edilir, mevcut
dosyanın ayrıca yedeği alınır, geri konur, validate/reload yapılır. Mevcut uygulama
sağlığını sahibi tekrar doğrular. Relay DNS kaydı gerekiyorsa kullanıcı tarafından kaldırılır.

İkiliyi geri almak için relay'i durdurun, sahibin incelemesinden sonra
`agent-relay.onceki` dosyasını root:root 0755 ile `agent-relay` üzerine kurun ve
relay'i başlatıp yerel sağlığı doğrulayın. Veritabanı otomatik geri alınmaz; eski
sürümle şema uyumunu kontrol edin. İlk başarısız kurulumda önceki ikili bulunmaz.

```bash
sudo bash deploy/kaldir.sh
sudo bash deploy/kaldir.sh --uygula
```

Servis durur/devre dışı kalır; birim, **tüm yerel drop-in dizini** ve ikili dizini
kaldırılır, daemon-reload yapılır. Önce drop-in'in özel yedeğini alın. Hesap, config
ve veri korunur. `--veri-sil --uygula` config ve veri dizinini de kalıcı siler; ancak
ayrı açık veri silme kararı ve doğrulanmış yedekten sonra kullanın. Caddy bloğu bu
betikle kaldırılmaz; yukarıdaki geri alma adımı gerekir.

## Günlük işletim ve yedek

```bash
sudo journalctl -u agent-relay --since today
sudo -u agentrelay /opt/agent-relay/agent-relay admin --config /etc/agent-relay/config.json revoke ornek-ajan
```

Günlükleri public ortama taşımadan inceleyin. Mesaj saklama varsayılanı **14 gün**;
sunucu başlangıçta ve saatte bir süresi dolan mesajları temizler. Ajanlar/konuşma
kayıtları, yedekler ve journal bu işlemle silinmez. Caddy dönen logları 14 gün/14
dosya ve 10 MiB dilimleriyle sınırlanır; etkin dosya ve journal için mevcut sunucu
saklama politikasını sahibi ayrıca değerlendirmelidir. Yedekler de 14 gün saklanmalı;
silme yalnız relay'e ayrılmış yedek alanında, doğrulanmış hedeflerle yapılmalıdır.

SQLite WAL modundadır: çalışan serviste yalnız `cp relay.db` veya db/WAL/SHM dosyalarını
art arda kopyalamak tutarlı yedek değildir. `.backup` yerine, servis çalışırken SQLite'ın
**`VACUUM INTO`** komutuyla yeni ve tutarlı bir anlık kopya üretin. `sqlite3` CLI hazır
değilse kurulumu sahibin ayrı onaylı paket değişikliğidir. Yeni, kullanılmayan bir
dosya adı seçin; örnek tarih-saat yer tutucusunu değiştirin:

```bash
sudo -u agentrelay sqlite3 /var/lib/agent-relay/relay.db ".timeout 5000" "VACUUM INTO '/var/lib/agent-relay/yedek-TARIH-SAAT.db';"
sudo chmod 0600 /var/lib/agent-relay/yedek-TARIH-SAAT.db
sudo -u agentrelay sqlite3 /var/lib/agent-relay/yedek-TARIH-SAAT.db 'PRAGMA integrity_check;'
```

`ok` sonucunu doğrulayın; sonra **üretilmiş kapalı snapshot dosyasını** sunucu sahibinin
belirlediği şifreli, erişimi kısıtlı yedek alanına kopyalayın. Config ve özel drop-in'i
ayrı, kısıtlı erişimle yedekleyin. Geri yüklemeyi izole ortamda sınayın. Düşük trafikte
çalıştırın; kopya için ek disk, I/O ve WAL büyüme payı gerekir. `sudo -u agentrelay`
ile açılan admin/sqlite3 süreçleri systemd servisinin kaynak/namespace sınırlarını
miras almaz; eşzamanlı ağır işlemleri sınırlayın. Mevcut uygulamaların zamanlanmış
işlerine ekleme yapmayın; relay yedek zamanlamasını sahibi ayrı değerlendirsin.

## İlk kurulumda servis başlamazsa: önce neyi gevşet

Belirti için `sudo journalctl -u agent-relay -b --no-pager | tail -50` ve
`systemctl status agent-relay` bakın (örn. `status=31/SYS` seccomp, `226/NAMESPACE`
ad alanı/InaccessiblePaths, `Permission denied` dosya sahipliği). Her seferinde **tek**
değişiklik yapın, drop-in ile (`sudo systemctl edit agent-relay` → ayrı bir
`20-gevsetme.conf`; ana birimi düzenlemeyin), `daemon-reload` + `restart agent-relay` ile
deneyin ve nedenini sunucu sahibine bildirin. Sırayla, en az güvenlik kaybı olandan:

1. `10-yerel.conf` içindeki var olmayan `InaccessiblePaths` yolu (226/NAMESPACE): yolu düzelt veya sil; `-` önekiyle sessizleştirme.
2. `SystemCallFilter=~@privileged @resources` içinden yalnız `@resources` ifadesini çıkar
   (`SystemCallFilter=` ile listeyi sıfırlayıp `@system-service` ve `~@privileged` yeniden verilir).
   Gerekçe: Go 1.19+ başlangıçta `prlimit64` (@resources) çağırır; birimde `SystemCallErrorNumber=EPERM` olduğu
   için normalde ölmez, ama modernc/libc beklenmedik bir çağrı yaparsa buradan çözülür.
   `SystemCallErrorNumber=EPERM` satırını silmeyin.
3. `ProcSubset=pid` (ve gerekirse `ProtectProc`): `/proc` dosyası bulunamadı hataları için.
4. `PrivateUsers=yes` -> `no`: config/veri `Permission denied` veriyor veya kullanıcı ad alanı
   çekirdekte kapalıysa. `ReadOnly`/sahiplik (`root:agentrelay 0640`) önce doğrulanır.
5. `MemoryDenyWriteExecute=yes` -> `no`: yalnız SIGSEGV/SIGSYS ile erken ölümde ve diğerleri sonuç vermediyse.
6. `RestrictNamespaces`, `IPAddressDeny/Allow` ve `DevicePolicy`: son çare; gevşetmeden önce sahibine danışın.

Sınırları (`MemoryMax`, `TasksMax`, `LimitNOFILE`) çok daraltmak da `NRestarts` artışıyla
görülür; bunlar güvenlik değil kaynak ayarıdır, önce `systemctl show` ile gerçek kullanımı ölçün.
Hiçbir adımda `NoNewPrivileges`, `CapabilityBoundingSet=`, `ProtectSystem=strict` veya `InaccessiblePaths`
kaldırılmaz. `/etc/agent-relay` dizin sahipliği (root:agentrelay 0750) ve config dosyası
(root:agentrelay 0640) beklendiği gibi kalmalıdır; `ConfigurationDirectory=` bu dizine dokunduysa
`stat -c '%U:%G:%a' /etc/agent-relay /etc/agent-relay/config.json` ile doğrulayın.

### Caddy tarafında ilk doğrulanacaklar

`caddy validate` root olarak çalışır; Caddyfile'da `{$DEĞİŞKEN}` kullanılıyorsa
(ör. systemd `EnvironmentFile`) kuru/uygulama aşamasında doğrulama başarısız olabilir.
Bu durumda sahibi `--envfile` ile elle doğrular; betik hata verip dosyayı değiştirmeden durur.
Emin olunamayan yönergeler: `request_body { max_size }`, `reverse_proxy` altında `transport http`
içindeki `response_header_timeout`/`read_timeout`, `log` altında `format filter` + `fields`
(`request>headers>Authorization delete`, `request>uri delete`), `roll_keep_for`. Hepsi Caddy 2.x
belgelerine uygundur ama kurulu sürümde `caddy validate` ile kesinleşir.

## Bilinen sınırlar ve yerelde doğrulananlar

`MemoryDenyWriteExecute` saf Go/modernc SQLite'ın JIT kullanmamasıyla uyumludur.
Ancak Go 1.27 Linux çalışma zamanı ile bu birimin **tam birleşimi** Windows'ta
sınanamaz. Özellikle `PrivateUsers`/`RestrictNamespaces`, `ProtectProc`/`ProcSubset`,
`SystemCallFilter` (`@resources` dahil), IP BPF filtreleri ve `DevicePolicy` gerçek
Ubuntu çekirdeği/systemd üzerinde doğrulanmalıdır. Gereken özelliğin yokluğunu
sessizce kabul etmeyin; sahibiyle değerlendirin. `IPAddressAllow=localhost` aynı
sunucudaki diğer yerel portlara çıkışı da serbest bırakır; bu tam ağ izolasyonu değildir.
Root, kernel açığı veya eksik yerel drop-in karşısında systemd mutlak sınır değildir.

Hız sınırı ajan başınadır, Caddy çekirdeğinde genel hız limiti yoktur. 64 KB gövde
sınırı eşzamanlı anonim istek sayısını sınırlamaz. Bellek sınırında relay OOM ile
kapanabilir; CPU/Tasks/NOFILE sınırları yük altında gecikme veya hata doğurabilir.
50 saniyelik long-poll için iki upstream zaman aşımı 75 saniyedir; max_wait arttırılırsa
ikisini de pay bırakacak şekilde arttırın. TLS, Caddy reload/rollback, Linux kaynak
sınırları ve gerçek uygulama sağlığı bu paket hazırlanırken çalıştırılmamıştır.

Yerel kontroller Git Bash'te:

```bash
for f in deploy/*.sh deploy/test/*.sh; do bash -n "$f" || exit; done
bash deploy/test/kuru_calisma_test.sh
```

Test `--kok <dizin>` ile sahte Ubuntu/systemd/port/RAM girdilerini kullanır; root,
systemd süreci veya ağ gerektirmez. Argüman hatası, manifest, port/RAM, Caddy tek site
ve tekrar ekleme kontrollerini sınar; her çağrıda dosya ağacının içerik ve metadata
özetinin değişmediğini denetler. `--kok` ile `--uygula` kesinlikle yasaktır; üretimde
bu bayrak kullanılmaz. Yerel test Linux'taki ayrıcalıklı uygulama akışını doğrulamaz.
