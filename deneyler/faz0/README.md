# Faz 0 deneyi: sahte relay + `relay` CLI

> **Uyarı: bu yalnız lokal bir deney, gerçek relay değil.** Mesajlar bellekte durur, sunucu
> kapanınca kaybolur. Yalnız `127.0.0.1`'e bağlanır, şifreleme yok, tokenlar sahte. Gerçek
> sunucu Faz 1'de yazılacak.

## Ne işe yarar

[docs/mimari.md](../../docs/mimari.md) yöntem A'yı denemek içindir: Claude Code `relay wait`'i
arka planda çalıştırır, mesaj gelince komut çıkar ve oturum uyanır. Yalnız Python 3.11+ standart
kütüphanesi kullanılır, kurulum gerekmez.

## Başlatma

1. Sunucu (ayrı bir terminalde):

   ```
   python mock_relay.py --port 8799 --agent mehmet-a:tokA --agent mehmet-b:tokB
   ```

   `--max-turns N` konuşma başına tur sınırını değiştirir (varsayılan 20). Durdurmak: Ctrl+C.

2. İki ayar dosyası. `ornek/` altındakileri `config.toml` adıyla **değil**, `mehmet-a.toml`
   gibi kopyalayın (repo `.gitignore`'u `config.toml` ve `state.json`'u zaten yok sayıyor,
   ama bu adlar hiçbirine uymaz):

   ```
   mkdir -p ~/.agent-relay
   cp ornek/mehmet-a.config.example.toml ~/.agent-relay/mehmet-a.toml
   cp ornek/mehmet-b.config.example.toml ~/.agent-relay/mehmet-b.toml
   ```

   Okuma imleci ayar dosyasının yanında `state-<oda>-<ajan>.json` olarak tutulur. Bu dosya
   `.gitignore`'da **yok**; ayarları repo içinde tutarsanız commit'lemeyin.

## Örnek komutlar

```
python relay.py --config ~/.agent-relay/mehmet-a.toml send --type soru --to mehmet-b "Merhaba"
python relay.py --config ~/.agent-relay/mehmet-b.toml wait --timeout 600
python relay.py --config ~/.agent-relay/mehmet-b.toml inbox
echo "uzun metin" | python relay.py --config ~/.agent-relay/mehmet-b.toml send --type cevap --thread T-1 -
```

`wait` çıkış kodları: 0 mesaj geldi, 2 zaman aşımı, 3 sunucuya hiç ulaşılamadı.
`send` hata verirse çıkış kodu 1 (ör. konuşma kilitli).

## Testler

```
cd deneyler/faz0
python -m unittest -v test_faz0
```

Testler sunucuyu kendi içinde başlatıp kapatır; arkada süreç kalmaz.
