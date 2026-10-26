# cmd: sunucu ve istemci

İki ikili var, ikisi de tek dosya ve ek kurulum istemez:

| İkili | Ne yapar |
|---|---|
| `agent-relay` | Sunucu (`serve`) ve yönetici komutları (`admin`). Sunucuda çalışır. |
| `relay` | İstemci (`setup`, `version`, `doctor`, `send`, `wait`, `inbox`, `agents`, `thread`, `close`, `bridge`). Her ajanın makinesinde çalışır, Python gerekmez. |

## Derleme

Go 1.27.3 veya üzeri gerekir. Proje kökünden:

```
# Windows istemcisi (arkadaşın makinesi için de aynısı)
go build -o bin/relay.exe ./cmd/relay

# Linux sunucusu (CGO gerekmez; SQLite saf Go)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/agent-relay ./cmd/agent-relay
```

PowerShell'de Linux derlemesi: `$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"; go build -o bin/agent-relay ./cmd/agent-relay`.

Yayın için Git Bash ya da Linux'ta proje kökünden `bash tools/derle/derle.sh` çalıştırın.
Betik `dist/` altında Windows, Linux ve macOS için amd64/arm64 `relay` ikililerini,
Linux amd64/arm64 `agent-relay` ikililerini ve `SHA256SUMS` dosyasını üretir (toplam 8 ikili).
Go'yu PATH'ten veya Windows'ta `C:\Program Files\Go\bin` konumundan bulur; CGO kapalıdır,
`-trimpath` ve `-s -w` kullanılır. Sürüm `git describe --tags --always --dirty`, commit kısa hash'tir.
Varsayılanlar `GOFLAGS=-mod=vendor`, `GOPROXY=off`, `GOCACHE=<proje>/.gocache`;
bu üç ayar dışarıdan verilmişse korunur. Yeni bağımlılık indirilmez.

`relay version` ayar dosyası gerektirmeden sürüm, commit ve Go sürümünü basar.
Yayın betiği sürüm ve commit'i `-ldflags "-X github.com/MehmetKoc16/Agent-message/internal/client.version=... -X github.com/MehmetKoc16/Agent-message/internal/client.commit=..."`
ile doldurur; verilmemişse ikisi de `dev` olur.

## Sunucuyu lokalde başlatma

Yalnız arayüz tasarımını incelemek için Go derlemesi gerekmez:
`node tools/ui-preview/server.mjs` çalıştırıp `http://127.0.0.1:8791/v1/izle`
adresini açın (örnek kullanıcı `demo`, parola `relay-demo`). Bu önizleme gerçek
ajanlara bağlanmaz; değişiklikleri sayfayı yenileyerek gösterir.
Ayrıntılar: [yerel tasarım önizlemesi](../tools/ui-preview/README.md).

1. `cmd/agent-relay/config.example.json` dosyasını kopyalayıp düzenleyin (`db` yolunu yazılabilir bir yere verin). `host` yalnız `127.0.0.1` ya da `::1` olabilir; bilinmeyen alan hatadır.
2. Ajan ekleyin. Token **yalnız bir kez** ekrana basılır:

```
agent-relay admin --config server.json add-agent ali-claude --rooms ornek-proje
agent-relay admin --config server.json add-agent mehmet --rooms ornek-proje --kind insan
agent-relay admin --config server.json list
agent-relay admin --config server.json revoke ali-claude
agent-relay admin --config server.json reopen ornek-proje T-3   # kilitli konuşmayı açar (+max_turns)
agent-relay admin --config server.json purge                    # süresi dolan mesajları siler
```

3. Başlatın: `agent-relay serve --config server.json`. Ctrl+C ya da SIGTERM ile düzgün kapanır. Üretimde Caddy (TLS) arkasında çalışır; bkz. `docs/guvenlik.md`.

`listen` boşsa geliştirme için mevcut host/port TCP davranışı sürer (yalnız
`127.0.0.1`/`::1`). `listen: "systemd"` üretimde tek Unix dinleyicisini fd 3'ten
alır; LISTEN_PID bu süreç, LISTEN_FDS tam 1 olmalıdır. Aktivasyon değişkenleri
okunup temizlenir. Bu mod yalnız systemd üzerinden başlatılır.
`listen: "unix:<mutlak yol>"` yerel deneme içindir. Dolu listen ile host veya port
verilemez; bilinmeyen JSON alanları reddedilir. Unix hedefi mevcut normal dosyaysa
başlamaz; eski soketi kaldırır, 0660 dinler, kapanışta soketi siler.

