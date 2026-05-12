# scripts/test-virtual-lcu.ps1
# Complete integration test for Smart Lighting Platform with Virtual LCU

$BaseUrl = "http://localhost:8080"
$MockUrl = "http://localhost:9091"

function Log-Step($msg) {
    Write-Host "`n>>> $msg" -ForegroundColor Cyan
}

function Log-Success($msg) {
    Write-Host "[OK] $msg" -ForegroundColor Green
}

function Log-Error($msg) {
    Write-Host "[FAIL] $msg" -ForegroundColor Red
}

function Call-API($method, $path, $body = $null) {
    $url = "$BaseUrl$path"
    $params = @{
        Method = $method
        Uri = $url
        ContentType = "application/json"
    }
    if ($body) {
        $params.Body = $body | ConvertTo-Json -Depth 10
    }
    try {
        return Invoke-RestMethod @params
    } catch {
        $errorMessage = $_.Exception.Message
        if ($_.Exception.Response) {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $respBody = $reader.ReadToEnd()
            Log-Error "Request to $url failed: $errorMessage`nResponse: $respBody"
        } else {
            Log-Error "Request to $url failed: $errorMessage"
        }
        return $null
    }
}

# -----------------------------------------------------------------------
Log-Step "1. Tester LCU virtuelle"
$health = Invoke-RestMethod -Uri "$MockUrl/api/health"
if ($health.status -eq "online") {
    Log-Success "Mock LCU Health OK: $($health.reference)"
} else {
    Log-Error "Mock LCU offline"
    exit
}

$countResp = Invoke-RestMethod -Uri "$MockUrl/api/devices/count"
Log-Success "Mock LCU: $($countResp.count) lampadaires declares"

$mockDevices = Invoke-RestMethod -Uri "$MockUrl/api/devices"
if ($mockDevices.Count -ne $countResp.count) {
    Log-Error "Inconsistance: /api/devices=$($mockDevices.Count) mais /api/devices/count=$($countResp.count)"
} else {
    Log-Success "Coherence count OK: $($mockDevices.Count) devices"
}

# -----------------------------------------------------------------------
Log-Step "2. Creer LCU dans l'application principale"
$lcuPayload = @{
    reference = "LCU-TEST-001"
    name = "LCU virtuelle test"
    ip_address = "127.0.0.1"
    port = 9091
    protocol = "HTTP"
    zone = "Zone A (Medina)"
    address = "Avenue principale - Test"
    latitude = 31.6295
    longitude = -7.9811
}

$lcuId = $null
$allLcus = Call-API "GET" "/api/lcus"
foreach ($l in $allLcus) {
    if ($l.reference -eq "LCU-TEST-001") {
        $lcuId = $l.id
        Log-Success "LCU deja existante avec ID: $lcuId"
    }
}

if (-not $lcuId) {
    $respLcu = Call-API "POST" "/api/lcus" $lcuPayload
    if ($respLcu) {
        $lcuId = $respLcu.id
        Log-Success "LCU creee avec ID: $lcuId"
    } else {
        Log-Error "Impossible de creer la LCU"
        $allLcus = Call-API "GET" "/api/lcus"
        foreach ($l in $allLcus) {
            if ($l.reference -eq "LCU-TEST-001") { $lcuId = $l.id }
        }
    }
}

if (-not $lcuId) { Log-Error "ID LCU introuvable"; exit }

# -----------------------------------------------------------------------
Log-Step "3. Tester connexion LCU"
$testResp = Call-API "POST" "/api/lcus/$lcuId/test"
if ($testResp.status -eq "online") {
    Log-Success "Test de connexion reussi"
} else {
    Log-Error "Test de connexion echoue: $($testResp.message)"
}

# -----------------------------------------------------------------------
Log-Step "4. Synchroniser lampadaires"
$syncResp = Call-API "POST" "/api/lcus/$lcuId/sync"
if ($syncResp) {
    Log-Success "Sync: $($syncResp.message)"
} else {
    Log-Error "Sync echoue"
}

