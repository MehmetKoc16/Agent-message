# Araştırma

Kaynaklar: ChatGPT ile yapılan ön sohbet (11.10.2026) ve Claude'un aynı gün GitHub sayfalarından
ve Claude Code belgelerinden yaptığı kontrol.

## 1. Hazır projeler

| Proje | Ne yapıyor | Artı | Eksi / engel |
|---|---|---|---|
| [EcoRelay](https://github.com/josortmel/EcoRelay) | Her makinede bir hub daemon'ı, makineler arası WebSocket relay (~200 satır), oturum başına MCP channel (19 `relay_*` aracı). Claude Code, Codex, agy, Copilot CLI, Cursor, OpenCode desteği | Bizim istediğimize en yakın tasarım. Codex/agy bağlayıcıları var, çevrimdışı posta kutusu var | **Lisans PolyForm Noncommercial**: ticari kullanım yasak. Başlatma örneği `--dangerously-skip-permissions` istiyor. Bun gerektiriyor. 15 yıldız, genç proje |
| [agent-comms](https://github.com/ExaDev/agent-comms) | Ajan kimlikleri, odalar, özel mesajlar, web paneli. ECDSA ile cihaz kimliği, uçtan uca şifreleme | MIT lisans, güvenlik tasarımı ciddi | Varsayılan olarak **üçüncü taraf hub'ı** kullanıyor (`wss://mesh.exadev.io`); kendi sunucumuza yönlendirilebilir. Asıl odağı aynı makine. Karmaşık; v1→v2 kırıcı değişiklik yaşamış. Claude yolu hook + dosya kuyruğuyla çalışıyor |
| [claude-chat](https://github.com/vikrantjain/claude-chat) | Claude Code oturumlarını ortak bir WebSocket sunucusuna bağlıyor | Sade | ⚠️ (GPT'nin özeti, biz açmadık) **Kimlik doğrulama yok**, mesaj geçmişi yok |

### Sonuç 💡

**Relay'i kendimiz yazıyoruz.** Sebepleri:
1. KVKK verisi tutan canlı bir sunucuda kimlik doğrulamasız (claude-chat) ya da ticari
   kullanımı yasak (EcoRelay) bir kodu çalıştıramayız.
2. İhtiyacımız küçük: tek oda, birkaç ajan, long-poll. Yaklaşık 200–300 satır Python.
3. Sunucu kısıtları (RAM ≤ 100 MB, systemd, Docker yok) kendi kodumuzla kolayca karşılanır.

**Bu projelerden aldığımız fikirler:**
- Çevrimdışı posta kutusu: ajan kapalıyken mesajlar birikir (EcoRelay)
- `agy agentapi send-message` ile çalışan agy'ye mesaj itme (EcoRelay)
- Tüm bağlantıların istemciden dışarı doğru kurulması, PC'lerde port açılmaması (EcoRelay)
- Gönderen kimliği sunucuda zorunlu tutulur, varsayılan "reddet" (agent-comms)
- Hook ile uyandırma alternatifi (agent-comms): `Stop` / `UserPromptSubmit` hook'ları.
  A yöntemi tutmazsa yedek plan.

## 2. Claude Code Channels

Kaynak: https://code.claude.com/docs/en/channels-reference (11.10.2026'da okundu ✅)

- Channel, Claude Code'un alt süreç olarak başlattığı (stdio) yerel bir MCP sunucusudur.
  `capabilities.experimental['claude/channel']` bildirir ve `notifications/claude/channel`
  ile çalışan oturuma olay iter. Olay Claude'a `<channel source="..." ...>` etiketiyle görünür.
- İki yönlü kullanım: MCP aracı olarak bir `reply` aracı sunulur.
- **Araştırma önizlemesi.** Kendi channel'ımız onaylı listede olmadığı için
  `claude --dangerously-load-development-channels server:<ad>` ile başlatılmalı. Bu bayrak
  yalnız izin listesini atlar, izin denetimlerini kapatmaz. `-p` (başsız) modda yok sayılır.
- Claude meşgulken gelen olaylar kuyruğa girer, sonraki turda topluca işlenir.
- Belge **gönderen doğrulamasını** zorunlu görüyor: "An ungated channel is a prompt injection
  vector." Kontrol oda kimliğine göre değil, gönderen kimliğine göre yapılmalı.
- İzin aktarımı (`claude/channel/permission`): araç onaylarını uzaktan vermek. **Bunu
  kullanmayacağız.** Karşı ajanın bizim oturumumuzda izin verebilmesi kabul edilemez.
- VS Code panelinde dev bayrağıyla başlatmanın belgelenmiş bir yolu yok. Bu yüzden panel
  için A yöntemi (arka plan bekleme) birincil.

## 3. Ajanların ilgili özellikleri

Ayrıntı [envanter.md](envanter.md#ajanların-bu-iş-için-önemli-yetenekleri-yardım-çıktılarından-)'de.
Kısaca: Codex `exec resume`, agy `--conversation` ve `agentapi send-message`. İkisinde de
başsız modda konuşma sürdürmek mümkün görünüyor.

## 4. GPT sohbetinden notlar

- GPT de "yalnız yönetici ajanlar konuşsun, Codex/agy yerel yardımcı kalsın" önerdi.
  Bizim varsayılanımız da bu.
- GPT, Codex tarafı için `codex app-server` + `turn/start` önerdi. Bizim makinede app-server
  "[experimental]" görünüyor ✅. Daha basit olan `exec resume` ile başlayıp gerekirse
  app-server'a geçeceğiz.
- GPT'nin önerdiği prompt eki (iletişim kuralları) Faz 2'de prompt.md'ye uyarlanacak.
