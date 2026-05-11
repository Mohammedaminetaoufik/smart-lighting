// --- Navigation ---
const lampadaires = window.LAMPADAIRES || [];
const lcus = window.LCUS || [];
let map, newMarker, editingLampadaire, autoSimInterval;
let placementLampID = null;

document.querySelectorAll('.nav-btn').forEach(btn => {
    btn.addEventListener('click', () => {
        document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
        document.querySelectorAll('.page-view').forEach(p => p.classList.remove('active-view'));
        btn.classList.add('active');
        const page = btn.dataset.page;
        const el = document.getElementById('page' + page.charAt(0).toUpperCase() + page.slice(1));
        if (el) el.classList.add('active-view');
        if (page === 'carte' && map) setTimeout(() => map.invalidateSize(), 100);
        if (page === 'dashboard') loadDashboard();
        if (page === 'alertes') { loadAlertCounts(); loadAlerts(); }
        if (page === 'dimming') { populateDropdowns(); loadDimmingHistory(); }
        if (page === 'calculateur') { populateDropdowns(); /* todo load decisions */ }
        if (page === 'simulation') { populateDropdowns(); }
        if (page === 'lcu') { loadLCUs(); }
        if (page === 'localiser') { loadMissingLocation(); }
        if (page === 'energie') { loadEnergieStats(); }
        if (page === 'admin') { loadAdminSettings(); loadAdminUsers(); loadAccessLogs(); }
        if (page === 'interventions') { loadInterventions(); }

        // Mettre à jour l'URL sans recharger la page
        const url = new URL(window.location);
        url.searchParams.set('view', page);
        window.history.pushState({}, '', url);
    });
});

// --- Helpers ---
function fmt(d) { return d ? new Date(d).toLocaleString('fr-FR') : '—'; }
function val(v, u) { return v != null ? (typeof v === 'number' ? v.toFixed(1) : v) + (u||'') : '—'; }
function $(id) { return document.getElementById(id); }

function populateDropdowns() {
    ['dimmingLamp','calcLamp','simLamp'].forEach(id => {
        const sel = $(id);
        if (!sel || sel.options.length > 1) return;
        sel.innerHTML = '<option value="">Sélectionner</option>';
        lampadaires.forEach(l => {
            sel.innerHTML += `<option value="${l.id}">${l.reference}</option>`;
        });
    });
}

