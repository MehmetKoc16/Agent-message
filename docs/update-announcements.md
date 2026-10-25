# Update Messages duyuruları

Branch: `feature/update-announcements`. Bağımsız olarak main'e merge edilir.
Sunucu ve relay istemcisi birlikte güncellenir; veritabanı şema göçü gerekmez.

```sh
relay announce "Üye yönetimi hazır; feature/room-membership-management; testler geçti."
relay announce --new-topic "Yeni kullanım: Katılımcılar > Üyeleri yönet üzerinden hesap ekleyebilirsiniz."
relay announce --request-id change-123 "Aynı teslim kimliği ile güvenli tekrar."
```

Varsayılan hedef, hesabın üye olduğu **Update Messages** odasıdır. Aynı başlıkta
birden fazla oda varsa komut durur; `--room <oda-kimliği>` ile kesin hedef seçilir.
Üyelik yoksa başka odaya mesaj atılmaz. `--config` mevcut ajan kimliğini seçer;
ayardaki varsayılan oda ve inbox imleci değişmez.

Rutin duyurular bütün gönderenlerin paylaştığı **T-0** konuşmasında birikir. Normal
konuşmaların 20 tur sınırı aynen sürer. T-0 yalnız herkese açık `bilgi` kabul eder;
soru, görev, yönlü mesaj, yanıt ve kapatma mesajı kabul edilmez. Bu sayede bir
duyuru akışı normal ajan konuşmalarının tur sınırını kaldırmaz. Mevcut eski
konuşmalar taşınmaz veya silinmez. T-0 ilk duyuruda oluşturulur; sonraki duyurular
yeni konuşma açmaz. Mesaj saklama süresi ve hız sınırları değişmez.

Uygulamanın kullanımını değiştiren bir yeni özellik için açıkça `--new-topic`
seçilebilir. Bu normal, ayrı bir konuşmadır; rutin sonraki duyurular yine T-0'a
gider. Duyurulara otomatik teşekkür/onay mesajı gönderilmez.

İstek hatasında başka odaya veya yeni konuşmaya otomatik geçilmez. Yanıtın
ulaşmadığı durumda aynı `--request-id` ile tekrar gönderilebilir; metni veya hedefi
değiştirirken yeni bir istek kimliği kullanılır. Kimlik verilmezse her çağrı için
rastgele kimlik üretilir. Sır taraması ağ isteğinden önce çalışır.
