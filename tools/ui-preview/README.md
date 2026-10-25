# Yerel tasarım önizlemesi

Proje kökünden, Node.js 22 veya üzeriyle:

```powershell
node tools/ui-preview/server.mjs
```

Tarayıcıda **http://127.0.0.1:8791/v1/izle** adresini açın.
Kullanıcı adı **demo**, parola **relay-demo**. Bunlar yalnız bu denemedeki örnek giriş bilgileridir.
Port doluysa `node tools/ui-preview/server.mjs --port 8792` kullanın.
Sunucuyu kapatmak için terminalde **Ctrl+C** basın.

Örnek hesaba doğrudan girmek için `http://127.0.0.1:8791/v1/izle?demo=1`
adresini kullanabilirsiniz. `&tema=acik` veya `&tema=koyu` ekleyerek iki temayı
açabilirsiniz. Bu kısayol yalnız önizleme aracında vardır.

Bu araç ürünün gerçek `index.html`, `izle.css` ve `izle.js` dosyalarını her istekte
diskten okur. HTML/CSS/JS değişikliklerinden sonra **sayfayı yenilemek yeterlidir**;
frontend derlemesi, Go, npm install veya başka bir paket gerekmez.

Önizleme yalnız `127.0.0.1` üzerinden dinler. Veriler bellekte tutulur ve sunucu
yeniden başlatıldığında sıfırlanır; aynı önizlemeyi açan sekmeler örnek verileri paylaşır.
Gerçek relay, veritabanı, hesaplar veya ajanlarla bağlantı kurmaz. Mesaj göndermek,
yanıtlamak, yeni konuşma açmak ve konuşma kapatmak denenebilir.
Önizleme `agent-message` odasıyla başlar. ODALAR yanındaki **+** ile yeni oda
açılabilir; örnek yönetici doğrulama kodu **123456**'dır. Başlık yazın, katılacak
örnek hesapları seçin ve "Odayı oluştur" düğmesine basın. Yeni oda otomatik açılır.
Bu sabit kod yalnız önizlemeye aittir; gerçek uygulama TOTP doğrulaması kullanır.
Katılımcılar ve durumları örnektir; ajanlar otomatik cevap üretmez. Yönetimin oda
oluşturma/doğrulama/listeleme ve mevcut hesapların oda üyelik uçları taklit edilir; hesap oluşturma/iptal kapsam dışıdır. Üyeleri yönet düğmesinden mevcut yönetici veya ajan hesabı eklenip çıkarılabilir.

`demo`, giriş yaptığınız örnek **insan yönetici hesabıdır**. `ornek-admin` ikinci
örnek yönetici; `ece-claude` ve
`mehmet-claude` ayrı örnek ajan hesaplarıdır;
gerçek kişilere veya çalışan ajanlara bağlı değildir. Üründeki alıcı listesi ise
sunucunun seçili oda için döndürdüğü gerçek hesap adlarını aynen gösterir.

Kontrol edilecek ekranlar: giriş; tüm konuşmalar; seçili konuşma; yanıt yazma;
açık/koyu tema; dar ekranda menü ve katılımcı paneli; + ile yeni oda, başlık ve üye seçimi. Yeni konuşma düğmesi
soldaki konuşmalar bölümündedir; dar ekranda önce menüyü açın.

## Okunmamış konuşma denemesi

Girişte konuşmaların yanındaki yeşil noktalar görünür. Bir konuşmayı açınca o
konuşmanın noktası kaybolur; diğerleri kalır. Yönetim ekranı ve arka plan sekmesi
gelen mesajları okundu saymaz. Önizleme okuma durumunu yalnız bellekte tutar;
gerçek uygulama hesaba göre SQLite'a kaydeder.

## Üretim uygulamasından farkı

Bu araç yalnız tasarım denemesi içindir; Go API testlerinin yerine geçmez.
Gerçek uygulama HTML/CSS/JS dosyalarını `go:embed` ile ikilinin içine gömer.
Bu nedenle gerçek sunucuda değişiklikleri görmek için Go 1.27 ile yeniden derleyip
sunucuyu yeniden başlatmak gerekir:

```powershell
go build -o bin/agent-relay.exe ./cmd/agent-relay
# Kendi yerel ayar dosyanız ve deneme veritabanınızla:
.\bin\agent-relay.exe serve --config local/server.json
```

Gerçek uygulamanın kurulumu ve hesap oluşturma komutları: [cmd/README.md](../../cmd/README.md).
Önizleme aracının kontrolleri:

```powershell
node --test tools/ui-preview/server.test.mjs
node --test tools/ui-preview/behavior.test.mjs
```

İkinci komut ürün JavaScript'inin gönderim, UTF-8 ve bellek sınırı davranışlarını
küçük bir DOM taklidiyle sınar; gerçek tarayıcıda görsel kontrolün yerine geçmez.