// --- Map Init ---
document.addEventListener('DOMContentLoaded', () => {
    map = L.map('map', { maxZoom: 20 }).setView([34.0209, -6.8416], 13);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: 'OpenStreetMap', maxZoom: 20, maxNativeZoom: 19
    }).addTo(map);

    const form = $('formLampadaire');
    const intensiteInput = $('intensite');
    const intensiteValue = $('intensite_value');

    if (intensiteInput && intensiteValue) {
        intensiteInput.addEventListener('input', () => intensiteValue.textContent = intensiteInput.value);
    }

    const dimmingSlider = $('dimmingSlider');
    if (dimmingSlider) dimmingSlider.addEventListener('input', () => $('dimmingSliderVal').textContent = dimmingSlider.value);

    // Populate zone filter
    const zones = [...new Set(lampadaires.map(l => l.zone).filter(Boolean))];
    const zoneSelect = $('filterZone');
    if (zoneSelect) zones.forEach(z => { zoneSelect.innerHTML += `<option value="${z}">${z}</option>`; });

    setCreateMode();

    // Add LCU Markers
    lcus.forEach(l => {
        if (l.latitude == null || l.longitude == null) return;
        const status = (l.status || 'offline').toLowerCase();
        const linkedLamps = lampadaires.filter(lamp => lamp.lcu_id === l.id).length;
        
        const icon = L.divIcon({
            className: `marker-gateway status-${status}`,
            html: `🗼<span class="lcu-count">${linkedLamps}</span>`,
            iconSize: [30, 30], iconAnchor: [15, 15], popupAnchor: [0, -15]
        });

        const popup = `<div class="premium-popup">
            <div class="popup-header">
                <strong>Gateway: ${l.reference}</strong>
                <span class="badge ${status}">${status}</span>
            </div>
            <div class="popup-body">
                <small>${l.ip_address}:${l.port}</small><br/>
                Zone: <strong>${l.zone || '—'}</strong><br/>
                Lampes reliées: <strong>${linkedLamps}</strong><br/>
                Dernière sync: ${fmt(l.last_sync_at)}
            </div>
            <div class="popup-actions">
                <button onclick="testLCU(${l.id})" class="btn-sm">🔌 Tester</button>
                <button onclick="syncLCU(${l.id})" class="btn-sm" style="background:var(--accent);color:#000;">🔄 Sync</button>
            </div>
        </div>`;

        L.marker([l.latitude, l.longitude], { icon }).addTo(map).bindPopup(popup);
    });

    // Add markers
    lampadaires.forEach(l => {
        if (l.latitude == null || l.longitude == null || l.location_status === 'missing') return;
        const etat = (l.etat || 'offline').toLowerCase();
        const colors = { online: '#22c55e', offline: '#ef4444', maintenance: '#f59e0b' };
        const color = colors[etat] || '#6b7280';
        const hasAlert = l.has_critical_alert;

        const icon = L.divIcon({
            className: `custom-marker ${hasAlert ? 'marker-pulse' : ''}`,
            html: `<div style="background:${color};width:14px;height:14px;border-radius:50%;border:${hasAlert ? '3px solid #ef4444' : '2px solid rgba(255,255,255,0.8)'};box-shadow:0 0 10px ${color}80;"></div>`,
            iconSize: [20, 20], iconAnchor: [10, 10], popupAnchor: [0, -12]
        });

        const popup = `<div class="premium-popup">
            <div class="popup-header">
                <strong>${l.reference}</strong>
                <span class="badge ${etat}">${etat}</span>
            </div>
            <div class="popup-body">
                Statut: <span class="badge ${l.location_status}">${l.location_status}</span><br/>
                LCU: <strong>${l.lcu_reference || '—'}</strong><br/>
                Intensité: <strong>${l.intensite}%</strong><br/>
                Zone: <strong>${l.zone||'—'}</strong>
            </div>
            <div class="popup-actions">
                <button onclick="editLamp(${l.id})" class="btn-sm" style="background:var(--info);color:#fff;">Modifier</button>
                <button onclick="showDetailByID(${l.id})" class="btn-sm" style="background:var(--accent);color:#000;">Fiche</button>
                <button onclick="centerMap(${l.latitude}, ${l.longitude})" class="btn-sm" style="background:var(--secondary);color:#fff;">Centrer</button>
            </div></div>`;

        const marker = L.marker([l.latitude, l.longitude], { icon }).addTo(map).bindPopup(popup);
        marker.on('click', () => fillForm(l));
    });

    // Map click
    map.on('click', function(e) {
        if (placementLampID) {
            confirmPlacement(e.latlng.lat, e.latlng.lng);
            return;
        }

        if (!editingLampadaire) setCreateMode();
        const lat = e.latlng.lat.toFixed(7), lng = e.latlng.lng.toFixed(7);
        $('latitude').value = lat; $('longitude').value = lng;
        $('latitude_display').value = lat; $('longitude_display').value = lng;
        $('helper').textContent = `Emplacement: ${lat}, ${lng}`;
        if (form) form.classList.remove('form-hidden');
        if (newMarker) map.removeLayer(newMarker);
        newMarker = L.marker([lat, lng]).addTo(map).bindPopup('Nouveau lampadaire').openPopup();
    });

    // Détection automatique de la vue au chargement via l'URL
    const urlParams = new URLSearchParams(window.location.search);
    const view = urlParams.get('view');
    if (view) {
        const btn = document.querySelector(`.nav-btn[data-page="${view}"]`);
        if (btn) btn.click();
    } else {
        loadDashboard();
    }
});