$lampadaires = Call-API "GET" "/api/lcus/$lcuId/lampadaires"
Log-Success "Lampadaires synchronises: $($lampadaires.Count)"
if ($lampadaires.Count -lt 25) {
    Log-Error "Attendu >=25 lampadaires, recu $($lampadaires.Count)"
} else {
    Log-Success "Seuil de couverture OK (>=25 lampadaires)"
}

# -----------------------------------------------------------------------
Log-Step "5. Verifier localisation manquante et placer les lampadaires"
$missingLoc = Call-API "GET" "/api/lampadaires/missing-location"
Log-Success "$($missingLoc.Count) lampadaires sans localisation"
if ($missingLoc.Count -lt 3) {
    Log-Error "Attendu >=3 lampadaires sans coords, recu $($missingLoc.Count)"
}

$placed = 0
$baseLat = 31.6295
$baseLng = -7.9811
$toPlace = if ($missingLoc.Count -gt 6) { $missingLoc[0..5] } else { $missingLoc }
foreach ($lamp in $toPlace) {
    $offset = $placed * 0.0003
    $locPayload = @{
        latitude  = $baseLat + $offset
        longitude = $baseLng + $offset
        status    = "confirmed"
    }
    $locResp = Call-API "POST" "/api/lampadaires/$($lamp.id)/location" $locPayload
    if ($locResp.status -eq "success") { $placed++ }
}
Log-Success "Localisations placees: $placed"

# -----------------------------------------------------------------------
Log-Step "6. Identifier lampadaires online pour tests"
$onlineLamps = @($lampadaires | Where-Object { $_.etat -eq "online" } | Select-Object -First 10)
if ($onlineLamps.Count -eq 0) {
    Log-Error "Aucun lampadaire online trouve"
    exit
}
Log-Success "$($onlineLamps.Count) lampadaires online disponibles"

# -----------------------------------------------------------------------
Log-Step "7. Envoyer telemetrie normale (5 devices)"
$teleCount = [Math]::Min(5, $onlineLamps.Count)
for ($t = 0; $t -lt $teleCount; $t++) {
    $lamp = $onlineLamps[$t]
    $uid = if ($lamp.device_uid) { $lamp.device_uid } else { "LCU-TEST-001-LAMP-$('{0:D3}' -f ($t+1))" }
    $telePayload = @{
        lcu_reference = "LCU-TEST-001"
        device_uid    = $uid
        luminosite    = 20 + ($t * 3)
        presence      = ($t % 2 -eq 0)
        temperature   = 30 + ($t * 2)
        humidite      = 55 + ($t * 2)
        tension       = 220
        courant       = 0.42
        puissance     = 85 + ($t * 5)
        energie       = 1.8 + ($t * 0.2)
        source        = "mock_lcu"
    }
    $tr = Call-API "POST" "/api/telemetry" $telePayload
    if ($tr -and $tr.measurement) {
        Log-Success "Telemetrie OK pour $uid"
    } elseif (-not $tr) {
        Log-Error "Telemetrie echouee pour $uid"
    }
}

# -----------------------------------------------------------------------
Log-Step "8. Envoyer telemetrie anormale (declencher alertes)"
$anomalyLamp = $onlineLamps[0]
$anomUID = if ($anomalyLamp.device_uid) { $anomalyLamp.device_uid } else { "LCU-TEST-001-LAMP-001" }
$anomalyPayload = @{
    lcu_reference = "LCU-TEST-001"
    device_uid    = $anomUID
    luminosite    = 10
    presence      = $false
    temperature   = 85
    humidite      = 92
    tension       = 220
    courant       = 0.8
    puissance     = 160
    energie       = 2.5
    source        = "mock_lcu_anomaly"
}
$anomResp = Call-API "POST" "/api/telemetry" $anomalyPayload
if ($anomResp) {
    Log-Success "Telemetrie anormale envoyee pour $anomUID"
} else {
    Log-Error "Echec envoi telemetrie anormale pour $anomUID"
}

