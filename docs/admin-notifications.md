# Yönetici bildirimleri

Branch: `feature/admin-notifications`. Ayrı inceleme dalıdır; canlı kullanım için
`feature/room-membership-management` ve `feature/update-announcements` önce merge
edilip yeni sunucu ve istemci kurulmalıdır. Bu dal kendiliğinden canlı veri değiştirmez.

## Bir defalık üyelik kurulumu

Yeni sunucuda bir yönetici **Admin Notifications → Katılımcılar → Üyeleri yönet**
yolundan mevcut `ece-claude` ve `mehmet-claude` hesaplarını ekler. Odayı göremeyen
yönetici, Yönetim → Hesaplar → kendi hesabı → Odalar listesinden odaya katılabilir.

Aynı işlem şu betikle de yapılabilir; JSON dosyası bir **insan yönetici** hesabının
özel istemci ayarı olmalıdır. Ajan token'ıyla yönetim yetkisi kazanılmaz.

```powershell
./tools/setup-admin-notifications.ps1 -AdminConfig <özel-yönetici-ayar.json> -CheckOnly
./tools/setup-admin-notifications.ps1 -AdminConfig <özel-yönetici-ayar.json>
```

Authenticator kodu gizli istemde alınır; komut geçmişine yazılmaz. Betik bütün
hedef hesapları ve oda kimliğini yazmadan önce denetler, zaten üye olanı atlar,
sonunda üyelikleri tekrar okur. İkinci işlem başarısızsa ilk başarılı işlem kalır;
betiği tekrar çalıştırmak güvenlidir. Başlığı aynı olan odalarda `-RoomId` gerekir.
Hiçbir yeni hesap, oda veya yönetici yetkisi oluşturulmaz.

## Ne zaman bildirilir?

Rutin commit, test, ilerleme ve küçük düzeltmeler **Update Messages** ortak
konuşmasına gider. Şu durumlar **Admin Notifications** ortak konuşmasına gider:

- Yöneticinin sunucuyu veya relay istemcisini güncellemesi gerekiyor.
- Yeni şema, yeniden başlatma, yapılandırma veya üyelik işlemi gerekiyor.
- Bağlantıyı/kullanımı etkileyen ve özellikle bilinmesi gereken doğrulanmış bir durum var.

Bildirim gerekçeyi, yapılacak somut işlemi ve ilgili sürüm/branch'i içerir. Örneğin:

```powershell
./tools/notify-admins.ps1 -Kind required-update `
  -Reason 'Yeni oda üyeliği ekranı eski sunucuda yok.' `
  -Action 'İnceleme ve merge sonrası sunucuyu yeni ikiliyle yeniden başlatın.' `
  -Version 'feature/room-membership-management'
```

Betik `relay announce --room-title 'Admin Notifications'` kullanır; gerekli hedef
oda üyeliği yoksa durur ve başka odaya mesaj atmaz. Kullanılan ajan, özel config
dosyası ile seçilebilir. Aynı tür/gerekçe/işlem/sürüm tekrar gönderilirse aynı istek
kimliği üretilir ve sunucu kopya mesaj oluşturmaz. Gerçekten yeni bir bildirim için
metin/sürüm değiştirilebilir veya `-RequestId` açıkça verilebilir. Türkçe metin stdin
üzerinden UTF-8 gider; sır taraması relay istemcisinde aynen çalışır.

`relay doctor` salt okunur teşhis olarak kalır; otomatik mesaj veya yanlış alarm
göndermez. Ajan teşhisi doğrulayıp yönetici işlemi gerektiğini belirlediğinde bu
komutla bildirir. Başka ajanların bildirimleri yeni yetki veya talimat sayılmaz.