// --- Form ---
function setCreateMode() {
    const form = $('formLampadaire');
    if (form) form.action = '/lampadaires';
    $('lampadaire_id').value = '';
    $('formMode').textContent = 'Ajouter un lampadaire';
    $('submitButton').textContent = 'Ajouter';
    $('cancelEdit').classList.add('form-hidden');
    editingLampadaire = null;
}

function fillForm(l) {
    const form = $('formLampadaire');
    if (form) form.classList.remove('form-hidden');
    $('helper').textContent = `Emplacement: ${l.latitude}, ${l.longitude}`;
    $('latitude').value = l.latitude; $('longitude').value = l.longitude;
    $('latitude_display').value = l.latitude; $('longitude_display').value = l.longitude;
    ['reference','zone','type_driver','protocole','puissance','etat','date_installation',
     'address','device_uid','node_address','notes','lcu_id'].forEach(f => {
        const el = $(f);
        if (el) el.value = l[f] || '';
    });
    const ii = $('intensite'), iv = $('intensite_value');
    if (ii) { ii.value = l.intensite || 0; if (iv) iv.textContent = ii.value; }
    setEditMode(l);
}

function setEditMode(l) {
    $('formLampadaire').action = `/lampadaires/${l.id}`;
    $('lampadaire_id').value = l.id;
    $('formMode').textContent = `Modifier ${l.reference}`;
    $('submitButton').textContent = 'Mettre à jour';
    $('cancelEdit').classList.remove('form-hidden');
    editingLampadaire = l;
}

function editLamp(id) {
    const l = lampadaires.find(x => x.id === id);
    if (l) fillForm(l);
}

function archiveLamp(id) {
    if (confirm('Archiver ce lampadaire ?')) {
        const form = document.createElement('form');
        form.method = 'POST'; form.action = `/lampadaires/${id}/archive`;
        document.body.appendChild(form); form.submit();
    }
}

// --- Filters ---
function applyFilters() {
    const params = new URLSearchParams();
    const q = $('filterQ').value; if (q) params.set('q', q);
    const e = $('filterEtat').value; if (e) params.set('etat', e);
    const z = $('filterZone').value; if (z) params.set('zone', z);
    const d = $('filterDriver').value; if (d) params.set('driver', d);
    params.set('view', 'carte'); // Rester sur la carte après filtrage
    window.location.href = '/?' + params.toString();
}