Yerel Linux denemesi için host/port alanlarını çıkarıp şu ayarı kullanın (db yazılabilir olmalı):

```json
{"listen":"unix:/run/agent-relay/relay.sock","db":"relay.db"}
```

```bash
agent-relay serve --config server.json
# Başka terminalde; soket dizini önceden oluşturulmuş ve kullanıcıya yazılabilir olmalı:
curl --unix-socket /run/agent-relay/relay.sock http://localhost/v1/health
```

Windows'ta `unix:` sonrasında yazılabilir mutlak Windows yolu kullanın; JSON'da
ters eğik çizgileri kaçırın veya `/` kullanın. Windows AF_UNIX desteği gerekir.
`trust_forwarded_for` varsayılan false; yalnız Unix/systemd ve güvenilen proxy
erişimiyle true yapılır. Son X-Forwarded-For adresini kullanır; TCP ile true hatadır.
Kimliksiz limitler varsayılan IP başına 20/dakika, global 300/dakikadır
(`unauth_per_min`, `unauth_global_per_min`). Başarılar sayılmaz; tanınan token özeti
ön sınırı atlar, DB iptal kontrolü sürer. Dolu sınırda yeni token da 429 alabilir.

Dağıtım Caddy aracının çıkışları: 0 tamam/kuru çalışma, 1 hata (değişiklik sonrası
mevcut uygulama 15 sn içinde 200 değilse geri alma), 3 kısmi başarı (relay 90 sn
içinde hazır değil; geri alınmaz). DNS önce sunucuyu göstermelidir; ayrıntı: `docs/kurulum.md`.

## İstemci ayarı

`relay setup --url https://relay.example.com --room ornek-proje --agent ali-claude`
ile ayarı oluşturun. Token yalnız stdin'den tek satır okunur; terminalde `Token: ` isteminde
Windows ve Linux'ta yazılan değer ekrana yansıtılmaz. Diğer platformlarda yansıtma kapatılamıyorsa
uyarı verilir. Boru/dosyadan okumada istem gösterilmez; token'ı komut metnine yazmayın.
Baştaki/sondaki boşluklar kırpılır. Token 20–200 base64url karakteri (`A–Z`, `a–z`, `0–9`, `_`, `-`),
oda ve ajan 1–64 küçük harf/rakam/`._-` olmalı; adın ilk karakteri harf ya da rakamdır.

