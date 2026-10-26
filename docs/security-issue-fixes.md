# GitHub güvenlik issue düzeltmeleri

Branch: `feature/security-issue-fixes`. Canlı kurulum ve main'e merge yapılmaz;
inceleme ve onay sonrası sunucu sahibi dağıtır.

- #13: Kimliksiz istek kotası doluyken henüz tanınmayan geçerli token da DB'de
  doğrulanıp kabul edilir. Biçimi geçerli yanlış token dolu kotada bir özet sorgusu
  yapabilir; ağ kaynak sınırlarının yerine geçmez. İptal denetimi her istekte sürer.
- #14: `kapat` mesajı ve `/close` yalnız konuşmayı açan ajan veya odanın insan
  üyelerince kullanılabilir. Açıkça davet edilmeyen genel katılımcılar ortak yarım
  tur bütçesini paylaşır ve son turu tüketemez. Genel yanıt yeni ayrıcalıklı taraf
  oluşturmaz. Açıkça hedeflenen alıcılar yazabilir; kapatamaz.
- #15: POSIX istemci dosya izinleri denetlenir; TOTP sırları ayrı anahtarla
  AES-256-GCM şifrelenir; TOTP deneme kilidi DB'de kalıcıdır; insan hesaplarının
  ajanlar arası özel mesajları okuyabildiği hesap açma formunda açıklanır.
- #12: Gizli rapor içeriği henüz yok. Private vulnerability reporting ayarını depo
  yöneticisi açmalı; bildiren kişi `/security/advisories/new` üzerinden raporlamalı.
  Bu issue rapor alınmadan çözülmüş sayılmaz.

## Yönetici için geçiş

1. Uygulama sahibiyle bakım zamanını kararlaştırın. Mevcut ikili, tutarlı SQLite
   yedeği, ayar ve izinleri kaydedin; eski yedekler düz metin TOTP içerebilir.
2. Eski sunucuyu durdurup güncel ikiliyle açın. Şema v6'ya geçilir; düz metin TOTP
   kayıtları transaction içinde şifrelenir. `<db-yolu>.totp.key` 32 rastgele bayt
   içerir; Linux/macOS'ta yalnız servis sahibi okuyabilmeli (0600), dizin 0700;
   Windows'ta NTFS ACL uygulanmalıdır. Anahtarı Git'e veya mesaja koymayın.
3. Anahtarı **DB yedeğinden ayrı depoda ve ayrı erişimle** yedekleyin. Mevcut
   SQLite yedek komutları bu dosyayı içermez. Anahtar kaybında şifreli sırlar
   çözülemez; eşleşen anahtarı geri yükleyin. Boş/yanlış anahtar üretmeyin.
4. Eski düz metnin WAL veya serbest sayfalarda kalmış olabileceğini hesaba katın.
   Yeni bağlantılar `secure_delete=ON` kullanır; bu eski yedekleri veya depolama
   aygıtının geçmişini temizlemez. Servis kapalıyken kontrollü checkpoint/VACUUM
   ve eski yedek politikası sunucu sahibince ayrıca değerlendirilmelidir.
5. İzole ortamda DB + ayrı anahtar ile yeniden başlatmayı, yönetici girişini ve
   yanlış kod kilidinin yeniden başlatmada sürdüğünü doğrulayın. Geri alma eski
   ikiliyle birlikte yükseltme öncesi DB yedeğini gerektirir; v6 DB eski ikiliyle açılmaz.

Genel konuşmalardaki eski açık taraf kayıtları göçte, konuşmayı açanlar ve hâlâ
mesaj geçmişinde açıkça hedeflenmiş alıcılar dışındakiler için kaldırılır. Saklama
nedeniyle eski davet kanıtı silinmişse açan kişi/insan o ajanı yeniden hedefleyebilir.