// --- Detail Panel ---
function showDetailByID(id) {
    fetch(`/api/lampadaires/${id}`).then(r => r.json()).then(l => {
        $('detailTitle').textContent = `Fiche: ${l.reference}`;
        let html = `<div class="detail-section"><h4>Informations générales</h4><div class="detail-grid">
            <div class="detail-item"><div class="dl">Référence</div><div class="dv">${l.reference}</div></div>
            <div class="detail-item"><div class="dl">Zone</div><div class="dv">${l.zone||'—'}</div></div>
            <div class="detail-item"><div class="dl">Quartier</div><div class="dv">${l.quartier||'—'}</div></div>
            <div class="detail-item"><div class="dl">Adresse</div><div class="dv">${l.address||'—'}</div></div>
            <div class="detail-item"><div class="dl">Position</div><div class="dv">${l.latitude}, ${l.longitude}</div></div>
            <div class="detail-item"><div class="dl">Installation</div><div class="dv">${l.date_installation||'—'}</div></div>
        </div></div>`;
        html += `<div class="detail-section"><h4>Configuration technique</h4><div class="detail-grid">
            <div class="detail-item"><div class="dl">Type driver</div><div class="dv">${l.type_driver||'—'}</div></div>
            <div class="detail-item"><div class="dl">Réf. driver</div><div class="dv">${l.driver_reference||'—'}</div></div>
            <div class="detail-item"><div class="dl">Protocole</div><div class="dv">${l.protocole||'—'}</div></div>
            <div class="detail-item"><div class="dl">LCU / Gateway</div><div class="dv">${l.lcu_reference||'—'}</div></div>
            <div class="detail-item"><div class="dl">Puissance</div><div class="dv">${l.puissance?l.puissance+'W':'—'}</div></div>
        </div></div>`;
        html += `<div class="detail-section"><h4>État opérationnel</h4><div class="detail-grid">
            <div class="detail-item"><div class="dl">État</div><div class="dv"><span class="badge ${l.etat}">${l.etat}</span></div></div>
            <div class="detail-item"><div class="dl">Intensité</div><div class="dv">${l.intensite}%</div></div>
            <div class="detail-item"><div class="dl">Dernière comm.</div><div class="dv">${fmt(l.last_seen_at)}</div></div>
            <div class="detail-item"><div class="dl">Dernière cmd</div><div class="dv">${fmt(l.last_command_at)}</div></div>
        </div></div>`;

        // IoT data section
        html += `<div class="detail-section"><h4>Données IoT</h4><div id="detailIoT">Chargement...</div></div>`;
        html += `<div class="detail-section"><h4>Calculateur</h4><div id="detailCalc">
            <button class="btn btn-secondary btn-sm" onclick="runCalcForDetail(${l.id})">🧠 Analyser</button>
            <button class="btn btn-primary btn-sm" onclick="runCalcForDetail(${l.id},true)">⚡ Analyser + Appliquer</button>
            <div id="detailCalcResult" style="margin-top:8px;"></div></div></div>`;

        $('detailContent').innerHTML = html;
        $('detailPanel').classList.remove('hidden');

        // Load IoT data
        fetch(`/api/lampadaires/${id}/telemetry/latest`).then(r=>r.ok?r.json():null).then(m => {
            if (!m) { $('detailIoT').innerHTML = '<span style="color:var(--text-dim)">Aucune donnée</span>'; return; }
            $('detailIoT').innerHTML = `<div class="detail-grid">
                <div class="detail-item"><div class="dl">Luminosité</div><div class="dv">${val(m.luminosite,' lux')}</div></div>
                <div class="detail-item"><div class="dl">Présence</div><div class="dv">${m.presence?'✅ Oui':'❌ Non'}</div></div>
                <div class="detail-item"><div class="dl">Température</div><div class="dv">${val(m.temperature,'°C')}</div></div>
                <div class="detail-item"><div class="dl">Humidité</div><div class="dv">${val(m.humidite,'%')}</div></div>
                <div class="detail-item"><div class="dl">Tension</div><div class="dv">${val(m.tension,'V')}</div></div>
                <div class="detail-item"><div class="dl">Courant</div><div class="dv">${val(m.courant,'A')}</div></div>
                <div class="detail-item"><div class="dl">Puissance</div><div class="dv">${val(m.puissance,'W')}</div></div>
                <div class="detail-item"><div class="dl">Énergie</div><div class="dv">${val(m.energie,'kWh')}</div></div>
            </div><div style="margin-top:6px;font-size:11px;color:var(--text-dim);">Dernière MAJ: ${fmt(m.created_at)}</div>`;
        }).catch(() => { $('detailIoT').innerHTML = 'Erreur chargement'; });
    });
}

function closeDetail() { $('detailPanel').classList.add('hidden'); }

function runCalcForDetail(id, apply) {
    fetch(`/api/calculateur/run/${id}`, { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({apply:!!apply}) })
    .then(r=>r.json()).then(d => {
        $('detailCalcResult').innerHTML = `<div class="panel" style="padding:12px;margin-top:4px;">
            <strong>Recommandation: ${d.recommended_intensity}%</strong><br/>
            Raison: ${d.decision_reason}<br/>
            Confiance: ${(d.confidence*100).toFixed(0)}% · Appliqué: ${d.applied?'✅':'❌'}
        </div>`;
    });
}