Varsayılan dosya `~/.agent-relay/config.json` (Windows'ta `%USERPROFILE%\.agent-relay\config.json`),
biçim `cmd/relay/config.example.json` ile aynıdır. Başka yer için setup'a `--config <yol>` verin;
diğer komutlarda `relay --config <yol> ...` kullanın. Mevcut dosya yalnız `--force` ile değiştirilir.
Yeni klasör 0700, dosya 0600 izinle oluşturulur (Windows izinleri kısmen uygular); geçici dosya
ve rename ile yazılır. Token argümandan ya da ortam değişkeninden **alınmaz**, çıktıda basılmaz.
`url` `https` olmalı; düz `http` yalnız `127.0.0.1`/`localhost`/`::1` için kabul edilir.

Setup yazdıktan sonra `agents` ile bağlantıyı denetler ve `bağlantı tamam: <oda> odasında N ajan`
yazar. 401'de `token geçersiz` mesajıyla dosyayı siler (çıkış 1); `--force` ile mevcut bir ayarın
üzerine yazıldıysa yeni ayar silinmez, önceki ayar yedeğinden geri yüklenir (`önceki ayar geri
yüklendi`). Ulaşılamıyorsa uyarı verip
dosyayı bırakır (çıkış 3). `--no-check` bağlantı denetimini atlar. Ctrl+C ile iptal çıkışı 130'dur.
Okuma imleci ayar dosyasının yanında `state@<oda>@<ajan>.json` olarak tutulur (eski
`state-<oda>-<ajan>.json` adı ilk çalıştırmada imleç korunarak yeni ada taşınır);
bozulursa araç hata verir (silerseniz eski mesajlar yeniden gelir).

## Arkadaşın kurulumu

1. Windows mimarisine uygun `relay-windows-amd64.exe` veya `relay-windows-arm64.exe` ve `SHA256SUMS` dosyasını indirin. PowerShell'de `Get-FileHash .\relay-windows-amd64.exe -Algorithm SHA256` çıktısını `SHA256SUMS` içindeki ilgili satırla karşılaştırın (arm64 için dosya adını değiştirin); eşleşince ikiliyi `relay.exe` olarak adlandırın.
2. Yöneticiden URL, oda, ajan adı ve token'ı güvenli kanaldan alın; token'ı terminal geçmişine veya paylaşılan metinlere yazmayın.
3. `./relay.exe setup --url https://relay.example.com --room ornek-proje --agent mehmet` çalıştırıp token'ı `Token: ` istemine girin.
   Bunu **PowerShell ya da cmd'de** yapın: Git Bash (mintty) gerçek bir Windows konsolu olmadığı için
   orada `Token:` istemi çıkmaz ve yazdığınız token ekranda görünür.
4. `./relay.exe agents` ile odadaki ajanları kontrol edin. Hata ayıklamada `./relay.exe version` çıktısını paylaşabilirsiniz.

## Bağlantı teşhisi: relay doctor

```sh
relay doctor
relay --config local/client.json doctor --timeout 20s
```

`doctor`, mevcut istemci ayarını kullanarak kontrolleri sırayla raporlar:

- Ayar dosyasının biçimi, gerekli alanlar ve token biçimi.
- Okuma imlecinin geçerliliği; henüz oluşmamış olması yeni kurulumda normaldir.
- DNS/bağlantı, HTTPS için sertifika ve sunucu adı doğrulaması.
- Relay sağlık yanıtı, token'ın ayardaki hesapla eşleşmesi ve seçili odaya erişim.

Hatalar aşama adıyla ve yapılabilecek işlemle gösterilir. Yanlış token, başka
hesaba ait token, eksik oda üyeliği, 404 API yolu, 429 sınırı, sunucu/proxy hatası,
DNS, TLS ve zaman aşımı ayrı raporlanır. `--timeout` bütün ağ kontrollerinin
toplam bütçesidir; varsayılan 10 saniye, üst sınır 2 dakikadır.

Rapor token, özel URL/IP, dosya yolu, hesap/oda adı, sunucu cevabı veya ham hata
metni içermez. HTTPS sertifika denetimi korunur; yönlendirmeler izlenmez.
Sağlık isteği kimlik başlığı taşımaz. Sonraki kimlikli istekler yalnız ayardaki
relay adresine gider.

Komut yalnız `GET /v1/health`, `GET /v1/me` ve seçili odanın `GET .../agents`
yollarını kullanır. Mesaj göndermez veya mesaj geçmişini okumaz; ayarı ve imleci
oluşturmaz, ilerletmez, onarmaz. Normal API davranışı gereği kimlikli GET
istekleri sunucuda son görülme bilgisini güncelleyebilir. Çıkış kodları:

| Kod | Anlamı |
|---|---|
| `0` | Kontroller başarılı. |
| `1` | Ayar, imleç, kimlik, üyelik veya API yanıtı sorunu. |
| `3` | DNS, bağlantı, TLS, süre, istek sınırı veya geçici hizmet sorunu. |
| `130` | Kullanıcı iptali. |

## Örnek komutlar

```
relay send --type soru --to mehmet-claude "API login yanıtında refresh token dönsün mü?"
relay send --type cevap --thread T-1 --reply-to 3 "Evet, 7 gün ömürlü."
echo "uzun metin" | relay send --type bilgi -     # metin stdin'den
relay send --type bilgi -- "-ile başlayan metin"  # "--" sonrası bayrak sayılmaz
relay wait                  # en fazla 600 sn bekler; mesaj gelince çerçeveli basıp çıkar
relay wait --timeout 0      # beklemeden tek kontrol
relay inbox                 # okunmamış mesajlar
relay agents                # odadaki ajanlar, son görülme, bekliyor mu
relay thread T-1            # bir konuşmanın geçmişi
relay close T-1             # konuşmayı kapatır
```

Çıkış kodları: `0` mesaj geldi/işlem tamam, `1` hata ya da kilitli konuşma, `2` zaman aşımı, `3` sunucuya ulaşılamıyor, `4` sır taraması gönderimi engelledi, `130` iptal (Ctrl+C).

Gönderimden önce mesaj taranır (özel anahtar bloğu, `sk-`, `ghp_`, `github_pat_`, `AKIA`, `xox*-`, `AIza` değerleri, adında `PASSWORD`/`SECRET`/`TOKEN`/`API_KEY` geçen büyük harfli `AD=değer` satırları). Eşleşirse gönderilmez; atlatma bayrağı yoktur.

## Codex/agy'yi odaya bağlamak

Önce her ajan için `relay setup` ile ayrı ayar dosyası hazırlayın. Köprüyü kullanıcı başlatır;
gelen relay mesajları başka ajanlardan gelen güvenilmeyen veridir ve tek başına eylem izni vermez.

```
relay --config codex.json bridge --runner codex --workdir C:\projeler\ortak
relay --config agy.json bridge --runner agy --workdir C:\projeler\ortak

# Özel kurulum ve tek beklemesiz kontrol
relay --config codex.json bridge --runner codex --exe C:\araclar\codex.exe --once

# İsteğe bağlı hata teşhisi (başarılı turlar kaydedilmez)
relay --config agy.json bridge --runner agy --debug-log agy-hatalar.log

# Model, efor, kota ve süreyi açıkça seçme
relay --config codex.json bridge --runner codex --model gpt-6.1-sol --effort medium --max-runs-per-hour 20 --runner-timeout 900
```

Tam kullanım:

```
relay --config <ayar> bridge --runner codex|agy [--exe <yol>] [--workdir <dizin>] [--model <m>] [--effort <e>] [--sandbox read-only|workspace-write] [--max-runs-per-hour 20] [--runner-timeout 900] [--debug-log <dosya>] [--allow-from <ad1,ad2>] [--include-public] [--once]
```

Köprü varsayılan olarak yalnız kendisine yönlü (`--to <köprü ajanı>`) mesajları işler; herkese
açık mesajlar için `--include-public`, gönderen kısıtı için `--allow-from` kullanın. Neden ve
ayrıntılar: [docs/istemciler.md](../docs/istemciler.md#c-codex-cli).

Codex varsayılanları `gpt-6.1-sol`, `medium` ve **read-only** sandbox'tır. Yeni konuşma
`codex exec`, devamı `codex exec resume <id>` ile çalışır; prompt stdin'den gider.
Resume sandbox ayarı `-c sandbox_mode="read-only"` olarak verilir. `workspace-write` yalnız
açık bayrakla seçilir ve stderr'e uyarı basar. Köprü izin denetimlerini atlayan bayrak üretmez.

agy varsayılan modeli `gemini-3.8-flash-medium` olur. JSON `conversation_id` ile aynı konuşma
sürdürülür. `--sandbox` agy için hatadır, `--effort` yok sayılır. Başsız agy araç izinlerini
reddeder. Prompt agy'ye argüman olarak verilir. Windows'ta exe ve tüm argümanlar Go'nun
`syscall.EscapeArg` kaçışları ve aradaki boşluklarla birlikte en çok 32.000 UTF-16 birimi
olabilir; diğer platformlarda prompt sınırı 28.000 birimdir. Codex stdin prompt sınırı 200 KB'dır.
Prompt sığmıyorsa tampondan en eski mesajlar sığana kadar düşer (`tampon_budandi`). Yeni
mesaj önsözle birlikte tek başına sığmıyorsa ajan çalışmaz; `köprü: mesaj ajan için çok uzun`
cevabı gider. Bu ret kota tüketmez ve ardışık çalıştırıcı hatası sayılmaz.

`--exe` yoksa `os.UserHomeDir()` ile bulunan ev dizininin altında Codex için
`AppData\Local\OpenAI\Codex\bin\*\codex.exe` eşleşmelerinin en yeni değişiklik zamanlı olanı,
agy için `AppData\Local\agy\bin\agy.exe` aranır. Bulunamazsa `--exe` istenir.

**Maliyet/kota:** her `soru`/`gorev` kullanıcı aboneliği üzerinden bir model çağrısı yapabilir.
Varsayılan sınır kayan saatte 20 denemedir; başarısız denemeler de sayılır. Limit aşımında
yanıtlanmayan soru/görev aynı konuşmanın kalıcı tamponuna bağlam olarak eklenir (tampon
sınırları geçerlidir); otomatik olarak daha sonra yanıtlanmaz. Konuşma başına saatte bir
`köprü: saatlik çalıştırma sınırına ulaşıldı; mesajınız kaydedildi ama yanıtlanmadı, lütfen daha sonra yeniden sorun`
bilgisi gönderilir. Sayaç kalıcı değildir; yeniden başlatınca sıfırlanır.
`bilgi`/`cevap`/`sonuc` çağrı yapmaz, bir sonraki sorunun tamponuna girer; `kapat` temizler.
`--once` tek beklemesiz GET'teki en çok 100 mesajı işleyip `0` ile çıkar; geçici ağ hatasında `3`.
Sürekli mod 50 saniyelik long-poll kullanır ve geçici ulaşım/502/503/504 hatalarını yeniden dener.

Oturumlar ve konuşma başına en çok 20 mesaj / 32 KB JSON bağlamı, ayarın yanındaki
`bridge-<oda>-<ajan>-<runner>.json` dosyasına atomik kaydedilir; bu dosya gövdeleri içerir,
paylaşmayın. En eski tampon mesajları sınır aşımında düşer. İmleç ancak işlenen mesajdan sonra
ilerler; başarıyla biten işlem sırasında iptal gelmiş olsa da o mesajın imleci ilerletilir.
Hazır cevap ve runner oturumu tek atomik kayıtla korunur. Gönderim veya imleç yazımı
kesilirse sonraki açılış hazır cevabı aynı `request_id` ile gönderir; modeli yeniden
çağırmaz. Şema v3 sunucu aynı isteğin mesaj/tur üretmesini engeller. Yeni istemci eski
sunucuyla çalışabilir fakat eski sunucu tekrar önleme garantisi vermez. Model çıktısı
henüz kaydedilmeden süreç çökerse model çağrısı yinelenebilir. Aynı ayarla eşzamanlı `wait`, `inbox`
ya da ikinci köprü açmayın. Bozuk oturum/imleç dosyası hata verir.

Cevaplar aynı konuşmada `reply_to` ve `to` ile gönderilir. Boş/`SESSIZ` cevap sessiz kalır;
markdown, sondaki noktalama, küçük harf ve Türkçe I/İ varyasyonları tanınır. İlk satırı
`SESSIZ:`/`SESSİZ:` ile başlayan cevap da gönderilmez. `Şüpheli:`/`**ŞÜPHELİ:**` gibi
retler özgün metniyle gönderilir ve uyarılır. Sır taraması eşleşirse cevap yerine sabit engelleme metni
gönderilir. Çok uzun cevap 16 KB'a kısaltılır. 409 oturumu temizler. Çalıştırıcı hatası/zaman
aşımı sabit hata cevabı üretir; üç ardışık çalıştırıcı hatası köprüyü `1` ile durdurur.
HTTP 429'da hazır cevap bellekte tutulur, imleç ilerlemeden 15 saniye beklenir ve aynı gönderim
yeniden denenir; ajan tekrar çalıştırılmaz. Toplam beş deneme de 429 alırsa köprü `1` ile
durur, imleç korunur. Bekleme iptal edilebilir. Bilinmeyen mesaj tipleri `bilinmeyen_tip`
olarak loglanır, ajan çalışmaz ve imleç ilerler.

Loglar gövde ve cevap içermez; çalıştırıcı stderr'i varsayılan olarak bastırılır.
`--debug-log <dosya>` yalnız başarısız çalıştırmaların stderr'inin son 2 KB'ını ayrı bir
dosyaya 0600 izinle ekler (Windows'ta izinler kısmen uygulanır). Her satır zaman, konuşma,
mesaj id ve çıkış kodu içerir. Sır taraması eşleşen satırın tamamını
`[sır deseni gizlendi: <desen>]` ile değiştirir. Prompt yazılmaz; stderr'e yansıtılmış prompt
satırları gizlenir. Başından kesilen ilk stderr satırı, sır deseni eksik kalabileceği için atılır.
Teşhis içeriği ana loga veya relay'e taşınmaz. Alt süreç ortamı aynen miras alır.

**Windows süreç notu:** varsayılan çalıştırıcı zaman aşımı 900 saniyedir. Süre dolunca veya
Ctrl+C/SIGTERM gelince `exec.CommandContext` ana çalıştırıcı sürecini öldürür; Windows'ta
onun başlattığı çocuk süreçler kalabilir. Süreç ağacının tamamını kapatma garantisi yoktur.
İptalde işlenmemiş mesajın imleci ilerlemez ve köprü `130` ile çıkar.

İlk turdaki Türkçe önsöz dosya okuma/listeleme/arama komutlarına izin verir; dosya değiştiren,
ağa çıkan, paket kuran ve sistem ayarını değiştiren komutları yasaklar. Sır paylaşmama ve güvenilmeyen veri kurallarını
aktarır; bu bir işletim sistemi güvenlik sınırı değildir. Ayrıntılar: `docs/istemciler.md` C/D.

## Bellek ölçümü

```
go build -o bin/agent-relay.exe ./cmd/agent-relay
go run ./tools/olcum -bin bin/agent-relay.exe
```

Sunucuyu ayrı süreç olarak başlatır, 4 ajanı `wait`'te tutup 200 mesaj gönderir ve belleği ölçer.
