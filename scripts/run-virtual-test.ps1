# scripts/run-virtual-test.ps1
# Helper script to launch the virtual LCU and run the test suite

Write-Host ">>> Nettoyage du port 9091..." -ForegroundColor Cyan
$portProcess = Get-NetTCPConnection -LocalPort 9091 -ErrorAction SilentlyContinue
if ($portProcess) {
    Stop-Process -Id $portProcess.OwningProcess -Force
    Start-Sleep -Seconds 1
}

Write-Host ">>> Demarrage de la LCU virtuelle..." -ForegroundColor Cyan
Start-Process go -ArgumentList "run", "./tools/mock-lcu/main.go" -NoNewWindow -PassThru

Write-Host ">>> Attente de 3 secondes pour l'initialisation..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

Write-Host ">>> Demarrage des tests..." -ForegroundColor Cyan
./scripts/test-virtual-lcu.ps1

Write-Host "`n>>> Note: L'application principale (go run .) doit etre lancee dans un autre terminal sur http://localhost:8080" -ForegroundColor Yellow
Write-Host ">>> La LCU virtuelle tourne toujours en background (port 9091)." -ForegroundColor Yellow
