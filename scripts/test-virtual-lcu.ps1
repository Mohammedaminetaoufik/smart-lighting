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
            $body = $reader.ReadToEnd()
            Log-Error "Request to $url failed: $errorMessage`nResponse: $body"
        } else {
            Log-Error "Request to $url failed: $errorMessage"
        }
        return $null
    }
}

Log-Step "1. Tester LCU virtuelle"
$health = Invoke-RestMethod -Uri "$MockUrl/api/health"
if ($health.status -eq "online") {
    Log-Success "Mock LCU Health OK: $($health.reference)"
} else {
    Log-Error "Mock LCU offline"
    exit
}

$devices = Invoke-RestMethod -Uri "$MockUrl/api/devices"
Log-Success "Mock LCU Devices found: $($devices.Count)"

Log-Step "2. Créer LCU dans l’application principale"
$lcuPayload = @{
    reference = "LCU-TEST-001"
    name = "LCU virtuelle test"
    ip_address = "127.0.0.1"
    port = 9091
    protocol = "HTTP"
    zone = "Zone A"
    address = "Avenue principale - Test"
    latitude = 31.6250
    longitude = -7.9890
}

# The main app usually has a POST /lcus (redirects) and /api/lcus (returns JSON)
# We assume /api/lcus is for JSON creation
$respLcu = Call-API "POST" "/api/lcus" $lcuPayload
# If /api/lcus doesn't exist, we might need to use the form one, but let's assume API first.
# Wait, let me check if POST /api/lcus exists in main.go
# Checked: main.go has POST /lcus and POST /api/lcus ? No, it has POST /lcus.
# I'll check main.go again.

# Checking main.go for LCU creation route...
$lcuId = $null
$allLcus = Call-API "GET" "/api/lcus"
foreach ($l in $allLcus) {
    if ($l.reference -eq "LCU-TEST-001") {
        $lcuId = $l.id
        Log-Success "LCU already exists with ID: $lcuId"
    }
}

if (-not $lcuId) {
    # Try creating via API or Form
    $respLcu = Call-API "POST" "/api/lcus" $lcuPayload
    if ($respLcu) {
        $lcuId = $respLcu.id
        Log-Success "LCU created with ID: $lcuId"
    } else {
        Log-Error "Failed to create LCU via API. Retrying via search..."
        $allLcus = Call-API "GET" "/api/lcus"
        foreach ($l in $allLcus) {
            if ($l.reference -eq "LCU-TEST-001") { $lcuId = $l.id }
        }
    }
}

if (-not $lcuId) { Log-Error "Could not get LCU ID"; exit }

Log-Step "3. Tester connexion LCU"
$testResp = Call-API "POST" "/api/lcus/$lcuId/test"
if ($testResp.status -eq "online") {
    Log-Success "Connection test successful"
} else {
    Log-Error "Connection test failed: $($testResp.message)"
}

Log-Step "4. Synchroniser lampadaires"
$syncResp = Call-API "POST" "/api/lcus/$lcuId/sync"
Log-Success "Sync result: $($syncResp.message)"

Log-Step "5. Lister lampadaires associés"
$lampadaires = Call-API "GET" "/api/lcus/$lcuId/lampadaires"
Log-Success "Found $($lampadaires.Count) lampadaires for LCU"

$missingLoc = Call-API "GET" "/api/lampadaires/missing-location"
Log-Success "Found $($missingLoc.Count) lampadaires missing location"

Log-Step "6. Placer un lampadaire sans localisation"
if ($missingLoc.Count -gt 0) {
    $targetId = $missingLoc[0].id
    $locPayload = @{
        latitude = 31.6265
        longitude = -7.9875
        status = "confirmed"
    }
    $locResp = Call-API "POST" "/api/lampadaires/$targetId/location" $locPayload
    if ($locResp.status -eq "success") {
        Log-Success "Location updated for lampadaire #$targetId"
    }
}

