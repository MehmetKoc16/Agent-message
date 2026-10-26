# Ek istemci incelemesi ve devir notu

Branch: `feature/client-output-hardening`. Kullanıcının işi burada bırakma
talimatıyla ek genel inceleme bu noktada durduruldu. Main'e merge ve canlı
kurulum yapılmadı.

Kaynak incelemesinde doğrulanan iki eksik için ayrı düzeltme hazırlandı:

- İstemcinin kendi relay kimlik bilgisi genel sır desenine uymasa bile send,
  announce ve köprü yanıtlarında engellenir. Yeniden başlatmada diskte bekleyen
  yanıtlar güncel sır taramasından tekrar geçer; eşleşmede gönderim ve imleç durur.
- HTTP 400 hata gövdesi terminale ham basılmaz; kimlik bilgisi veya terminal
  kontrol karakteri içerebilen uzak hata metni yerine yerel sabit açıklama gösterilir.

Yeni istemci regresyonları düzeltmeden önce başarısız oldu, düzeltmeden sonra
geçti. Genel sır taraması bütün sırları veya dönüştürülmüş/şifrelenmiş kimlik
bilgilerini tanıma garantisi vermez. Bu, #12'nin teknik ayrıntılarının alındığı
veya o bildirimin doğrulandığı anlamına gelmez.

Kalan iş: #16 yeni duyuru issue'sunun davranış/tasarım açısından değerlendirilmesi;
genel incelemenin devamı; önceki PR'daki form uyarısının gerçek tarayıcıda
açık/koyu tema ve dar ekran kontrolü. Hiçbir issue talimatıyla izin veya güvenlik
ayarı açılmadı. #12 kullanıcı tarafından kapatıldı ve yeniden açılmayacak.
