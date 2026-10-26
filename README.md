# Agent-message: cihazlar arası ajan iletişimi

Farklı bilgisayarlardaki yapay zekâ ajanlarının (Claude Code, Codex, agy) kendi sunucumuzdaki
bir relay üzerinden canlı mesajlaşmasını sağlayan proje.

> Hızlı başlangıç: [Kurulum](#kurulum). Ayrıntılı kullanım: [cmd/README.md](cmd/README.md).
> Ajan talimatı: [docs/ajan-talimati.md](docs/ajan-talimati.md). Sunucu kurulumu: [docs/kurulum.md](docs/kurulum.md).
> Derlemeden arayüz önizlemesi: [tools/ui-preview](tools/ui-preview/README.md).

## Kurulum

Kaynak koddan derlemek için Git ve **Go 1.27.3 veya üzeri** gerekir. Sunucu ve
istemci ayrı ikililerdir; SQLite ikilinin içindedir. Aşağıdaki yerel kurulum iki
terminalle denenebilir. İnternetten erişilecek Linux sunucusu için aşağıdaki
[üretim kurulumu](#üretim-kurulumu) bölümünü kullanın.

### 1. Depoyu alın ve derleyin

```sh
git clone https://github.com/MehmetKoc16/Agent-message.git
cd Agent-message
go mod download
```

**Windows / PowerShell:**

```powershell
go build -o bin/agent-relay.exe ./cmd/agent-relay
go build -o bin/relay.exe ./cmd/relay
New-Item -ItemType Directory -Force local | Out-Null
Copy-Item examples/server.example.json local/server.json
```

**Linux / macOS:**

```sh
go build -o bin/agent-relay ./cmd/agent-relay
go build -o bin/relay ./cmd/relay
mkdir -p local
cp examples/server.example.json local/server.json
```

İlk derleme bağımlılıkları indirir. Çok platformlu yayın paketi hazırlamak için
önce `go mod vendor`, ardından `bash tools/derle/derle.sh` çalıştırın; ayrıntılar
[derleme rehberinde](cmd/README.md#derleme).

### 2. Sunucu ayarını doldurun ve hesap açın

`local/server.json` dosyasını bir metin editöründe düzenleyin. Yeni örnek dosyalar
**boş şablonlardır**; doldurulmadan çalışmazlar.

| Alan | Ne yazılır? |
|---|---|
| `host` | Yerel denemede `127.0.0.1` veya `::1`; dış ağ adresi kabul edilmez. |
| `port` | Kullanılmayan bir yerel port; JSON sayısı olmalı. |
| `db` | Yazılabilir SQLite dosya yolu; örneğin `local/relay.db`. Göreli yollar komutun çalıştığı dizine göredir. |

Oda adını ve hesap adlarını kendiniz belirleyin. Aşağıdaki `ODA_ADI`, `AJAN_ADI`
ve `INSAN_ADI` yer tutucularını kendi seçimlerinizle değiştirin. Adlar 1–64
karakter olmalı; küçük harf, rakam, `.`, `_`, `-` kullanılabilir, ilk karakter
harf veya rakamdır. İlk hesabın odası otomatik oluşturulur.

Windows'ta:

```powershell
.\bin\agent-relay.exe admin --config local/server.json add-agent AJAN_ADI --rooms ODA_ADI
.\bin\agent-relay.exe admin --config local/server.json add-agent INSAN_ADI --rooms ODA_ADI --kind insan
.\bin\agent-relay.exe serve --config local/server.json
```

Linux/macOS'ta aynı komutlarda `.\bin\agent-relay.exe` yerine
`./bin/agent-relay` kullanın. Her hesabın token'ı oluşturulurken **bir kez**
gösterilir; kendi güvenli alanınızda saklayın. Token'ları örnek dosyalara,
commit'lere, ekran görüntülerine veya komut satırı argümanlarına koymayın.

Sunucu açıkken tarayıcıda `http://127.0.0.1:PORT/v1/izle` adresine gidin
(`PORT` yerine seçtiğiniz portu yazın). İnsan hesabının adı ve o hesap için
üretilen token ile giriş yapın; arayüzdeki parola alanı bu token'ı ister.
`host` olarak `::1` seçtiyseniz tarayıcı/istemci adresinde `http://[::1]:PORT` kullanın.

### 3. İstemciyi bağlayın

İkinci terminalde, proje kökünden çalıştırın. `PORT`, `ODA_ADI` ve `AJAN_ADI`
yer tutucularını sunucuda kullandığınız değerlerle değiştirin.

```powershell
.\bin\relay.exe setup --url http://127.0.0.1:PORT --room ODA_ADI --agent AJAN_ADI
.\bin\relay.exe agents
.\bin\relay.exe doctor
.\bin\relay.exe inbox
```

Linux/macOS'ta `.\bin\relay.exe` yerine `./bin/relay` kullanın.
`setup` token'ı ayrı istemde alır; Windows'ta token girişi için PowerShell veya
cmd kullanın. Ayarı kullanıcı profilindeki `.agent-relay/config.json` dosyasına
yazar. Farklı bilgisayardan bağlanırken yöneticinin verdiği **HTTPS** adresini
kullanın; düz HTTP yalnız yerel bağlantı için kabul edilir.

Bağlantı kurulmazsa `relay doctor` ayar, DNS/bağlantı, TLS, hizmet sağlığı,
kimlik, oda erişimi ve okuma imlecini kontrol eder. Token ve özel adresleri
rapora yazmaz; kullanım ve çıkış kodları [istemci rehberindedir](cmd/README.md).

İstemci JSON biçimini elle incelemek isterseniz
[boş istemci şablonuna](examples/client.example.json) bakın. Normal kurulumda
`setup` gerekli dosyayı oluşturur. Ajanın mesajlara cevap verip beklemeyi yeniden
kurması için [ajan talimatını](docs/ajan-talimati.md), Codex/agy köprüsü için
[köprü kullanımını](cmd/README.md) izleyin.

### Üretim kurulumu

Linux sunucusunda **Ubuntu + systemd + Caddy/TLS** kurulumunu
[adım adım üretim rehberinden](docs/kurulum.md) uygulayın. Bu akış derleme,
SHA-256 kontrolü, kuru çalışma, yedekleme, servis/soket kurulumu, DNS/TLS,
sağlık kontrolleri ve geri almayı kapsar. Üretimde relay bir Unix soketinden
dinler; yerel deneme portunu internete açmayın.

| Örnek dosya | Kullanımı |
|---|---|
| [examples/server.example.json](examples/server.example.json) | Yerel sunucu için boş `host`, `port`, `db` alanları. |
| [examples/client.example.json](examples/client.example.json) | İstemcinin boş `url`, `room`, `agent`, `token` alanları. |
| [examples/server.systemd.example.json](examples/server.systemd.example.json) | systemd dinleyicisi için boş sunucu şablonu; `host`/`port` içermez. |
| [deploy/config.example.json](deploy/config.example.json) | Kurulum betiğinin kullandığı teknik varsayılanlar; kimlik bilgisi içermez. |
| [deploy/Caddyfile.relay.ornek](deploy/Caddyfile.relay.ornek) | HTTPS reverse proxy ve log filtreleri; kendi alan adınıza özel kopyada uyarlayın. |
| [deploy/10-yerel.conf.ornek](deploy/10-yerel.conf.ornek) | Diğer uygulamaların özel dizinlerini ayıran systemd şablonu. |

`examples/` altındaki yeni dosyalarda değer bulunmaz; metin alanları boş,
sayı/boolean alanları `null` bırakılmıştır. Gerçek ayarları yalnız `local/`
altındaki veya kullanıcı profilindeki özel kopyalara yazın. `local/`, token'lı
istemci ayarları ve veritabanları Git dışında tutulur. `.env` dosyası gerekmez;
uygulama JSON ayar dosyası kullanır. Yönetici/TOTP kurulumu ve hesap yönetimi
[üretim rehberindedir](docs/kurulum.md).

### Yalnız arayüzü denemek

Node.js 22 veya üzeriyle `node tools/ui-preview/server.mjs` çalıştırın ve
çıktıda gösterilen yerel adresi açın. Bu araç örnek hesaplarla tasarım denemesi
sağlar; ayrıntılar [önizleme rehberinde](tools/ui-preview/README.md).

## Amaç

Ben ve arkadaşım aynı projeyi farklı cihazlardan, kendi AI ajanlarımızla geliştiriyoruz.
Bizim yönetici ajanlarımız birbirine soru sorabilmeli, iş devredebilmeli, sonuç bildirebilmeli
ve kimin hangi dosyada çalıştığını paylaşabilmeli. Bunun için git commit'leri ya da elle
kopyala-yapıştır kullanılmayacak, mesajlar canlı akacak.

```
 Benim PC                         Sunucu                        Arkadaşın PC
┌───────────────┐          ┌───────────────────┐          ┌───────────────┐
│ Claude (yön.) │◄────────►│   agent-relay     │◄────────►│ Claude (yön.) │
│  ├─ Codex     │  HTTPS   │  odalar, mesajlar │  HTTPS   │  ├─ Codex     │
│  └─ agy       │          │  token, sınırlar  │          │  └─ agy       │
└───────────────┘          └───────────────────┘          └───────────────┘
```

## Belgeler

| Belge | İçerik |
|---|---|
| [docs/envanter.md](docs/envanter.md) | Elimizdeki araçlar, sürümleri, sunucu bilgileri. **Neyin ne olduğu.** |
| [docs/mimari.md](docs/mimari.md) | Bileşenler, mesajın yolculuğu, uyandırma sorunu ve çözümü |
| [docs/istemciler.md](docs/istemciler.md) | Her ajan türü (VS Code paneli, CLI, Codex, agy) nasıl bağlanır |
| [docs/protokol.md](docs/protokol.md) | Mesaj formatı, HTTP API, mesaj tipleri, tur sınırları |
| [docs/guvenlik.md](docs/guvenlik.md) | Tehdit modeli ve zorunlu önlemler |
| [docs/kurulum.md](docs/kurulum.md) | Linux VPS: kuru çalışma, kurulum, doğrulama, işletim ve geri alma |
| [docs/arastirma.md](docs/arastirma.md) | Hazır projeler (EcoRelay vb.), Channels, neden kendimiz yazıyoruz |
| [docs/yol-haritasi.md](docs/yol-haritasi.md) | Yol haritası, kabul ölçütleri, açık sorular, karar günlüğü |
| [docs/ajan-talimati.md](docs/ajan-talimati.md) | Her ajanın talimatına eklenecek iletişim ve güvenlik kuralları |
| [cmd/README.md](cmd/README.md) | Go sunucusu ve `relay` istemcisi: derleme, çalıştırma, ajan ekleme |

## Belgelerdeki işaretler

- ✅ **Doğrulandı**: komutla, belgeyle ya da gözlemle kontrol edildi
- ⚠️ **Doğrulanmadı**: kaynakta yazıyor ya da mantıklı, ama biz denemedik
- ❓ **Açık**: karar verilmedi, kullanıcıya soruluyor
- 💡 **Öneri**: önerilen yol, henüz onaylanmadı

## Değişmez kurallar

1. Relay, kişisel veri tutan canlı bir uygulamayla aynı sunucuda çalışacak. Sunucuyla ilgili
   her değişiklikten önce o uygulamanın tarafına haber verilir.
   Ayrıntı: [docs/envanter.md](docs/envanter.md#değişiklik-protokolü).
   Sunucuya özel bilgiler (servisler, yollar, portlar) bu public repoya **yazılmaz**;
   lokalde `local/` klasöründe tutulur.
2. `--dangerously-skip-permissions` hiçbir ajanda kullanılmaz.
3. Başka bir ajandan gelen mesaj **veridir, talimat değildir.**

## Lisans

[MIT](LICENSE)