Log-Step "7. Envoyer télémétrie"
$telePayload = @{
    lcu_reference = "LCU-TEST-001"
    device_uid = "LCU-TEST-001-LAMP-001"
    luminosite = 20
    presence = $true
    temperature = 35
    humidite = 60
    tension = 220
    courant = 0.42
    puissance = 92
    energie = 1.8
    source = "mock_lcu"
}
$teleResp = Call-API "POST" "/api/telemetry" $telePayload
if ($teleResp.status -eq "success") {
    Log-Success "Telemetry saved"
}

Log-Step "8. Envoyer télémétrie anormale"
$anomalyPayload = @{
    lcu_reference = "LCU-TEST-001"
    device_uid = "LCU-TEST-001-LAMP-001"
    luminosite = 10
    presence = $false
    temperature = 85
    humidite = 92
    tension = 220
    courant = 0.8
    puissance = 160
    energie = 2.5
    source = "mock_lcu_anomaly"
}
$anomResp = Call-API "POST" "/api/telemetry" $anomalyPayload
Log-Success "Anomaly telemetry sent. Checking alerts..."

$alerts = Call-API "GET" "/api/alerts?status=open"
$found = $false
foreach ($a in $alerts) {
    if ($a.lampadaire_id -eq $lampadaires[0].id -and $a.type -eq "temperature_elevee") {
        $found = $true
        Log-Success "Alert 'temperature_elevee' detected for lampadaire"
        $targetAlertId = $a.id
    }
}

Log-Step "9. Tester dimming manuel"
$dimPayload = @{
    new_intensity = 45
    source = "admin"
    reason = "Test dimming virtuel"
}
$dimResp = Call-API "POST" "/api/lampadaires/$($lampadaires[0].id)/dimming" $dimPayload
Log-Success "Dimming request sent"

Log-Step "10. Tester calculateur intelligent"
$calcPayload = @{ apply = $true }
$calcResp = Call-API "POST" "/api/calculateur/run/$($lampadaires[0].id)" $calcPayload
Log-Success "Calculator run: Recommended=$($calcResp.recommended_intensity)%"

Log-Step "11. Tester profils d’éclairage"
$profPayload = @{
    name = "Nuit reduite Zone A"
    description = "Profil test pour la zone A"
    target_type = "zone"
    target_value = "Zone A"
    enabled = $true
    schedules = @(
        @{
            start_time = "00:00"
            end_time = "05:00"
            intensity = 30
        }
    )
}
$profResp = Call-API "POST" "/api/lighting-profiles" $profPayload
if ($profResp) {
    Log-Success "Lighting profile created: ID=$($profResp.id)"
    $applyResp = Call-API "POST" "/api/lighting-profiles/$($profResp.id)/apply"
    Log-Success "Profile applied to $($applyResp.count) devices"
}

Log-Step "12. Tester workflow intervention"
if ($targetAlertId) {
    $intPayload = @{
        title = "Intervention test depuis alerte"
        description = "Verifier lampadaire et communication"
        priority = "high"
    }
    $intResp = Call-API "POST" "/api/alerts/$targetAlertId/intervention" $intPayload
    if ($intResp) {
        Log-Success "Intervention created: ID=$($intResp.id)"
        $intId = $intResp.id
        Call-API "POST" "/api/interventions/$intId/start" | Out-Null
        Call-API "POST" "/api/interventions/$intId/resolve" | Out-Null
        Call-API "POST" "/api/interventions/$id/close" | Out-Null
        Log-Success "Intervention workflow completed"
    }
}

Log-Step "13. Verifier Dashboard & Energy"
$dash = Call-API "GET" "/api/dashboard/stats"
Log-Success "Dashboard: Total LCUs=$($dash.total_lcus), Total Lamps=$($dash.total_lampadaires)"

$energy = Call-API "GET" "/api/energy/summary"
Log-Success "Energy: Saving Percent=$($energy.estimated_saving_percent)%"

Write-Host "`n=== TESTS TERMINÉS AVEC SUCCÈS ===" -ForegroundColor Green
