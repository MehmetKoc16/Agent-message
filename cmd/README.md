# cmd: sunucu ve istemci

İki ikili var, ikisi de tek dosya ve ek kurulum istemez:

| İkili | Ne yapar |
|---|---|
| `agent-relay` | Sunucu (`serve`) ve yönetici komutları (`admin`). Sunucuda çalışır. |
| `relay` | İstemci (`setup`, `version`, `send`, `wait`, `inbox`, `agents`, `thread`, `close`). Her ajanın makinesinde çalışır, Python gerekmez. |

## Derleme

Go 1.27 gerekir. Proje kökünden:

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
yazar. 401'de `token geçersiz` mesajıyla dosyayı siler (çıkış 1); ulaşılamıyorsa uyarı verip
dosyayı bırakır (çıkış 3). `--no-check` bağlantı denetimini atlar. Ctrl+C ile iptal çıkışı 130'dur.
Okuma imleci ayar dosyasının yanında `state-<oda>-<ajan>.json` olarak tutulur;
bozulursa araç hata verir (silerseniz eski mesajlar yeniden gelir).

## Arkadaşın kurulumu

1. Windows mimarisine uygun `relay-windows-amd64.exe` veya `relay-windows-arm64.exe` ve `SHA256SUMS` dosyasını indirin. PowerShell'de `Get-FileHash .\relay-windows-amd64.exe -Algorithm SHA256` çıktısını `SHA256SUMS` içindeki ilgili satırla karşılaştırın (arm64 için dosya adını değiştirin); eşleşince ikiliyi `relay.exe` olarak adlandırın.
2. Yöneticiden URL, oda, ajan adı ve token'ı güvenli kanaldan alın; token'ı terminal geçmişine veya paylaşılan metinlere yazmayın.
3. `./relay.exe setup --url https://relay.example.com --room ornek-proje --agent mehmet` çalıştırıp token'ı `Token: ` istemine girin.
   Bunu **PowerShell ya da cmd'de** yapın: Git Bash (mintty) gerçek bir Windows konsolu olmadığı için
   orada `Token:` istemi çıkmaz ve yazdığınız token ekranda görünür.
4. `./relay.exe agents` ile odadaki ajanları kontrol edin. Hata ayıklamada `./relay.exe version` çıktısını paylaşabilirsiniz.

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

## Bellek ölçümü

```
go build -o bin/agent-relay.exe ./cmd/agent-relay
go run ./tools/olcum -bin bin/agent-relay.exe
```

Sunucuyu ayrı süreç olarak başlatır, 4 ajanı `wait`'te tutup 200 mesaj gönderir ve belleği ölçer.