// --- Dashboard ---
function loadDashboard() {
    fetch('/api/dashboard/stats').then(r=>r.json()).then(s => {
        $('dashStats').innerHTML = `
            <div class="stat-card"><div class="stat-label">Total lampadaires</div><div class="stat-value">${s.total_lampadaires}</div></div>
            <div class="stat-card"><div class="stat-label">Online</div><div class="stat-value green">${s.lampadaires_online}</div></div>
            <div class="stat-card"><div class="stat-label">Offline</div><div class="stat-value red">${s.lampadaires_offline}</div></div>
            <div class="stat-card"><div class="stat-label">Maintenance</div><div class="stat-value orange">${s.lampadaires_maintenance}</div></div>
            <div class="stat-card"><div class="stat-label">Offline (15m+)</div><div class="stat-value red">${s.inactive_lampadaires}</div></div>
            <div class="stat-card"><div class="stat-label">Alertes ouvertes</div><div class="stat-value red">${s.open_alerts}</div></div>
            <div class="stat-card"><div class="stat-label">Commandes/Jour</div><div class="stat-value blue">${s.commands_today}</div></div>
            <div class="stat-card"><div class="stat-label">Économie ést.</div><div class="stat-value green">-${s.estimated_saving_percent.toFixed(0)}%</div></div>
            <div class="stat-card"><div class="stat-label">Puissance économisée</div><div class="stat-value green">${(s.estimated_power_saving_w/1000).toFixed(2)} kW</div></div>`;

        const al = s.recent_alerts || [];
        $('dashAlerts').innerHTML = al.length ? al.map(a=>`<li><span><span class="badge ${a.severity}">${a.severity}</span> ${a.message.substring(0,50)}</span><span class="activity-time">${fmt(a.created_at)}</span></li>`).join('') : '<li>Aucune alerte</li>';

        const cm = s.recent_commands || [];
        $('dashCommands').innerHTML = cm.length ? cm.map(c=>`<li><span>💡 ${c.old_intensity??'?'}→${c.new_intensity}% (${c.source})</span><span class="activity-time">${fmt(c.created_at)}</span></li>`).join('') : '<li>Aucune commande</li>';

        const tl = s.recent_telemetry || [];
        $('dashTelemetry').innerHTML = tl.length ? tl.map(t=>`<li><span>📡 #${t.lampadaire_id} · ${val(t.puissance,'W')} · ${val(t.temperature,'°C')}</span><span class="activity-time">${fmt(t.created_at)}</span></li>`).join('') : '<li>Aucune donnée</li>';
    });
}

// --- Alerts ---
function loadAlertCounts() {
    fetch('/api/alerts/counts').then(r=>r.json()).then(c => {
        $('alertCounts').innerHTML = `
            <div class="stat-card"><div class="stat-label">Ouvertes</div><div class="stat-value red">${c.total}</div></div>
            <div class="stat-card"><div class="stat-label">Critiques</div><div class="stat-value red">${c.critical}</div></div>
            <div class="stat-card"><div class="stat-label">Warning</div><div class="stat-value orange">${c.warning}</div></div>
            <div class="stat-card"><div class="stat-label">Résolues</div><div class="stat-value green">${c.resolved}</div></div>`;
    });
}

function loadAlerts() {
    const status = $('alertFilterStatus').value;
    const severity = $('alertFilterSeverity').value;
    const params = new URLSearchParams();
    if (status) params.set('status', status);
    if (severity) params.set('severity', severity);
    fetch('/api/alerts?' + params).then(r=>r.json()).then(alerts => {
        $('alertsBody').innerHTML = (alerts||[]).map(a => `<tr>
            <td>${fmt(a.created_at)}</td><td>${a.reference||'#'+(a.lampadaire_id||'—')}</td>
            <td>${a.type}</td><td><span class="badge ${a.severity}">${a.severity}</span></td>
            <td>${a.message}</td><td><span class="badge ${a.status}">${a.status}</span></td>
            <td>${a.status==='open'?`<button class="btn btn-primary btn-sm" onclick="resolveAlert(${a.id})">Résoudre</button>`:''}</td>
        </tr>`).join('') || '<tr><td colspan="7">Aucune alerte</td></tr>';
    });
}

