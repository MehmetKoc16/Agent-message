# Faz 0 sonucu: uyandırma deneyi ✅ BAŞARILI

Tarih: 13.10.2026

## Soru

Boştaki bir Claude Code VS Code paneli, arka planda başlattığı `relay wait` komutu mesaj gelince
çıktığında kendiliğinden uyanıyor mu?

## Düzen

- Sahte relay: `mock_relay.py`, 127.0.0.1:8799, bellekte.
- İki Claude Code VS Code paneli, aynı makinede:
  - `mehmet-a`: konuşmayı başlatan oturum
  - `mehmet-b`: ikinci oturum
- İki oturum test boyunca birbirine **yalnız relay üzerinden** yazdı. Claude Code'un kendi
  oturumlar arası mesajlaşması da oturumu uyandırdığı için o kanal yalnız kurulum talimatı ve
  bitiş raporu için kullanıldı.
- Her iki taraf da `relay wait`'i Bash aracında `run_in_background: true` ile başlatıp turunu bitirdi.

## Sonuç

| Ölçüt | Sonuç |
|---|---|
| 5 mesajlık konuşma (soru → cevap → soru → cevap → kapat) | ✅ Tamamlandı |
| Kullanıcı araya girdi mi? | Hayır |
| Uyandıran | Her seferinde arka plan komutunun bitiş bildirimi (iki taraf da doğruladı) |
| İzin onayı istendi mi? | mehmet-b tarafında hayır |
| Zaman aşımı / hata | Yok. Sunucu logunda 5 × `POST 201`, 5 long-poll `GET 200` |
| `kapat` sonrası mesaj | `409`, CLI "konuşma kilitli" basıp 1 koduyla çıktı ✅ |
| Her taraf beklemeyi yeniden kurdu mu? | Evet. `kapat` alan taraf kurmadı (doğru davranış) |

Ajanların konuşma içeriği gerçek bir açık soruydu ve bir öneriyle bitti: **mesajlar 14 gün
saklansın, 10. günde uyarı verilsin, okunmadan silinen mesaj için göndericiye "teslim edilmedi"
bildirimi gitsin** (yol haritası S4). Bu ajanların önerisi; karar kullanıcının.

## Öğrenilenler

1. **Yöntem A çalışıyor.** VS Code panelinde ek bayrak, Channels ya da izin atlatma gerekmiyor.
2. **Ek adım:** Bitiş bildirimi yalnız özet veriyor. Ajan mesajı görmek için çıktı dosyasını
   ayrıca okumak zorunda kalıyor (her mesajda bir araç çağrısı daha). Faz 1'de bunu
   kısaltmanın yolları denenebilir.
3. **İzleme:** Sahte sunucu bir isteği ancak tamamlandıktan sonra logluyor. Bekleyen bir
   long-poll'u görmek için `netstat` gerekti. Faz 1'de bağlı ajanlar `GET /agents`'ten görülebilmeli.

## Kapsam dışı (henüz test edilmedi)

- Farklı makineler, HTTPS ve gerçek sunucu (Faz 4–5)
- Codex ve agy köprüsü (Faz 3)
- Uzun boşta kalma: bekleme saatlerce sürerse, ya da bilgisayar uykuya geçerse ne olur?
- Ajan meşgulken (başka bir iş yaparken) mesaj gelirse ne olur?
