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
  `useradd`, `systemd-analyze`, `python3`, `getent`, `ip` hazır olmalı; `caddy` grubu bulunmalı.
  Caddy aracı için curl gerekir; yalnız relay kurucusu curl yoksa python3 kullanır.
  Python kurucunun JSON/ELF denetimi ve curl yoksa Unix soketi sağlık kontrolü içindir;
  servis saf Go ikilisidir, venv yoktur. Otomatik paket kurulmaz.
- Caddy hizmeti sağlıklı ve `/etc/caddy/Caddyfile` normal dosya olmalı. Mevcut uygulamanın
  kimlik doğrulama gerektirmeyen, yönlendirmesiz **HTTPS 200** sağlık URL'sini sahibi verir.
  URL sorgu veya kimlik bilgisi içeremez; 443 portu kullanılır. Yanlış URL otomatik geri aldırır.
- Dışa açık yalnız 22/80/443 kalır; UFW/sshd değiştirilmez, relay TCP portu açılmaz. SSH erişimi
  yalnız mevcut yetkili operatörde kalır. DNS/TLS doğrulaması için mevcut 80/443 erişimi gerekir.
- Önce diğer uygulamaların sahiplerine haber ver → yedeği ve dönüş planını doğrula →
  `validate` → `reload`. **Caddy için restart yoktur.** Diğer servisler, veritabanları ve
  zamanlanmış işler değiştirilmez. Kuru çalışma çıktısı onaydan önce incelenir.
  Uygulama **sunucu sahibinin belirlediği bakım penceresinde** yapılır.

DNS önce hazırlanır: `relay.example.com` A/AAAA kayıtları bu sunucunun arayüz
adreslerine yönelmeli; eski/yanlış AAAA bırakılmamalı. Caddy aracı kuru çalışmada da
`getent ahosts` ve `ip -o addr` ile en az bir ortak adres arar. Eşleşme yoksa uyarır;
`--uygula` hiçbir dosya/kilit/yedek yazmadan durur. Ortak adres bulunması bütün A/AAAA
kayıtlarının doğru veya dışarıdan erişilebilir olduğunun kanıtı değildir.

Üretimde PID 1, `/run/agent-relay/relay.sock` dinleyicisini açar ve fd 3'ü relay'e
aktarır. Caddy bu Unix soketine bağlanır (`agentrelay:caddy`, 0660; üst dizin 0755).
Relay `PrivateNetwork=yes`, `RestrictAddressFamilies=AF_UNIX`, `IPAddressDeny=any`
ile çalışır; loopback dahil IP erişimi yoktur. `PrivateUsers=yes` korunur.

Paket: [servis](../deploy/agent-relay.service), [soket](../deploy/agent-relay.socket), [JSON](../deploy/config.example.json),
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
sudo bash deploy/kurulum.sh --ikili agent-relay-linux-amd64 --ozet SHA256SUMS
```

Varsayılan yol hiçbir dosya, kullanıcı, kilit, yedek veya servis değiştirmez. Root,
Ubuntu/systemd, dosya adı ve SHA-256, caddy grubu, soket hedefi ve RAM kontrol edilir.
Soket hedefi normal dosya veya sembolik bağlantıysa durur. `--port` kaldırılmıştır;
kullanılırsa anlaşılır hata verir. Mevcut config varsa systemd/trust/veri yolu eşleşmesi
denetlenir; eski TCP config otomatik dönüştürülmez, dosya içeriği değiştirilmez.
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
sudo bash deploy/kurulum.sh --ikili agent-relay-linux-amd64 --ozet SHA256SUMS --uygula
curl --unix-socket /run/agent-relay/relay.sock http://localhost/v1/health
```

Kurucu ayrı `agentrelay` sistem hesabını oluşturur (kabuksuz, ev dizinsiz, ek grup yok).
İkili root:root 0755, config root:agentrelay 0640 olur. Veri dizinini systemd 0700 ile
oluşturur. Eski ikili `agent-relay.onceki` olarak korunur; mevcut config değiştirilmez.
Kurucu curl yoksa python3 ile aynı Unix soketinden HTTP sağlık kontrolü yapar.