function resolveAlert(id) {
    fetch(`/api/alerts/${id}/resolve`, {method:'POST'}).then(()=>{ loadAlerts(); loadAlertCounts(); });
}

// --- Dimming ---
function applyDimming() {
    const id = $('dimmingLamp').value;
    if (!id) { alert('Sélectionnez un lampadaire'); return; }
    fetch(`/api/lampadaires/${id}/dimming`, {
        method:'POST', headers:{'Content-Type':'application/json'},
        body: JSON.stringify({ new_intensity: parseInt($('dimmingSlider').value), source:'admin', reason: $('dimmingReason').value||'Commande manuelle' })
    }).then(r=>r.json()).then(d => {
        $('dimmingResult').innerHTML = `<div class="alert-banner success">✅ Dimming appliqué: ${d.command.old_intensity}% → ${d.command.new_intensity}%</div>`;
        loadDimmingHistory();
    });
}

function loadDimmingHistory() {
    const id = $('dimmingLamp').value;
    if (!id) return;
    fetch(`/api/lampadaires/${id}/dimming`).then(r=>r.json()).then(cmds => {
        $('dimmingHistory').innerHTML = (cmds||[]).map(c => `<tr>
            <td>${fmt(c.created_at)}</td><td>#${c.lampadaire_id}</td><td>${c.source}</td>
            <td>${c.old_intensity??'—'}%</td><td>${c.new_intensity}%</td>
            <td>${c.reason||'—'}</td><td><span class="badge ${c.status}">${c.status}</span></td>
        </tr>`).join('');
    });
}

// --- Calculator ---
function runCalc(apply) {
    const id = $('calcLamp').value;
    if (!id) { alert('Sélectionnez un lampadaire'); return; }
    fetch(`/api/calculateur/run/${id}`, { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({apply}) })
    .then(r=>r.json()).then(d => {
        $('calcResult').innerHTML = `<div class="panel" style="padding:14px;margin-top:8px;">
            <div class="stat-label">Recommandation</div>
            <div class="stat-value green" style="font-size:32px;">${d.recommended_intensity}%</div>
            <p style="margin:8px 0;color:var(--text-dim);">${d.decision_reason}</p>
            <p>Confiance: <strong>${(d.confidence*100).toFixed(0)}%</strong> · Appliqué: ${d.applied?'✅ Oui':'❌ Non'}</p></div>`;
        loadCalcHistory(id);
    });
}

function runCalcAll() {
    fetch('/api/calculateur/run-all', { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({apply:false}) })
    .then(r=>r.json()).then(d => {
        $('calcResult').innerHTML = `<div class="alert-banner success">✅ ${d.count} analyses effectuées</div>`;
    });
}

function loadCalcHistory(id) {
    fetch(`/api/lampadaires/${id}/decisions`).then(r=>r.json()).then(decs => {
        $('calcHistory').innerHTML = (decs||[]).map(d => `<tr>
            <td>${fmt(d.created_at)}</td><td>#${d.lampadaire_id}</td>
            <td><strong>${d.recommended_intensity}%</strong></td><td>${d.decision_reason}</td>
            <td>${(d.confidence*100).toFixed(0)}%</td>
            <td><span class="badge ${d.applied?'applied':'pending'}">${d.applied?'Oui':'Non'}</span></td>
        </tr>`).join('');
    });
}

// --- Simulation ---
function simulateOne() {
    const id = $('simLamp').value;
    if (!id) { alert('Sélectionnez un lampadaire'); return; }
    fetch(`/api/simulator/telemetry/${id}`, {method:'POST'}).then(r=>r.json()).then(d => {
        const m = d.measurement;
        $('simResult').innerHTML = `<div class="alert-banner success">✅ Mesure générée pour #${m.lampadaire_id}</div>`;
        loadSimHistory(id);
        if (d.alerts && d.alerts.length) {
            $('simResult').innerHTML += `<div class="alert-banner error">⚠️ ${d.alerts.length} alerte(s) générée(s)</div>`;
        }
    });
}

