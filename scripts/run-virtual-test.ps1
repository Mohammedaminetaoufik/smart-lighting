# scripts/run-virtual-test.ps1
# Launches mock LCU + main app (HTTP mode), runs the full test suite, then cleans up.

$ProjectRoot = Split-Path -Parent $PSScriptRoot

function Kill-Port($port) {
    $conn = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue
    if ($conn) {
        Stop-Process -Id $conn.OwningProcess -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 1
    }
}

function Wait-ForPort($port, $timeoutSec = 30) {
    $deadline = (Get-Date).AddSeconds($timeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $tcp = New-Object System.Net.Sockets.TcpClient
            $tcp.Connect("127.0.0.1", $port)
            $tcp.Close()
            return $true
        } catch { Start-Sleep -Milliseconds 500 }
    }
    return $false
}

# -----------------------------------------------------------------------
Write-Host ">>> Nettoyage des ports 9091 et 8080..." -ForegroundColor Cyan
Kill-Port 9091
Kill-Port 8080

# -----------------------------------------------------------------------
Write-Host ">>> Demarrage de la LCU virtuelle (port 9091)..." -ForegroundColor Cyan
$mockProc = Start-Process go -ArgumentList "run", "$ProjectRoot\tools\mock-lcu\main.go" `
    -NoNewWindow -PassThru -WorkingDirectory $ProjectRoot

if (-not (Wait-ForPort 9091 20)) {
    Write-Host "[FAIL] LCU virtuelle n'a pas demarre dans les 20 secondes" -ForegroundColor Red
    exit 1
}
Write-Host "[OK] LCU virtuelle prete sur http://localhost:9091" -ForegroundColor Green

# -----------------------------------------------------------------------
# Set env var before Start-Process so the child process inherits it
$env:LCU_DISCOVERY_MODE = "http"
Write-Host ">>> Demarrage de l'application principale (LCU_DISCOVERY_MODE=http, port 8080)..." -ForegroundColor Cyan
$mainProc = Start-Process go -ArgumentList "run", "." `
    -NoNewWindow -PassThru -WorkingDirectory $ProjectRoot

if (-not (Wait-ForPort 8080 60)) {
    Write-Host "[FAIL] Application principale n'a pas demarre dans les 60 secondes" -ForegroundColor Red
    $mockProc | Stop-Process -Force -ErrorAction SilentlyContinue
    exit 1
}
Write-Host "[OK] Application principale prete sur http://localhost:8080" -ForegroundColor Green

# -----------------------------------------------------------------------
Write-Host "`n>>> Demarrage des tests..." -ForegroundColor Cyan
& "$PSScriptRoot\test-virtual-lcu.ps1"

# -----------------------------------------------------------------------
Write-Host "`n>>> Nettoyage..." -ForegroundColor Cyan
$mainProc | Stop-Process -Force -ErrorAction SilentlyContinue
Kill-Port 8080
Kill-Port 9091
Write-Host ">>> Termine." -ForegroundColor Green