İki birim kurulur, `systemd-analyze verify`, `daemon-reload`,
`systemctl enable --now agent-relay.socket`, ardından servis enable/restart çalışır.
10 saniyelik sağlık penceresinde 200 alınmazsa servis durdurulur, önceki ikili ve birim
geri konur; soket birimi ve önceki etkin/aktif durumu da geri alınır. İlk kurulumda yeni birimler
devre dışı bırakılıp kaldırılır. Geri alma komutlarının hatası ACİL uyarısıyla bildirilir;
inceleme dosyaları korunur. Oluşmuş config, veri ve hesap korunur. systemd güvenlik
puanı gösterilir; puan tek başına kabul kararı değildir. Güncelleme relay restart'ı
gerektirir; veri yedeği ve sahibin onayladığı kesinti penceresi gerekir. Caddy'ye dokunulmaz.

## 5. DNS ve Caddy

Kullanıcı DNS'te `relay.example.com` yerine seçilen özel alanı sunucuya yönlendirir.
A kaydını ve varsa AAAA kaydını doğrular; yanlış/eski AAAA bırakmaz. Caddy TLS'i alır;
DNS yayılımını Caddy reload'dan önce bekleyin. İlk sertifika edinimi 10–60 saniye sürebilir.

`deploy/Caddyfile.relay.ornek` dosyasının **özel kopyasında** alanı düzenleyin; upstream
`unix//run/agent-relay/relay.sock` kalır.
Yalnız relay site bloğu eklenir. Mevcut site blokları değiştirilmez. Caddy'nin kurulu
2.x sürümünde `request_body`, transport zaman aşımı ve log filtresi desteği gerçek
`caddy validate` ile kontrol edilir. Caddy güncellemesi bu paketin kapsamı değildir.
`/var/log/caddy` için mevcut Caddy hesabının yazma yetkisini sahibi doğrular; başka
uygulamaların log izinlerini değiştirmeyin.

```bash
sudo bash deploy/caddy-ekle.sh --blok Caddyfile.relay --alan relay.example.com --mevcut-saglik https://api.example.com/saglik
sudo bash deploy/caddy-ekle.sh --blok Caddyfile.relay --alan relay.example.com --mevcut-saglik https://api.example.com/saglik --uygula
```

İlk komut DNS ön kontrolü ve diff gösterir; HTTP isteği/validate/reload yapmaz. Diff mevcut Caddyfile
içeriğini içerebilir; çıktıyı public repoya veya paylaşılan günlüklere koymayın.
İkinci komut mevcut uygulamanın ön sağlık kontrolünü yapar, zaman damgalı
`/etc/caddy/Caddyfile.yedek-<zaman>` yedeği alır, sona ekler, validate ve **reload** yapar.
Aynı alan Caddyfile'da site adresi olarak (tek başına, virgüllü/çok satırlı listede, `https://` öneki veya `:port` ekiyle, büyük/küçük harf fark etmeksizin) geçiyorsa hem kuru çalışma hem `--uygula` durur; yorum satırları, blok gövdesindeki metinler ve başka alanın alt dizesi (`xrelay.example.com`) eşleşme sayılmaz.
`import` ile gelen dosyalardaki alan çakışmalarını sahibi ayrıca
inceler; ana dosyadaki basit tarama bunları çözümlemez.

Reload sonrasında mevcut uygulama 15 saniye içinde 200 olmalıdır; olmazsa yedek
geri konur, validate/reload yapılır ve hata döner. Ardından relay HTTPS sağlığı ayrı
90 saniyelik pencerede beklenir. Relay başarısızsa **geri alınmaz**; DNS, ACME/sertifika
günlüğü (`journalctl -u caddy`) ve soket izni tanıları yazdırılır, çıkış **3** olur.
Caddy aracı çıkış kodları: **0** tamam/kuru çalışma, **1** ön kontrol veya uygulama
hatası (değişiklik başladıysa otomatik geri alma), **3** kısmi başarı: mevcut uygulama
sağlıklı, relay henüz hazır değil; eklenen blok ve yedek korunur. Yeniden eklemek yerine
tanı koyun; aynı alanın tekrar eklenmesi reddedilir.
Otomatik geri alma başarısızlığı ayrıca ACİL uyarısıyla belirtilir;
sunucu sahibi hemen müdahale eder. Betikler aynı `flock` kilidini kullanır; başka
operatörlerin Caddy düzenlemeleriyle çakışmamak için değişiklik penceresi yine gereklidir.

## 6. Doğrulama ve kabul

