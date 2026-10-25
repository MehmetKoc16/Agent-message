Set-StrictMode -Version Latest

function Send-AdminNotification {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][ValidateSet('required-update', 'action-required', 'important')][string]$Kind,
        [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$Reason,
        [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$Action,
        [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$Version,
        [string]$Config = (Join-Path $env:USERPROFILE '.agent-relay/config.json'),
        [string]$RelayPath = 'relay',
        [string]$RoomId,
        [string]$RequestId
    )
    $labels = @{ 'required-update' = 'Güncelleme gerekli'; 'action-required' = 'Yönetici işlemi gerekli'; 'important' = 'Önemli bilgilendirme' }
    $body = $labels[$Kind] + "`nGerekçe: $Reason`nYapılacak: $Action`nİlgili sürüm/dal: $Version"
    if (-not $RequestId) {
        $hash = [System.Security.Cryptography.SHA256]::Create()
        try { $RequestId = 'admin-' + ([BitConverter]::ToString($hash.ComputeHash([Text.Encoding]::UTF8.GetBytes($body))).Replace('-', '').ToLowerInvariant()).Substring(0, 48) }
        finally { $hash.Dispose() }
    }
    $relayArgs = @('--config', $Config, 'announce', '--request-id', $RequestId)
    if ($RoomId) { $relayArgs += @('--room', $RoomId) }
    else { $relayArgs += @('--room-title', 'Admin Notifications') }
    $relayArgs += '-'
    $OutputEncoding = [Text.UTF8Encoding]::new($false)
    $body | & $RelayPath @relayArgs
    if ($LASTEXITCODE -ne 0) { throw 'Yönetici bildirimi gönderilemedi; üyeliği ve relay announce sürümünü denetle. Başka odaya gönderilmedi.' }
}

function Initialize-AdminNotificationRoom {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$AdminConfig,
        [System.Security.SecureString]$ManagementCode,
        [string]$RoomId,
        [switch]$CheckOnly
    )
    $cfg = Get-Content -LiteralPath $AdminConfig -Raw -Encoding UTF8 | ConvertFrom-Json
    $base = ([uri]$cfg.url)
    if (-not $base.IsAbsoluteUri -or $base.UserInfo -or $base.Query -or $base.Fragment -or ($base.Scheme -ne 'https' -and -not ($base.Scheme -eq 'http' -and $base.IsLoopback))) {
        throw 'Ayar adresi HTTPS veya yerel HTTP olmalı; kullanıcı bilgisi, sorgu veya fragment içeremez.'
    }
    $headers = @{Authorization = 'Bearer ' + $cfg.token}
    $root = $cfg.url.TrimEnd('/')
    # Suppress raw HTTP errors: they may contain private hosts or headers.
    function Call-Relay([string]$Path, [string]$Method = 'GET', $Payload) {
        $params = @{Uri = $root + $Path; Headers = $headers; Method = $Method; TimeoutSec = 15; MaximumRedirection = 0; ErrorAction = 'Stop'}
        if ($null -ne $Payload) {
            $params.ContentType = 'application/json; charset=utf-8'
            $params.Body = [Text.Encoding]::UTF8.GetBytes(($Payload | ConvertTo-Json -Compress))
        }
        try { Invoke-RestMethod @params }
        catch { throw 'Relay işlemi tamamlanamadı; yönetici kimliği, TOTP, üyelik ve yeni sunucu sürümünü denetle.' }
    }
    $me = Call-Relay '/v1/me'
    if ($me.kind -ne 'insan' -or -not $me.yonetici) { throw 'Kurulum yalnız yönetici insan hesabıyla yapılabilir.' }
    if (-not $ManagementCode) { $ManagementCode = Read-Host 'Authenticator kodu' -AsSecureString }
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($ManagementCode)
    try { $code = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer); $session = Call-Relay '/v1/yonetim/oturum' 'POST' @{kod = $code} }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer); $code = $null }
    $headers['X-Yonetim-Oturum'] = $session.oturum
    $rooms = (Call-Relay '/v1/yonetim/odalar').rooms
    $matches = @($rooms | Where-Object { if ($RoomId) { $_.name -eq $RoomId } else { $_.title -eq 'Admin Notifications' } })
    if ($matches.Count -ne 1) { throw 'Admin Notifications odası bulunamadı veya başlık belirsiz; doğru oda kimliğini -RoomId ile seç.' }
    $selectedRoom = $matches[0].name
    $accounts = @(Call-Relay '/v1/yonetim/hesaplar')
    $targets = @('ece-claude', 'mehmet-claude')
    foreach ($target in $targets) {
        $account = @($accounts | Where-Object { $_.name -eq $target -and -not $_.revoked -and $_.kind -eq 'ajan' })
        if ($account.Count -ne 1) { throw 'Gerekli ajan hesaplarından biri yok veya aktif değil; hiçbir üyelik değişikliği başlatılmadı.' }
    }
    foreach ($target in $targets) {
        $account = $accounts | Where-Object { $_.name -eq $target }
        if ($selectedRoom -in $account.rooms) { Write-Output "$target zaten üye."; continue }
        if ($CheckOnly) { Write-Output "$target eklenecek."; continue }
        $encoded = [uri]::EscapeDataString($target)
        $null = Call-Relay "/v1/yonetim/hesaplar/$encoded/odalar" 'POST' @{oda = $selectedRoom}
        Write-Output "$target eklendi."
    }
    if (-not $CheckOnly) {
        $verified = @(Call-Relay '/v1/yonetim/hesaplar')
        foreach ($target in $targets) {
            $account = @($verified | Where-Object { $_.name -eq $target -and -not $_.revoked -and $selectedRoom -in $_.rooms })
            if ($account.Count -ne 1) { throw 'Üyelik doğrulanamadı; yeniden çalıştır veya oda üyelerini kontrol et.' }
        }
        Write-Output 'İki ajanın Admin Notifications üyeliği doğrulandı.'
    }
}

Export-ModuleMember -Function Send-AdminNotification, Initialize-AdminNotificationRoom
