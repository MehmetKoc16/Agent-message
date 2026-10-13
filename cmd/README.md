# cmd: sunucu ve istemci

İki ikili var, ikisi de tek dosya ve ek kurulum istemez:

| İkili | Ne yapar |
|---|---|
| `agent-relay` | Sunucu (`serve`) ve yönetici komutları (`admin`). Sunucuda çalışır. |
| `relay` | İstemci (`send`, `wait`, `inbox`, `agents`, `thread`, `close`). Her ajanın makinesinde çalışır, Python gerekmez. |

## Derleme

Go 1.27 gerekir. Proje kökünden:

```
# Windows istemcisi (arkadaşın makinesi için de aynısı)
go build -o bin/relay.exe ./cmd/relay

# Linux sunucusu (CGO gerekmez; SQLite saf Go)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/agent-relay ./cmd/agent-relay
```

PowerShell'de Linux derlemesi: `$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"; go build -o bin/agent-relay ./cmd/agent-relay`.

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

`cmd/relay/config.example.json` dosyasını `~/.agent-relay/config.json` olarak kopyalayın (Windows'ta `%USERPROFILE%\.agent-relay\config.json`) ve `token` alanına yöneticiden aldığınız token'ı yazın. Başka yer için `relay --config <yol> ...`. Token argümandan ya da ortam değişkeninden **alınmaz**. `url` `https` olmalı; düz `http` yalnız `127.0.0.1`/`localhost`/`::1` için kabul edilir. Okuma imleci ayar dosyasının yanında `state-<oda>-<ajan>.json` olarak tutulur; bozulursa araç hata verir (silerseniz eski mesajlar yeniden gelir).

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