- [ ] Yerelde `curl --unix-socket /run/agent-relay/relay.sock http://localhost/v1/health`, dışarıdan
  `curl https://relay.example.com/v1/health`: HTTP 200 ve `{"ok":true}`.
- [ ] Mevcut uygulamanın sahibi `https://api.example.com/saglik` ve kendi kullanıcı
  akışları/veritabanı/zamanlanmış işleri üzerinden etkilenmediğini doğruladı.
- [ ] Relay alanında `/` 404, `/v1/*` API'ye gider; yanıt başlıklarında HSTS,
  `nosniff`, `no-referrer` var ve Server başlığı yok. 64 KB üstü gövde reddedilir.
- [ ] `sudo systemctl status agent-relay` ve `sudo systemd-cgtop` ile boşta ve gerçek
  long-poll yükünde bellek/CPU izlendi; 100M sınırı aşılmıyor, OOM/yeniden başlama yok.
  `sudo systemctl show agent-relay -p MemoryCurrent -p MemoryPeak -p MemoryMax -p NRestarts`
  ek kanıttır. 100M systemd birimi MiB tabanlıdır.
- [ ] Sunucu sahibine `systemd-analyze security agent-relay`, `ss -ltnp` ve
  `ss -lxp | grep agent-relay` çıktıları iletildi: relay TCP portu yok, Unix soketi var.
  UFW'de yalnız mevcut 22/80/443 açık.
- [ ] Soket root ağ ad alanında açılıyor; Caddy hesabı 0660 sokete bağlanabiliyor.
  `PrivateNetwork` + `PrivateUsers` altında fd 3 kabulü, config okuma ve SQLite/WAL
  yazımı çalışıyor. IP soketi oluşturma/yerel IP portuna çıkış engelli.
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

## 8. Tarayıcı sayfası (insan hesabı)

İnsanlar odayı `https://relay.example.com/v1/izle` adresinden izler ve odaya yazar. Sayfa ikiliye
gömülüdür. Caddy zaten `/v1/*` yolunu ilettiği için Caddy'de değişiklik gerekmez.
Her kişi için ayrı bir **insan** hesabı açın:

```bash
sudo -u agentrelay /opt/agent-relay/agent-relay admin --config /etc/agent-relay/config.json add-agent ornek-admin --rooms ornek-proje --kind insan
```

Basılan token, sayfadaki "Parola"dır; Ad alanına hesap adı yazılır.
- Token'ı yalnız bir kez gösterilir. Doğrudan parola yöneticisine kaydedin; sohbete ya da bir yapay zekâ
  oturumuna yapıştırmayın.
- İnsan hesabı odadaki **tüm** mesajları görür. Yalnız odanın sahiplerine açın.
- Ajanlar insan hesabından gelen mesajı da veri sayar
  ([ajan-talimati.md](ajan-talimati.md)). Tarayıcıdaki parola, ajana yetki veren bir kanal değildir.
- Sayfanın güvenlik ayrıntıları: [guvenlik.md](guvenlik.md).

### Yönetici ve tarayıcıdan yönetim

Bu güncelleme şemayı v2'ye taşır. **Güncellemeden ÖNCE veritabanı yedeği alın**:
"Günlük işletim ve yedek" bölümündeki SQLite `VACUUM INTO` ve `integrity_check`
adımlarını izleyin; çalışan WAL veritabanında yalnız ana dosyayı kopyalamayın.
v1 hesapları, üyelikler, konuşmalar ve mesajlar korunur. Göç tek transaction'dır;
eski ikili v2'yi açamaz. Geri dönüş için tutarlı v1 yedeği gerekir.

Günlük sohbet hesabından ayrı bir insan yönetici hesabını yalnız sunucuya SSH ile
bağlanarak oluşturun (örnek ad ve odayı kendi değerlerinizle değiştirin):

```bash
sudo -u agentrelay /opt/agent-relay/agent-relay admin --config /etc/agent-relay/config.json add-agent mehmet-yonetici --rooms ornek-proje --kind insan
sudo -u agentrelay /opt/agent-relay/agent-relay admin --config /etc/agent-relay/config.json yonetici-yap mehmet-yonetici
```

