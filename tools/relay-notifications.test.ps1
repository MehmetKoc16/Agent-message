$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'relay-notifications.psm1') -Force
function Assert-True($Condition, $Message) { if (-not $Condition) { throw $Message } }
$global:LASTEXITCODE = 0
$global:NotifyCalls = @()
function global:TestRelay {
    $global:NotifyCalls += @{Args = @($args); Body = ($input | Out-String)}
    $global:LASTEXITCODE = 0
}
$notice = @{Kind='required-update';Reason='Türkçe test';Action='Yeni sürümü kur';Version='test-v1';RelayPath='TestRelay';Config='test-only.json'}
Send-AdminNotification @notice
Send-AdminNotification @notice
Assert-True ($global:NotifyCalls.Count -eq 2) 'relay was not called'
Assert-True ('Admin Notifications' -in $global:NotifyCalls[0].Args) 'wrong room'
Assert-True ($global:NotifyCalls[0].Body.Contains('Türkçe test')) 'UTF-8 text changed'
Assert-True (($global:NotifyCalls[0].Args -join '|') -eq ($global:NotifyCalls[1].Args -join '|')) 'repeat changed request identity'

$global:SetupCalls = @()
$global:Members = @{ 'ece-claude' = @(); 'mehmet-claude' = @() }
$global:MissingAgent = $false
$global:DuplicateRoom = $false
function global:Invoke-RestMethod {
    param($Uri, $Headers, $Method, $TimeoutSec, $MaximumRedirection, $ErrorAction, $ContentType, $Body)
    $path = ([uri]$Uri).AbsolutePath
    $global:SetupCalls += "$Method $path"
    switch ($path) {
        '/v1/me' { return @{kind='insan';yonetici=$true} }
        '/v1/yonetim/oturum' { return @{oturum='fake-session'} }
        '/v1/yonetim/odalar' {
            if ($global:DuplicateRoom) { return @{rooms=@(@{name='r1';title='Admin Notifications'}, @{name='r2';title='Admin Notifications'})} }
            return @{rooms=@(@{name='r1';title='Admin Notifications'})}
        }
        '/v1/yonetim/hesaplar' {
            $list = @(@{name='ece-claude';kind='ajan';revoked=$false;rooms=$global:Members['ece-claude']})
            if (-not $global:MissingAgent) { $list += @{name='mehmet-claude';kind='ajan';revoked=$false;rooms=$global:Members['mehmet-claude']} }
            return $list
        }
        default {
            Assert-True ($Headers['X-Yonetim-Oturum'] -eq 'fake-session') 'missing TOTP session'
            $target = ($path -split '/')[4]
            $global:Members[$target] = @('r1')
            return @{ok=$true}
        }
    }
}
$temp = Join-Path ([IO.Path]::GetTempPath()) ('relay-admin-test-' + [guid]::NewGuid().ToString() + '.json')
try {
    @{url='http://127.0.0.1:9876';token='fake-test-only'} | ConvertTo-Json | Set-Content -LiteralPath $temp -Encoding UTF8
    $code = ConvertTo-SecureString '123456' -AsPlainText -Force
    $setup = @{AdminConfig=$temp;ManagementCode=$code}
    $null = Initialize-AdminNotificationRoom @setup -CheckOnly
    Assert-True (@($global:SetupCalls | Where-Object {$_ -match '^POST .*/hesaplar/'}).Count -eq 0) 'check-only wrote membership'
    $null = Initialize-AdminNotificationRoom @setup
    Assert-True (@($global:SetupCalls | Where-Object {$_ -match '^POST .*/hesaplar/'}).Count -eq 2) 'missing membership writes'
    $null = Initialize-AdminNotificationRoom @setup
    Assert-True (@($global:SetupCalls | Where-Object {$_ -match '^POST .*/hesaplar/'}).Count -eq 2) 'second setup duplicated writes'
    $global:MissingAgent = $true
    $failed = $false
    try { $null = Initialize-AdminNotificationRoom @setup } catch { $failed = $true }
    Assert-True $failed 'missing target agent accepted'
    Assert-True (@($global:SetupCalls | Where-Object {$_ -match '^POST .*/hesaplar/'}).Count -eq 2) 'partial write before validation'
    $global:MissingAgent = $false; $global:DuplicateRoom = $true; $failed = $false
    try { $null = Initialize-AdminNotificationRoom @setup } catch { $failed = $true }
    Assert-True $failed 'ambiguous room accepted'
    Write-Output 'PASS: notification routing, repeat identity, TOTP membership, check-only, idempotency, missing agent, ambiguous room.'
} finally {
    Remove-Item -LiteralPath $temp -ErrorAction SilentlyContinue
    Remove-Item Function:/TestRelay -ErrorAction SilentlyContinue
    Remove-Item Function:/Invoke-RestMethod -ErrorAction SilentlyContinue
}
