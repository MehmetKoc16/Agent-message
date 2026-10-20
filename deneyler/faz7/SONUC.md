# Faz 7 sonucu: tarayıcıdan hesap yönetimi ✅ CANLI

Tarih: 19–20.10.2026. Sunucuya kurulum 20.10, 07:00 UTC'de, sunucu sahibinin onayıyla bakım penceresinde yapıldı.

## Ne yapıldı

| Parça | Ayrıntı |
|---|---|
| Yönetici hesabı | Ayrı bir insan hesabı. Yetkiyi yalnız SSH'taki `admin yonetici-yap` verir; web'den yönetici yaratılamaz |
| İkinci faktör | TOTP (RFC 6238, Google Authenticator). Aynı kod ikinci kez kullanılamaz. 5 hatalı kod 15 dk kilit getirir. Yönetim oturumu 10 dk sürer |
| `/v1/yonetim/*` | Hesap açma (token bir kez gösterilir), hesaba oda ekleme/çıkarma, hesap iptali, denetim kaydı. Yönetici hesaplarına web'den dokunulamaz |
| SSH komutları | `yonetici-yap`, `yonetici-al`, `odaya-ekle`, `odadan-cikar`, `kayit` |
| Şema v2 | Adımlı göç: v1 verisi korunur. Yeni sütunlar `yonetici`, `totp_secret`, `totp_last_step`; yeni tablo `yonetim_kayit` |
| Sohbet düzeltmeleri | Enter gönderir. Konuşma seçili değilse mesaj son konuşmaya devam eder. Gönderim görünümü süzmez (Ece'nin canlı denemesinden) |

Kod: Codex (gpt-6.1-sol, orta efor). İnceleme, yerel uçtan uca deneme ve testler: Claude. Sonuç 208 Go testi.

## Yerel uçtan uca deneme (19.10)

Gerçek ikili, v1 veritabanının kopyası üzerinde denendi:
- Göçte eski hesaplar ve mesajlar korundu.
- Yönetim uçları: kodsuz istek 401, yanlış kod reddedildi, aynı kodun tekrarı reddedildi.
- Hesap açmada `"yonetici": true` gönderilse de yok sayıldı.
- Var olan adla hesap açmak 409 döndü.
- Yöneticiyi iptal etmeye çalışmak 403 döndü.
- Denetim kaydında token, kod ya da oturum anahtarı geçmedi.

## Kurulum ve kontroller (20.10)

Sıra: kuru çalışma → servis durdurularak veri dizininin yedeği (boyutlar eşit) → `--uygula`.

| Kontrol | Sonuç |
|---|---|
| Relay sağlık (soket + HTTPS), `/v1/izle` | ✅ 200; sunulan JS repodakiyle aynı |
| Canlı uygulama (API ve site) | ✅ Kurulumdan önce ve sonra 200 |
| `systemd-analyze security` | ✅ 0.2 SAFE |
| Drop-in | ✅ 15 InaccessiblePaths |
| TCP dinleyicisi | ✅ Relay'e ait yok |
| Soket etkinleştirmesi | ✅ Servis durdurulunca soket active kaldı, ilk istekte servis kendiliğinden kalktı |
| Göç | ✅ 4 hesap ve eski mesajlar yerinde |
| Yönetici | ✅ SSH ile açıldı ve atandı. TOTP sırrı yalnız kullanıcının ekranında kaldı. Kullanıcı tarayıcıda Authenticator koduyla yönetim panelini açtı |

## Öğrenilenler

1. **Yönlü mesaj başka ajana görünmez.** İki Claude aynı hatayı habersiz düzeltti. Talimata "işe başlamadan önce herkese
   açık duyur" kuralı eklendi.
2. **Tarayıcıda yeni bir panel eklerken kapsayıcının taşma ayarına bakılmalı.** Ana kutu taşmayı gizlediği için panel
   kaymıyordu; tek satırla düzeldi.
3. **Özel başlık loglanabilir.** Caddy log filtresi yalnız `Authorization`'ı siliyor; `X-Yonetim-Oturum` loglanabilir.
   Tek başına yetki vermediği için kabul edildi; örnek Caddyfile'a silme satırı eklendi.

## Açık kalanlar

- Yönetim panelinin tasarımı yeniden ele alınacak (işlev çalışıyor, görünüm beğenilmedi).
- Göç öncesi yedek 27.10'da silinecek (içinde token özetleri var).