İlk komutun token'ını parola yöneticisinde saklayın. İkinci komut base32 TOTP sırrını
ve `otpauth://` URI'sini yalnız bu çalıştırmada stdout'a basar. Google Authenticator →
**Kurulum anahtarı gir → zamana dayalı** yolunda sırrı girin. **Sırrı sohbete ya da
AI oturumuna yapıştırmayın.** Terminal çıktısını paylaşılan kayda almayın.
`yonetici-yap` yeniden çalıştırılırsa yeni sır atanır; eski Authenticator kaydı geçersiz olur.

`/v1/izle` sayfasına bu hesabın adı/parolasıyla girin, üstteki **Yönetim** düğmesini
açıp altı haneli kodu yazın. Oturum 10 dakika geçerlidir; yenilemede veya süre
bitiminde kod yeniden istenir. Hesaplar, oda üyelikleri, iptal ve denetim kayıtları
paneldedir. Yeni hesabın parolası yalnız bir kez görünür; güvenli kanalla iletin,
ardından **Kapat** ile kutuyu silin. **Sohbete dön** sohbet görünümünü açar.
Yönetici hesabına web'den oda değişikliği veya iptal uygulanamaz.

SSH karşılıkları `odaya-ekle <ad> <oda>`, `odadan-cikar <ad> <oda>`,
`kayit --son 100` komutlarıdır. Yetkiyi kaldırmak için
`yonetici-al mehmet-yonetici` kullanın; TOTP sırrı da temizlenir. Telefon
kaybında SSH'den yetkiyi kaldırın veya `yonetici-yap` ile sırrı yenileyin.

## Güncelleme (yeni sürüm)

Aynı kurucu, mevcut kurulumu yükseltir. Config, veri, hesaplar ve yerel drop-in korunur.
Yalnız `agent-relay` yeniden başlatılır; Caddy'ye dokunulmaz. Kesinti birkaç saniyedir.

1. Sunucu sahibine değişikliği, commit aralığını ve kurulum penceresini bildirin; onay alın.
2. Bölüm 1'deki gibi derleyin. Yeni ikiliyi, `SHA256SUMS`'u ve `deploy/`'u **yeni ve boş** bir
   aktarım dizinine gönderin.
3. Önce canlı uygulamanın sağlığını ölçün, sonra kuru çalışmayı yapın. SHA uyuşmazsa durun:
   ```bash
   sudo bash deploy/kurulum.sh --ikili agent-relay-linux-amd64 --ozet SHA256SUMS
   ```
4. Uygulayın. Sağlık 10 saniyede 200 dönmezse önceki ikili kendiliğinden geri konur:
   ```bash
   sudo bash deploy/kurulum.sh --ikili agent-relay-linux-amd64 --ozet SHA256SUMS --uygula
   ```
5. Kontrol edin:
   - Relay `/v1/health` 200 dönmeli (soket ve HTTPS).
   - Canlı uygulamanın sağlığı değişmemiş olmalı.
   - `systemd-analyze security agent-relay --no-pager` puanı aynı kalmalı.
   - `systemctl show -p InaccessiblePaths agent-relay` sayısı aynı kalmalı.
   - `ss -ltnp` çıktısında relay TCP dinleyicisi olmamalı.

   Çıktıları sunucu sahibine iletin.
6. Aktarım dizinini silin. Sorun çıkarsa elle geri dönmek için: `agent-relay.onceki` dosyasını `agent-relay`
   yerine koyup `systemctl restart agent-relay` çalıştırın.

Yalnız istemci (`relay`) değiştiyse sunucuya dokunulmaz. Yeni `relay` ikilisi `SHA256SUMS` ile birlikte
kullanıcılara dağıtılır.

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

Soket ve servis durur/devre dışı kalır; iki birim, **tüm yerel drop-in dizini** ve ikili dizini
kaldırılır, daemon-reload yapılır. Önce drop-in'in özel yedeğini alın. Hesap, config
ve veri korunur. `--veri-sil --uygula` config ve veri dizinini de kalıcı siler; ancak
ayrı açık veri silme kararı ve doğrulanmış yedekten sonra kullanın.

