# Okunmamış konuşmalar

Branch: `feature/unread-chat-indicators`. Diğer feature branch'lerinden bağımsızdır.

Sol konuşma listesinin en solunda, okunmamış mesajı olan konuşmalar yeşil bir
noktayla gösterilir. Renk mevcut temanın yeşil rengidir; açık ve koyu temada uyum
sağlar. Ekran okuyucu düğmeyi “Okunmamış mesajlar” olarak da tanımlar. Diğer odada
okunmamış konuşma varsa oda seçeneğinin başına da yeşil nokta eklenir.

- Bir konuşmayı açınca yalnız ekranda yüklenmiş mesajların son kimliği okundu kaydedilir.
- Odaya girişteki “Tüm konuşmalar” görünümü bütün konuşmaları otomatik okumaz.
- Seçili konuşma görünürken gelen yeni mesajlar okundu olur.
- Arka plan sekmesinde veya yönetim ekranındayken gelenler okunmamış kalır.
- Kendi gönderdiğin mesajlar okunmamış işareti oluşturmaz.
- Bir hesabın okuması diğer hesabın işaretlerini değiştirmez. Durum sunucudadır;
  çıkış/girişte ve başka cihazda korunur. CLI inbox imleciyle bağlantılı değildir.
- Başka bir sekmedeki eski okuma isteği veya gecikmiş ağ yanıtı okunmuş mesajları
  tekrar okunmamışa çeviremez. Ağ hatasında işaret korunur.

Özetler on saniyede bir yenilenir; açık odanın mevcut konuşmasına gelen mesajlar
canlı akışta işlenir. Son 100 oda mesajında bulunmayan konuşmalar da özet listesinde
yer alır. Açılınca konuşmanın son 100 mesajı getirilir; önceki sayfalar ayrıca açılabilir.

Sunucu bu dalda **şema v5**'e geçer ve `(hesap, oda, konuşma)` okuma imleci ekler.
Mevcut mesajlar silinmez. Eski mesajlar ilk açılana kadar okunmamış görünebilir.
Kurulumdan önce veritabanı yedeği alınır; eski v4 ikilisi v5 veritabanını açmaz.
Geri alma gerekiyorsa v4 yedeği geri yüklenir. Merge ve canlı kurulum sonrasında
bu yükseltme gereksinimi Admin Notifications'ta yöneticilere bildirilir.

API: insan hesapları için `GET /v1/unread`, oda üyeleri için
`POST /v1/rooms/{room}/threads/{thread}/read` (`last_id`) ve insanlara ait sayfalı
konuşma günlüğü. Kimlik, üyelik ve gerçek mesaj kimliği sunucuda doğrulanır;
başka odadaki veya gelecekteki bir kimlik göndererek mesajlar okundu yapılamaz.