function simulateAll() {
    fetch('/api/simulator/telemetry/all', {method:'POST'}).then(r=>r.json()).then(d => {
        $('simResult').innerHTML = `<div class="alert-banner success">✅ ${d.count} mesures générées</div>`;
    });
}

function loadSimHistory(id) {
    fetch(`/api/lampadaires/${id}/telemetry`).then(r=>r.json()).then(ms => {
        $('simHistory').innerHTML = (ms||[]).slice(0,20).map(m => `<tr>
            <td>${fmt(m.created_at)}</td><td>${val(m.luminosite,' lux')}</td>
            <td>${m.presence?'✅':'❌'}</td><td>${val(m.temperature,'°C')}</td>
            <td>${val(m.humidite,'%')}</td><td>${val(m.puissance,'W')}</td>
        </tr>`).join('');
    });
}

function toggleAutoSim() {
    const btn = $('autoSimBtn');
    if (autoSimInterval) {
        clearInterval(autoSimInterval);
        autoSimInterval = null;
        btn.textContent = '▶ Auto-simulation';
        btn.classList.remove('btn-danger');
        btn.classList.add('btn-secondary');
    }
}

function centerMap(lat, lng) {
    if (map) map.setView([lat, lng], 18);
}

function simulateAnomaly(id) {
    fetch(`/api/simulator/telemetry/${id}?anomaly=true`, {method:'POST'})
    .then(r=>r.json()).then(d => {
        alert(`Anomalie générée ! ${d.alerts.length} alerte(s) créée(s).`);
        if (editingLampadaire && editingLampadaire.id === id) showDetailByID(id);
    });
}

// --- LCU Management ---
function loadLCUs() {
    fetch('/api/lcus').then(r=>r.json()).then(list => {
        $('lcuList').innerHTML = (list||[]).map(l => `
            <div class="lcu-card">
                <div class="lcu-card-header">
                    <div>
                        <strong style="font-size:16px;">${l.reference}</strong><br/>
                        <small style="color:var(--text-dim);">${l.name || 'Sans nom'}</small>
                    </div>
                    <span class="badge ${l.status}">${l.status}</span>
                </div>
                <div class="lcu-card-stats">
                    <div>🌐 IP: ${l.ip_address}:${l.port}</div>
                    <div>🛠️ Protocole: ${l.protocol}</div>
                    <div>🕒 Dern. comm: ${fmt(l.last_seen_at)}</div>
                    <div>🔄 Dern. sync: ${fmt(l.last_sync_at)}</div>
                </div>
                <div class="lcu-card-actions">
                    <button class="btn btn-secondary btn-sm" onclick="testLCU(${l.id})">🔌 Test</button>
                    <button class="btn btn-primary btn-sm" onclick="syncLCU(${l.id})">🔄 Sync</button>
                    <button class="btn btn-secondary btn-sm" onclick="openLCUModal(${JSON.stringify(l).replace(/"/g, '&quot;')})">⚙️ Config</button>
                </div>
            </div>
        `).join('') || '<p>Aucune LCU enregistrée.</p>';
    });
}

function openLCUModal(lcu = null) {
    $('lcuModal').classList.remove('hidden');
    if (lcu) {
        $('lcuFormMode').textContent = 'Modifier la Gateway';
        $('formLcu').action = `/lcus/${lcu.id}`;
        $('lcu_id_field').value = lcu.id;
        $('lcu_reference_field').value = lcu.reference;
        $('lcu_name_field').value = lcu.name || '';
        $('lcu_ip_field').value = lcu.ip_address;
        $('lcu_port_field').value = lcu.port;
        $('lcu_protocol_field').value = lcu.protocol;
        $('lcu_zone_field').value = lcu.zone || '';
        $('lcu_lat_field').value = lcu.latitude || '';
        $('lcu_lng_field').value = lcu.longitude || '';
    } else {
        $('lcuFormMode').textContent = 'Ajouter une Gateway';
        $('formLcu').action = '/lcus';
        $('lcu_id_field').value = '';
        $('formLcu').reset();
        $('lcu_port_field').value = 8080;
    }
}

