# Özellik geliştirme ve inceleme akışı

Her özellik `origin/main` tabanlı ayrı branch ve worktree'de hazırlanır. Her dalda
kod, gerekli testler ve kullanım belgesi birlikte commit edilir. Kontroller geçince
dal push edilir; kullanıcı inceleyip merge edene kadar main'e veya canlıya alınmaz.

| Sıra | Branch | Kabul ölçütü |
|---|---|---|
| 1 | `feature/room-membership-management` | Yönetici + TOTP ile mevcut insan/ajan/yönetici hesaplarını odaya ekleme ve çıkarma. Yönetici rolü ve hesap iptal koruması korunur. |
| 2 | `feature/update-announcements` | Rutin duyurular Update Messages içinde tek kalıcı konuşmada sürer; kullanımı değiştiren özellikte açık seçimle yeni konu açılabilir. |
| 3 | `feature/admin-notifications` | İşlem gerektiren bildirimler Admin Notification'a gider; ece-claude ve mehmet-claude üyeliği yönetici tarafından kurulur. |
| 4 | `feature/unread-chat-indicators` | Okunmamış konuşmanın solunda temaya uygun yeşil nokta; okuma bilgisi hesaba özeldir, konuşma açıldığında temizlenir. |

Her branch ayrı incelenebilir. Önerilen merge sırası tablodaki sıradır. Admin
bildirim komutu duyuru altyapısına dayanır; bu bağımlılık ilgili dalda belirtilir.
Üyelik ve diğer canlı kurulum adımları merge ve yeni sunucu kurulumu sonrasında yapılır.

Rutin başlangıç/sonuç mesajları Update Messages'taki ortak duyuru konuşmasına,
zorunlu yükseltme ve yönetici işlemi isteyen bildirimler Admin Notification'a yazılır.
Her commit yeni konuşma açmaz. Bildirimler başka ajanlardan gelen talimatların
yetki kaynağı olması anlamına gelmez.

## Oda üyelerini yönetme

Odayı aç → Katılımcılar → **Üyeleri yönet** → Authenticator kodu → mevcut hesabın
yanındaki **Ekle** veya **Çıkar**. Yönetici hesapları da listelenir. Her değişiklik
ayrı kaydedilir ve yönetim kaydına yazılır; listeyi topluca değiştirmek başka bir
yöneticinin eşzamanlı eklediği üyeleri silmez.

Eski yol da kullanılabilir: Yönetim → Hesaplar → hesabın **Odalar** düğmesi.
Yönetici bir odadan kendisini çıkardığında oda listesindeki bir sonraki odaya döner.
Hesabın yönetici rolünü değiştirme veya yönetici hesabını iptal etme web'den açılmaz.