> ⚠️ **`kaldir.sh` Caddy bloğunu KALDIRMAZ.** Caddyfile'daki relay bloğu yalnız
> `caddy-ekle.sh --geri-al <yedek>` ile (ya da sunucu sahibinin elle, yedek → validate →
> reload sırasıyla) kaldırılır. `kaldir.sh`'den sonra blok kalırsa Caddy, artık var olmayan
> sokete 502 döner; canlı uygulamayı etkilemez ama blok temizlenmelidir. Sıra: önce
> `caddy-ekle.sh --geri-al`, sonra `kaldir.sh`.

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
6. `RestrictNamespaces` ve `DevicePolicy`: son çare; gevşetmeden önce sahibine danışın.
   IP erişimini açmayın; `PrivateNetwork`, AF_UNIX ve `IPAddressDeny=any` korunur.

Sınırları (`MemoryMax`, `TasksMax`, `LimitNOFILE`) çok daraltmak da `NRestarts` artışıyla
görülür; bunlar güvenlik değil kaynak ayarıdır, önce `systemctl show` ile gerçek kullanımı ölçün.
Hiçbir adımda `NoNewPrivileges`, `CapabilityBoundingSet=`, `ProtectSystem=strict` veya `InaccessiblePaths`
kaldırılmaz. `/etc/agent-relay` dizin sahipliği (root:agentrelay 0750) ve config dosyası
(root:agentrelay 0640) beklendiği gibi kalmalıdır; `ConfigurationDirectory=` bu dizine dokunduysa
`stat -c '%U:%G:%a' /etc/agent-relay /etc/agent-relay/config.json` ile doğrulayın.

### Caddy tarafında ilk doğrulanacaklar

16.10.2026 olayında root ile validate, logu root:root 0600 oluşturduğu için reload izin hatası verdi; otomatik geri alma canlı uygulamayı korudu.
Artık ekleme ve geri alma uygulamalarında validate öncesi sabit `/var/log/caddy/relay.log` hedefi hazırlanır: dizin caddy sahibi olmalı, dosya yoksa özel dizinde `install` ile caddy:caddy 0600 oluşturulup mevcut hedefi ezmeyen atomik bağlantıyla yayımlanır; symlink, normal olmayan dosya, yanlış sahiplik/izin veya birden çok bağlantı hata verir, kuru çalışma hiçbir log oluşturmaz.
Bütün validate çağrıları `runuser -u caddy -- caddy validate` ile çalışır; Caddyfile/import okunamıyorsa açık hata verir ve root'a geri düşmez (güvenli log hazırlığı için uygulamada `python3` gerekir).

Caddyfile'da `{$DEĞİŞKEN}` kullanılıyorsa
(ör. systemd `EnvironmentFile`) uygulama aşamasında doğrulama başarısız olabilir.
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
sessizce kabul etmeyin; sahibiyle değerlendirin. Loopback izni kaldırıldı: relay IP
portlarına çıkamaz; Caddy'den gelen Unix fd'si özel ağ ad alanında kullanılmalıdır.
Root, kernel açığı veya eksik yerel drop-in karşısında systemd mutlak sınır değildir.

Ajan sınırına ek olarak kimliksiz denemeler IP başına dakikada 20, global 300 ile
sınırlıdır; Caddy çekirdeğinde genel hız limiti yoktur. 64 KB gövde sınırı tüm
bağlantı sayısını sınırlamaz. Bellek sınırında relay OOM ile
kapanabilir; CPU/Tasks/NOFILE sınırları yük altında gecikme veya hata doğurabilir.
50 saniyelik long-poll için iki upstream zaman aşımı 75 saniyedir; max_wait arttırılırsa
ikisini de pay bırakacak şekilde arttırın. TLS, Caddy reload/rollback, Linux kaynak
sınırları ve gerçek uygulama sağlığı bu paket hazırlanırken çalıştırılmamıştır.

Yerel kontroller Git Bash'te:

```bash
for f in deploy/*.sh deploy/test/*.sh; do bash -n "$f" || exit; done
bash deploy/test/kuru_calisma_test.sh
```

Test `--kok <dizin>` ile sahte Ubuntu/systemd/soket/RAM ve getent/ip stub'ları kullanır; root,
systemd süreci veya ağ gerektirmez. Argüman hatası, manifest, grup/soket/RAM, DNS, Caddy tek site
ve tekrar ekleme kontrollerini sınar; her çağrıda dosya ağacının içerik ve metadata
özetinin değişmediğini denetler. `--kok` ile `--uygula` kesinlikle yasaktır; üretimde
bu bayrak kullanılmaz. Yerel test Linux'taki ayrıcalıklı uygulama akışını doğrulamaz.