function closeLCUModal() { $('lcuModal').classList.add('hidden'); }

function testLCU(id) {
    fetch(`/api/lcus/${id}/test`, {method:'POST'}).then(r=>r.json()).then(d => {
        alert(d.message || "Test réussi");
        loadLCUs();
    }).catch(e => alert("Erreur: " + e.message));
}

function syncLCU(id) {
    fetch(`/api/lcus/${id}/sync`, {method:'POST'}).then(r=>r.json()).then(d => {
        alert(d.message || "Synchronisation réussie");
        loadLCUs();
        if (window.location.search.includes('view=carte')) window.location.reload();
    }).catch(e => alert("Erreur: " + e.message));
}

// --- Location Correction ---
function loadMissingLocation() {
    fetch('/api/lampadaires/missing-location').then(r=>r.json()).then(list => {
        $('missingLocationBody').innerHTML = (list||[]).map(l => `
            <tr>
                <td><strong>${l.reference}</strong></td>
                <td>${l.lcu_reference || '—'}</td>
                <td><code style="font-size:11px;">${l.device_uid}</code></td>
                <td>${l.node_address || '—'}</td>
                <td>${l.zone || '—'}</td>
                <td><button class="btn btn-primary btn-sm" onclick="startPlacement(${l.id})">📍 Placer sur carte</button></td>
            </tr>
        `).join('') || '<tr><td colspan="6">Aucun équipement à localiser.</td></tr>';
    });
}

function startPlacement(id) {
    placementLampID = id;
    document.querySelector('.nav-btn[data-page="carte"]').click();
    $('placementBanner').classList.remove('hidden');
}

function cancelPlacementMode() {
    placementLampID = null;
    $('placementBanner').classList.add('hidden');
}

function confirmPlacement(lat, lng) {
    if (!placementLampID) return;
    if (confirm(`Confirmer la position (${lat.toFixed(5)}, ${lng.toFixed(5)}) ?`)) {
        fetch(`/api/lampadaires/${placementLampID}/location`, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ latitude: lat, longitude: lng, status: 'confirmed' })
        }).then(r => r.json()).then(() => {
            alert("Position enregistrée !");
            window.location.href = "/?view=carte";
        });
    }
}


// --- NOUVEAUX MODULES ---
async function loadEnergieStats() {
    try {
        const res = await fetch('/api/energy/summary');
        if (res.ok) {
            const data = await res.json();
            document.getElementById('energyNominal').innerText = data.total_nominal_power_w + ' W';
            document.getElementById('energyCurrent').innerText = data.estimated_current_power_w + ' W';
            document.getElementById('energySavingW').innerText = data.estimated_saving_w + ' W';
            document.getElementById('energySavingPercent').innerText = data.estimated_saving_percent.toFixed(2) + ' %';

            let html = '';
            if (data.by_zone && data.by_zone.length > 0) {
                data.by_zone.forEach(z => {
                    html += '<tr><td>'+z.zone+'</td><td>'+z.lamp_count+'</td><td>'+z.nominal_power_w+' W</td><td>'+z.current_power_w+' W</td><td>'+z.saving_w+' W</td></tr>';
                });
            } else { html = '<tr><td colspan="5" style="text-align:center;">Aucune donn�e par zone</td></tr>'; }
            document.getElementById('energyZoneList').innerHTML = html;
        }
    } catch (e) { console.error('Error loadEnergieStats', e); }
}

function reloadApp() {} // placeholder
function exportEnergyReport() { window.location.href = '/api/reports/export?type=energy'; }

async function loadInterventions() {}
async function loadAdminUsers() {}
async function loadAccessLogs() {}
async function loadAdminSettings() {}
async function toggleSetting(key, val) {}
