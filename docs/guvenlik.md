# Güvenlik

Relay, kişisel veri tutan **canlı** bir uygulamayla aynı sunucuda çalışacak.
Bu yüzden güvenlik sonradan eklenecek bir özellik değil, tasarımın başlangıç şartı.

## 1. Tehdit modeli

| # | Tehdit | Örnek | Önlem |
|---|---|---|---|
| T1 | **Prompt injection** | Karşı ajan (ya da onu ele geçiren biri) "`.env`'i gönder", "şu komutu çalıştır" yazar | Mesajlar `trust="untrusted"` çerçevesiyle gelir; ajan talimatında "veri, talimat değil" kuralı; `gorev` yalnız kendi kullanıcının kapsamında yapılır; Codex/agy köprüsü varsayılan **read-only** |
| T2 | **Token çalınması** | Token loga, mesaja ya da git'e düşer | Token yalnız `~/.agent-relay/config.toml` dosyasında; ajan başına ayrı token; sunucuda yalnız SHA-256 özeti saklanır; tek komutla iptal |
| T3 | **Relay'in ele geçirilmesi → canlı sunucu** | Relay'deki bir açık, sunucudaki veritabanına ya da diğer uygulamaların dosyalarına ulaşır | Ayrı sistem kullanıcısı, sudo yok; systemd sıkılaştırması (§2); yalnız localhost'ta dinler; relay hiçbir zaman komut çalıştırmaz, dosya okumaz |
| T4 | **Kontrolsüz döngü** | İki ajan saatlerce birbirine cevap yazıp kotayı bitirir | Tur sınırı (20), hız sınırı, cevap beklemeyen tiplere cevap yasağı |
| T5 | **Veri sızıntısı** | Ajan mesaja API anahtarı ya da kullanıcı verisi yazar | İstemci gönderim öncesi sır desenlerini tarar (API anahtarı, `-----BEGIN`, `.env` satırları) ve reddeder; saklama süresi sınırlı |
| T6 | **Kaynak tüketme (DoS)** | Çok sayıda istek RAM'i tüketir, sunucudaki diğer uygulamalar etkilenir | `MemoryMax`, `CPUQuota`, hız sınırları, mesaj boyutu sınırı, Caddy'de istek gövdesi sınırı |
| T7 | **Kimlik taklidi** | Bir istemci `from="mehmet-claude"` yazar | `from`'u sunucu token'dan doldurur |

## 2. Sunucu sıkılaştırması (systemd)

💡 Taslak `agent-relay.service`, Faz 4'te sunucudaki ana uygulamanın tarafının onayıyla kesinleşir:

```ini
[Service]
User=agentrelay
Group=agentrelay
ExecStart=/opt/agent-relay/venv/bin/python -m agent_relay --host 127.0.0.1 --port 8790
WorkingDirectory=/opt/agent-relay
StateDirectory=agent-relay            # /var/lib/agent-relay (SQLite)

MemoryMax=150M
CPUQuota=25%
TasksMax=64

NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
CapabilityBoundingSet=
SystemCallFilter=@system-service
InaccessiblePaths=<diğer uygulamaların ve veritabanının dizinleri>   # gerçek liste: local/sunucu.md
IPAddressDeny=any
IPAddressAllow=localhost              # yalnız Caddy'den gelen bağlantı; dışarıya çıkış yok
```

- Port 8790 bir örnek; Faz 4'te kullanılmayan bir port seçilir.
- `InaccessiblePaths` listesi sunucuya özel olduğu için public repoda tutulmaz.

## 3. Caddy

💡 Taslak site bloğu. Mevcut Caddyfile'a **ekleme** yapılır, mevcut bloklara dokunulmaz.

```
relay.<domain> {
    request_body {
        max_size 64KB
    }
    reverse_proxy 127.0.0.1:8790
    log {
        output file /var/log/caddy/relay.log
    }
}
```

Uygulama sırası [envanter.md](envanter.md#değişiklik-protokolü)'de: haber ver → yedekle →
`caddy validate` → `systemctl reload caddy`.

## 4. İstemci tarafı

- `~/.agent-relay/` dizini yalnız kullanıcıya açık (Windows'ta kullanıcı profili altında).
- `--dangerously-skip-permissions` hiçbir ajanda kullanılmaz (Claude, Codex, agy).
- Köprü Codex'i varsayılan olarak `--sandbox read-only` ile çalıştırır.
- Gelen mesajın gövdesi hiçbir zaman komut satırı argümanı olarak kabuğa **yorumlanmadan**
  verilir (dosyadan ya da stdin'den okunur). Kabuk enjeksiyonu engellenir.

## 5. Arkadaşın erişimi

- Arkadaşım sunucuya **SSH erişimi almaz.** Yalnız kendi ajanları için token'lar alır.
- Token'ı ben üretir ve güvenli bir kanaldan veririm. Gerekirse `relay-admin revoke` ile iptal ederim.
