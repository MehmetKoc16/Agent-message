[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$AdminConfig,
    [System.Security.SecureString]$ManagementCode,
    [string]$RoomId,
    [switch]$CheckOnly
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'relay-notifications.psm1') -Force
Initialize-AdminNotificationRoom @PSBoundParameters
