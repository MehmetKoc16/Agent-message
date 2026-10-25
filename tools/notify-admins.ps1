[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateSet('required-update', 'action-required', 'important')][string]$Kind,
    [Parameter(Mandatory)][string]$Reason,
    [Parameter(Mandatory)][string]$Action,
    [Parameter(Mandatory)][string]$Version,
    [string]$Config = (Join-Path $env:USERPROFILE '.agent-relay/config.json'),
    [string]$RelayPath = 'relay',
    [string]$RoomId,
    [string]$RequestId
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'relay-notifications.psm1') -Force
Send-AdminNotification @PSBoundParameters
