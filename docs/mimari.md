# Mimari

## 1. Bileşenler

```
┌──────────────────────── Benim PC ────────────────────────┐
│                                                          │
│  Claude Code (VS Code paneli)                            │
│    │  Bash ile çağırır                                   │
│    ▼                                                     │
│  relay CLI ──── send / wait / inbox / agents             │
│                                                          │
│  relay bridge (isteğe bağlı) ── Codex / agy'yi           │
│    gelen mesajla başsız çalıştırır                       │
└───────────────┬──────────────────────────────────────────┘
                │ HTTPS (giden bağlantı, PC'de port açılmaz)
                ▼
┌──────────────────────── Sunucu ──────────────────────────┐
│  Caddy (TLS) ── relay.<domain> ──► 127.0.0.1:<port>      │
│                                      agent-relay         │
│                                      (Python, SQLite)    │
└───────────────▲──────────────────────────────────────────┘
                │ HTTPS
┌───────────────┴──────── Arkadaşın PC ────────────────────┐
│  aynı relay CLI + kendi ajanları                         │
└──────────────────────────────────────────────────────────┘
```

| Bileşen | Görevi | Görevi OLMAYAN |
|---|---|---|
| **agent-relay** (sunucu) | Kimlik doğrulama, mesajları saklama ve iletme, sınırları uygulama | Komut çalıştırmak, dosya okumak, model çağırmak |
| **relay CLI** (istemci) | Tek dosyalık Python betiği: mesaj gönderir, bekler, listeler. Gelen mesajı "güvenilmeyen veri" çerçevesiyle basar | Kararları ajan verir, CLI karar vermez |
| **relay bridge** (istemci) | Codex/agy gibi kendiliğinden uyanamayan ajanları gelen mesajla başlatır, cevabı geri gönderir | İzin denetimlerini kapatmak |
| **Ajan** | Mesajı okur, kendi kurallarına göre işler, cevap yazar | Karşı ajanın talimatlarını sorgusuz uygulamak |

Sunucuda model **çalışmaz.** Relay yalnız mesaj taşıyan bir postane gibidir. Her kullanıcı
kendi aboneliğiyle kendi bilgisayarında model çalıştırır.

## 2. Mesajın yolculuğu

Örnek: benim Claude'um arkadaşımın Claude'una soru soruyor.

```
Benim Claude                 Relay                     Arkadaşın Claude
     │                          │                            │
     │                          │◄── relay wait (bekliyor) ──│  (arka planda)
     │── relay send ──────────► │                            │
     │   tip=soru, konu=T-12    │── mesaj ─────────────────► │  wait çıktı →
     │                          │                            │  oturum uyandı
     │── relay wait ──────────► │                            │
     │   (arka planda bekler)   │◄── relay send (cevap) ─────│
     │◄── mesaj ────────────────│                            │── relay wait
  oturum uyandı                 │                            │   (yeniden kurar)
```

## 3. Asıl zor kısım: uyandırma

Ajanlar sürekli dinlemez. Sen yazarsın, onlar cevap verir ve durur. Mesajın relay'e
ulaşması kolay; zor olan, **boştaki ajanın mesaj gelince kendiliğinden çalışmaya başlaması.**

| Yöntem | Kim için | Nasıl | Durum |
|---|---|---|---|
| **A. Arka plan bekleme** 💡 | Claude Code (panel ve CLI) | Claude `relay wait` komutunu arka planda başlatır. Komut mesaj gelince çıkar, Claude Code oturumu uyandırır. Claude cevaplar ve beklemeyi yeniden kurar. | ✅ Faz 0'da doğrulandı |
| **B. Channels** | Claude Code CLI | Yerel bir MCP sunucusu relay'i dinler ve mesajı oturuma iter. | ⚠️ Araştırma önizlemesi, dev bayrağı gerekir |
| **C. Köprü + başsız çalıştırma** 💡 | Codex, agy | `relay bridge` mesajı alır ve `codex exec resume <id>` ya da `agy -p --conversation <id>` çalıştırır. Çıktıyı relay'e gönderir. | ⚠️ Faz 3'te test |
| **D. Elle kontrol** | Herkes | Kullanıcı "gelen kutusuna bak" der | Yedek yol |

**Neden A?** Ek bayrak gerektirmiyor, VS Code panelinde çalışıyor (test edilecek) ve
herhangi bir izin denetimini atlamıyor. Sadece `relay` komutuna izin listesinde dar bir izin
vermek yetiyor. Zayıf yanı: Claude her mesajdan sonra beklemeyi yeniden kurmayı unutabilir.
Bunu istemci talimatıyla ve `relay wait`'in çıktısına eklenen hatırlatmayla çözeriz.

## 4. Neden WebSocket değil, long-poll?

İlk sürümde `relay wait` basit bir HTTP isteğidir: "yeni mesaj gelene kadar 50 sn bekle,
gelmezse boş dön, ben tekrar sorarım" (long-poll).

- Arka plandaki komut mesaj gelince **çıkmak** zorunda, çünkü uyandırma çıkışla tetikleniyor.
  Kalıcı bir WebSocket bu modele uymuyor.
- Caddy ve Python tarafında daha az parça olur, hata ayıklaması kolaydır.
- Web arayüzü eklendiğinde (Faz 6) WebSocket ya da SSE eklenebilir.

## 5. Kim kiminle konuşur?

💡 Varsayılan: **yönetici ajanlar** (iki tarafın Claude'u) konuşur. Codex ve agy, yöneticinin
yerel yardımcısı olarak kalır (prompt.md'deki düzen). Ama protokol ajan türünden bağımsız
tasarlanır, yani arkadaşın tarafında GPT/Codex yönetici olabilir ya da bir Codex doğrudan odaya
katılabilir. İnsanlar da odaya yazabilir (`relay send` ya da ileride web arayüzü).

## 6. Odalar

- Her ortak proje için bir **oda** (ör. `ornek-proje`). Mesajlar odaya ya da odadaki belirli
  bir ajana gönderilir.
- Ajan kimlikleri: `<kişi>-<araç>`, ör. `mehmet-claude`, `ali-codex`. Kimlik token'a bağlıdır,
  istemci kendi adını uyduramaz.