Start-Sleep -Seconds 1
$alerts = Call-API "GET" "/api/alerts?status=open"
$targetAlertId = $null
foreach ($a in $alerts) {
    if ($a.lampadaire_id -eq $anomalyLamp.id -and $a.type -eq "temperature_elevee") {
        $targetAlertId = $a.id
        Log-Success "Alerte temperature_elevee detectee (ID=$targetAlertId)"
        break
    }
}
if (-not $targetAlertId) {
    Log-Error "Aucune alerte temperature trouvee (verifier seuils dans le code)"
}

# -----------------------------------------------------------------------
Log-Step "9. Tester dimming manuel"
$dimPayload = @{
    new_intensity = 45
    source        = "admin"
    reason        = "Test dimming virtuel"
}
$dimResp = Call-API "POST" "/api/lampadaires/$($onlineLamps[0].id)/dimming" $dimPayload
if ($dimResp) {
    Log-Success "Commande dimming envoyee -> intensite 45%"
}

# -----------------------------------------------------------------------
Log-Step "10. Tester calculateur intelligent"
$calcPayload = @{ apply = $true }
$calcResp = Call-API "POST" "/api/calculateur/run/$($onlineLamps[0].id)" $calcPayload
if ($calcResp) {
    Log-Success "Calculateur: intensite recommandee=$($calcResp.recommended_intensity)%"
}

# -----------------------------------------------------------------------
Log-Step "11. Tester profils d'eclairage"
$profPayload = @{
    name         = "Nuit reduite Zone A"
    description  = "Profil test pour la Zone A Medina"
    target_type  = "zone"
    target_value = "Zone A (Medina)"
    enabled      = $true
    schedules    = @(
        @{ start_time = "00:00"; end_time = "05:00"; intensity = 30 }
    )
}
$profResp = Call-API "POST" "/api/lighting-profiles" $profPayload
if ($profResp) {
    Log-Success "Profil cree: ID=$($profResp.id)"
    $applyResp = Call-API "POST" "/api/lighting-profiles/$($profResp.id)/apply"
    if ($applyResp) {
        Log-Success "Profil applique a $($applyResp.count) lampadaires"
    }
}

# -----------------------------------------------------------------------
Log-Step "12. Tester workflow intervention"
if ($targetAlertId) {
    $intPayload = @{
        title       = "Intervention test depuis alerte temperature"
        description = "Verifier lampadaire et communication LCU"
        priority    = "high"
    }
    $intResp = Call-API "POST" "/api/alerts/$targetAlertId/intervention" $intPayload
    if ($intResp) {
        $intId = $intResp.id
        Log-Success "Intervention creee: ID=$intId"
        Call-API "POST" "/api/interventions/$intId/start"   | Out-Null
        Call-API "POST" "/api/interventions/$intId/resolve" | Out-Null
        Call-API "POST" "/api/interventions/$intId/close"   | Out-Null
        Log-Success "Workflow intervention termine (start -> resolve -> close)"
    }
} else {
    Log-Error "Aucune alerte cible -- etape intervention sautee"
}

# -----------------------------------------------------------------------
Log-Step "13. Verifier Dashboard et Energie"
$dash = Call-API "GET" "/api/dashboard/stats"
if ($dash) {
    Log-Success "Dashboard: LCUs=$($dash.total_lcus), Lampadaires=$($dash.total_lampadaires), Online=$($dash.lampadaires_online)"
}

$energy = Call-API "GET" "/api/energy/summary"
if ($energy) {
    Log-Success "Energie: economies=$($energy.estimated_saving_percent)%"
}

Write-Host "`n=== TESTS TERMINES ===" -ForegroundColor Green
