// --- Navigation ---
const lampadaires = window.LAMPADAIRES || [];
const lcus = window.LCUS || [];
let map, newMarker, editingLampadaire, autoSimInterval;
let placementLampID = null;
let currentMapMode = 'add_lcu'; // Modes: add_lcu, add_lampadaire_manual, place_missing_lampadaire, view

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
        if (page === 'profiles') { loadLightingProfiles(); }
        if (page === 'basestations') { loadBasestations(); }
        if (page === 'armoires') { loadCabinets(); }
        if (page === 'controllers') { loadControllers(); }
        if (page === 'workorders') { loadWorkOrders(); }
        if (page === 'commissioning') { loadCommissioning(); }

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
function esc(str) { const d = document.createElement('div'); d.textContent = str ?? '—'; return d.innerHTML; }
function showToast(msg, type = 'error') {
    const el = document.createElement('div');
    el.className = 'alert-banner ' + type;
    el.textContent = msg;
    const content = document.querySelector('.content');
    if (content) { content.prepend(el); setTimeout(() => el.remove(), 4000); }
}
function withLoading(el, promise) {
    if (el) el.classList.add('loading');
    return promise.finally(() => { if (el) el.classList.remove('loading'); });
}

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
    const savedView = (() => { try { return JSON.parse(localStorage.getItem('mapView')); } catch { return null; } })();
    const initCenter = (savedView && savedView.lat != null) ? [savedView.lat, savedView.lng] : [34.0209, -6.8416];
    const initZoom   = (savedView && savedView.zoom != null) ? savedView.zoom : 13;
    map = L.map('map', { maxZoom: 20 }).setView(initCenter, initZoom);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: 'OpenStreetMap', maxZoom: 20, maxNativeZoom: 19
    }).addTo(map);
    map.on('moveend zoomend', () => {
        const c = map.getCenter();
        localStorage.setItem('mapView', JSON.stringify({ lat: c.lat, lng: c.lng, zoom: map.getZoom() }));
    });

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
                Nom: <strong>${l.name || '—'}</strong><br/>
                IP: <strong>${l.ip_address}:${l.port}</strong> (${l.protocol})<br/>
                Zone: <strong>${l.zone || '—'}</strong><br/>
                Lampes reliées: <strong>${linkedLamps}</strong><br/>
                Dernière sync: ${fmt(l.last_sync_at)}
            </div>
            <div class="popup-actions">
                <button onclick="testLCU(${l.id})" class="btn-sm">🔌 Tester</button>
                <button onclick="syncLCU(${l.id})" class="btn-sm" style="background:var(--accent);color:#000;">🔄 Sync</button>
                <button onclick="openLCUModal(${l.id})" class="btn-sm" style="background:var(--info);color:#fff;">Modifier</button>
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
                Location: <span class="badge ${l.location_status}">${l.location_status}</span><br/>
                Commissioning: <span class="badge status-${l.commissioning_status}">${l.commissioning_status}</span><br/>
                <select onchange="updateCommissioning(${l.id}, this.value)" style="margin-top:5px;width:100%;font-size:11px;">
                    <option value="">Changer statut...</option>
                    <option value="discovered" ${l.commissioning_status==='discovered'?'selected':''}>Discovered</option>
                    <option value="located" ${l.commissioning_status==='located'?'selected':''}>Located</option>
                    <option value="configured" ${l.commissioning_status==='configured'?'selected':''}>Configured</option>
                    <option value="tested" ${l.commissioning_status==='tested'?'selected':''}>Tested</option>
                    <option value="commissioned" ${l.commissioning_status==='commissioned'?'selected':''}>Commissioned</option>
                </select><br/>
                LCU: <strong>${l.lcu_reference || '—'}</strong><br/>
                Intensité: <strong>${l.intensite}%</strong><br/>
                Zone: <strong>${l.zone||'—'}</strong>
            </div>
            <div class="popup-actions">
                <button onclick="editLamp(${l.id})" class="btn-sm" style="background:var(--info);color:#fff;">Modifier</button>
                <button onclick="showDetailByID(${l.id})" class="btn-sm" style="background:var(--accent);color:#000;">Fiche</button>
                <button onclick="startPlacementMode(${l.id})" class="btn-sm" style="background:var(--secondary);color:#fff;">Corriger loc.</button>
            </div></div>`;

        const marker = L.marker([l.latitude, l.longitude], { icon }).addTo(map).bindPopup(popup);
        marker.on('click', () => fillForm(l));
    });

    // Map click
    map.on('click', function(e) {
        const lat = e.latlng.lat.toFixed(7);
        const lng = e.latlng.lng.toFixed(7);

        if (currentMapMode === 'place_missing_lampadaire') {
            confirmPlacement(lat, lng);
            return;
        }

        if (currentMapMode === 'add_lcu') {
            $('lcu_lat_map').value = lat;
            $('lcu_lng_map').value = lng;
            $('helper').textContent = `LCU: ${lat}, ${lng}`;
            $('mapFormLcu').classList.remove('form-hidden');
            $('formLampadaire').classList.add('form-hidden');
            if (newMarker) map.removeLayer(newMarker);
            newMarker = L.marker([lat, lng]).addTo(map).bindPopup('Nouvelle Gateway').openPopup();
            return;
        }

        if (currentMapMode === 'add_lampadaire_manual') {
            if (!editingLampadaire) setCreateMode();
            $('latitude').value = lat;
            $('longitude').value = lng;
            $('latitude_display').value = lat;
            $('longitude_display').value = lng;
            $('helper').textContent = `Lampadaire: ${lat}, ${lng}`;
            $('formLampadaire').classList.remove('form-hidden');
            $('mapFormLcu').classList.add('form-hidden');
            if (newMarker) map.removeLayer(newMarker);
            newMarker = L.marker([lat, lng]).addTo(map).bindPopup('Nouveau lampadaire').openPopup();
            return;
        }
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
    const title = $('formModeTitle');
    if (title && currentMapMode === 'add_lampadaire_manual') title.textContent = 'Ajouter un lampadaire';
    $('submitButton').textContent = 'Ajouter';
    $('cancelEdit').classList.add('form-hidden');
    editingLampadaire = null;
}

// --- Map Modes ---
function setMapMode(mode) {
    currentMapMode = mode;
    const indicator = $('mapModeIndicator');
    const title = $('formModeTitle');
    const instructions = $('formInstructions');
    
    // Reset
    if (newMarker) { map.removeLayer(newMarker); newMarker = null; }
    $('mapFormLcu').classList.add('form-hidden');
    $('formLampadaire').classList.add('form-hidden');
    $('helper').textContent = 'Aucun emplacement sélectionné.';

    if (mode === 'add_lcu') {
        indicator.textContent = 'Mode: Ajouter LCU / Gateway';
        title.textContent = 'Ajouter une LCU';
        instructions.textContent = 'Cliquez sur la carte pour placer la Gateway.';
    } else if (mode === 'add_lampadaire_manual') {
        indicator.textContent = 'Mode: Ajouter lampadaire manuel';
        title.textContent = 'Ajouter un lampadaire';
        instructions.textContent = 'Cliquez sur la carte pour choisir un emplacement.';
        setCreateMode();
    } else if (mode === 'view') {
        indicator.textContent = 'Mode: Consultation';
        title.textContent = 'Consultation';
        instructions.textContent = 'Cliquez sur un équipement pour voir les détails.';
    } else if (mode === 'place_missing_lampadaire') {
        indicator.textContent = 'Mode: Placement assisté';
        title.textContent = 'Placer équipement';
        instructions.textContent = 'Cliquez sur la carte pour localiser le lampadaire.';
    }
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
            <div class="detail-item"><div class="dl">Référence</div><div class="dv">${esc(l.reference)}</div></div>
            <div class="detail-item"><div class="dl">Zone</div><div class="dv">${esc(l.zone)}</div></div>
            <div class="detail-item"><div class="dl">Quartier</div><div class="dv">${esc(l.quartier)}</div></div>
            <div class="detail-item"><div class="dl">Adresse</div><div class="dv">${esc(l.address)}</div></div>
            <div class="detail-item"><div class="dl">Position</div><div class="dv">${l.latitude}, ${l.longitude}</div></div>
            <div class="detail-item"><div class="dl">Installation</div><div class="dv">${esc(l.date_installation)}</div></div>
        </div></div>`;
        html += `<div class="detail-section"><h4>Configuration technique</h4><div class="detail-grid">
            <div class="detail-item"><div class="dl">Type driver</div><div class="dv">${esc(l.type_driver)}</div></div>
            <div class="detail-item"><div class="dl">Réf. driver</div><div class="dv">${esc(l.driver_reference)}</div></div>
            <div class="detail-item"><div class="dl">Protocole</div><div class="dv">${esc(l.protocole)}</div></div>
            <div class="detail-item"><div class="dl">LCU / Gateway</div><div class="dv">${esc(l.lcu_reference)}</div></div>
            <div class="detail-item"><div class="dl">Puissance</div><div class="dv">${l.puissance ? l.puissance + 'W' : '—'}</div></div>
        </div></div>`;
        html += `<div class="detail-section"><h4>État opérationnel</h4><div class="detail-grid">
            <div class="detail-item"><div class="dl">État</div><div class="dv"><span class="badge ${esc(l.etat)}">${esc(l.etat)}</span></div></div>
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
    }).catch(e => showToast('Erreur chargement dashboard: ' + e.message));
}

// --- Alerts ---
function loadAlertCounts() {
    fetch('/api/alerts/counts').then(r=>r.json()).then(c => {
        $('alertCounts').innerHTML = `
            <div class="stat-card"><div class="stat-label">Ouvertes</div><div class="stat-value red">${c.total}</div></div>
            <div class="stat-card"><div class="stat-label">Critiques</div><div class="stat-value red">${c.critical}</div></div>
            <div class="stat-card"><div class="stat-label">Warning</div><div class="stat-value orange">${c.warning}</div></div>
            <div class="stat-card"><div class="stat-label">Résolues</div><div class="stat-value green">${c.resolved}</div></div>`;
    }).catch(e => showToast('Erreur chargement alertes: ' + e.message));
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
            <td>
                ${a.status==='open'?`<button class="btn btn-primary btn-sm" onclick="resolveAlert(${a.id})">Résoudre</button>`:''}
                ${a.status==='open'?`<button class="btn btn-secondary btn-sm" onclick="openInterventionModal(${a.id})">🔧 Intervention</button>`:''}
            </td>
        </tr>`).join('') || '<tr><td colspan="7">Aucune alerte</td></tr>';
    }).catch(e => showToast('Erreur chargement alertes: ' + e.message));
}

function resolveAlert(id) {
    fetch(`/api/alerts/${id}/resolve`, {method:'POST'}).then(()=>{ loadAlerts(); loadAlertCounts(); });
}

// --- Dimming ---
function applyDimming(btn) {
    const id = $('dimmingLamp').value;
    if (!id) { alert('Sélectionnez un lampadaire'); return; }
    withLoading(btn, fetch(`/api/lampadaires/${id}/dimming`, {
        method:'POST', headers:{'Content-Type':'application/json'},
        body: JSON.stringify({ new_intensity: parseInt($('dimmingSlider').value), source:'admin', reason: $('dimmingReason').value||'Commande manuelle' })
    }).then(r=>r.json()).then(d => {
        $('dimmingResult').innerHTML = `<div class="alert-banner success">✅ Dimming appliqué: ${d.command.old_intensity}% → ${d.command.new_intensity}%</div>`;
        loadDimmingHistory();
    }).catch(e => { showToast('Erreur dimming: ' + e.message); }));
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
function simulateOne(btn) {
    const id = $('simLamp').value;
    if (!id) { alert('Sélectionnez un lampadaire'); return; }
    withLoading(btn, fetch(`/api/simulator/telemetry/${id}`, {method:'POST'}).then(r=>r.json()).then(d => {
        const m = d.measurement;
        $('simResult').innerHTML = `<div class="alert-banner success">✅ Mesure générée pour #${m.lampadaire_id}</div>`;
        loadSimHistory(id);
        if (d.alerts && d.alerts.length) {
            $('simResult').innerHTML += `<div class="alert-banner error">⚠️ ${d.alerts.length} alerte(s) générée(s)</div>`;
        }
    }).catch(e => showToast('Erreur simulation: ' + e.message)));
}

function simulateAll() {
    fetch('/api/simulator/telemetry/all', {method:'POST'}).then(r=>r.json()).then(d => {
        $('simResult').innerHTML = `<div class="alert-banner success">✅ ${d.count} mesures générées</div>`;
    }).catch(e => showToast('Erreur simulation: ' + e.message));
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
    } else {
        const id = $('simLamp').value;
        if (!id) { alert('Sélectionnez un lampadaire'); return; }
        autoSimInterval = setInterval(() => {
            fetch(`/api/simulator/telemetry/${id}`, { method: 'POST' })
                .then(r => r.json())
                .then(() => loadSimHistory(id))
                .catch(e => console.error('Auto-sim error:', e));
        }, 5000);
        btn.textContent = '⏹ Arrêter';
        btn.classList.remove('btn-secondary');
        btn.classList.add('btn-danger');
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
                        <strong style="font-size:16px;">${esc(l.reference)}</strong><br/>
                        <small style="color:var(--text-dim);">${esc(l.name) || 'Sans nom'}</small>
                    </div>
                    <span class="badge ${esc(l.status)}">${esc(l.status)}</span>
                </div>
                <div class="lcu-card-stats">
                    <div>🌐 IP: ${esc(l.ip_address)}:${l.port}</div>
                    <div>🛠️ Protocole: ${esc(l.protocol)}</div>
                    <div>🕒 Dern. comm: ${fmt(l.last_seen_at)}</div>
                    <div>🔄 Dern. sync: ${fmt(l.last_sync_at)}</div>
                </div>
                <div class="lcu-card-actions">
                    <button class="btn btn-secondary btn-sm" onclick="testLCU(${l.id}, this)">🔌 Test</button>
                    <button class="btn btn-primary btn-sm" onclick="syncLCU(${l.id}, this)">🔄 Sync</button>
                    <button class="btn btn-secondary btn-sm" onclick="openLCUModal(${JSON.stringify(l).replace(/"/g, '&quot;')})">⚙️ Config</button>
                </div>
            </div>
        `).join('') || '<p>Aucune LCU enregistrée.</p>';
    }).catch(e => showToast('Erreur chargement LCUs: ' + e.message));
}

function openLCUModal(input = null) {
    let lcu = input;
    if (typeof input === 'number') {
        lcu = lcus.find(x => x.id === input);
    }
    
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

function testLCU(id, btn) {
    withLoading(btn, fetch(`/api/lcus/${id}/test`, {method:'POST'}).then(r=>r.json()).then(d => {
        alert(d.message || "Test réussi");
        loadLCUs();
    }).catch(e => showToast("Erreur test LCU: " + e.message)));
}

function syncLCU(id, btn) {
    withLoading(btn, fetch(`/api/lcus/${id}/sync`, {method:'POST'}).then(r=>r.json()).then(d => {
        alert(d.message || "Synchronisation réussie");
        loadLCUs();
        if (window.location.search.includes('view=carte')) window.location.reload();
    }).catch(e => showToast("Erreur sync LCU: " + e.message)));
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

async function loadAdminUsers() {
    try {
        const r = await fetch('/api/users');
        const users = await r.json();
        $('adminUserList').innerHTML = users.map(u => `<tr>
            <td>${u.full_name}</td>
            <td>${u.email}</td>
            <td><span class="badge">${u.role}</span></td>
            <td><span class="badge ${u.status}">${u.status}</span></td>
            <td><button class="btn btn-secondary btn-sm" onclick="editUser(${u.id})">Modifier</button></td>
        </tr>`).join('');
    } catch(e) {}
}

async function loadAccessLogs() {
    try {
        const r = await fetch('/api/logs');
        const logs = await r.json();
        $('adminAccessLogs').innerHTML = logs.map(l => `<li><strong>${l.action}</strong> <small>${fmt(l.created_at)}</small></li>`).join('');
    } catch(e) {}
}

async function loadAdminSettings() {
    // Placeholder for settings
}

async function toggleSetting(key, val) {
    fetch('/api/admin/settings', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({key, value: val ? 'true' : 'false'})
    });
}

// --- Commissioning ---
function updateCommissioning(id, status) {
    if (!status) return;
    fetch(`/api/lampadaires/${id}/commissioning`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({commissioning_status: status})
    }).then(r => r.json()).then(d => {
        if (d.error) {
            alert("Erreur: " + d.error);
        } else {
            alert("Statut de commissioning mis à jour !");
            window.location.reload();
        }
    });
}

// --- Lighting Profiles ---
function loadLightingProfiles() {
    fetch('/api/lighting-profiles').then(r => r.json()).then(data => {
        $('profileListBody').innerHTML = data.map(p => `<tr>
            <td><strong>${p.name}</strong></td>
            <td>${p.target_value}</td>
            <td><span class="badge">${p.target_type}</span></td>
            <td><span class="badge ${p.enabled ? 'online' : 'offline'}">${p.enabled ? 'Actif' : 'Inactif'}</span></td>
            <td><small>${p.schedules?.map(s => `${s.start_time}-${s.end_time}: ${s.intensity}%`).join('<br>') || '—'}</small></td>
            <td>
                <button class="btn btn-secondary btn-sm" onclick="openProfileDetails(${p.id})">🔍 Détails</button>
                <button class="btn btn-primary btn-sm" onclick="applyProfile(${p.id})">🚀 Appliquer</button>
                <button class="btn btn-secondary btn-sm" onclick="toggleProfile(${p.id}, ${p.enabled})">${p.enabled ? 'Désactiver' : 'Activer'}</button>
            </td>
        </tr>`).join('') || '<tr><td colspan="6" style="text-align:center;color:var(--text-dim);">Aucun profil défini.</td></tr>';
    });
}

function populateTargetValueOptions() {
    const type = $('prof_target_type').value;
    const sel = $('prof_target_value');
    sel.innerHTML = '';
    let options = [];
    if (type === 'zone') {
        options = [...new Set(lampadaires.map(l => l.zone).filter(Boolean))].sort();
    } else if (type === 'lcu') {
        // value=id so backend Atoi succeeds; label shows the human-readable reference
        if (lcus.length === 0) {
            sel.innerHTML = '<option value="">-- Aucune LCU disponible --</option>';
            return;
        }
        sel.innerHTML = lcus.filter(l => l.id).sort((a,b)=>(a.reference||'').localeCompare(b.reference||''))
            .map(l => `<option value="${l.id}">${l.reference || 'LCU #'+l.id}</option>`).join('');
        return;
    } else if (type === 'group') {
        options = [...new Set(lampadaires.map(l => l.quartier).filter(Boolean))].sort();
    }
    if (options.length === 0) {
        sel.innerHTML = '<option value="">-- Aucune valeur disponible --</option>';
    } else {
        sel.innerHTML = options.map(o => `<option value="${o}">${o}</option>`).join('');
    }
}

function openProfileModal() {
    $('prof_name').value = '';
    $('prof_desc').value = '';
    $('prof_target_type').value = 'zone';
    $('prof_start').value = '19:00';
    $('prof_end').value = '07:00';
    $('prof_intensity').value = '80';
    populateTargetValueOptions();
    $('profileModal').classList.remove('hidden');
}

function saveProfile() {
    const p = {
        name: $('prof_name').value,
        description: $('prof_desc').value,
        target_type: $('prof_target_type').value,
        target_value: $('prof_target_value').value,
        enabled: true,
        schedules: [{
            start_time: $('prof_start').value,
            end_time: $('prof_end').value,
            intensity: parseInt($('prof_intensity').value)
        }]
    };
    fetch('/api/lighting-profiles', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(p)
    }).then(r => {
        if(r.ok) {
            $('profileModal').classList.add('hidden');
            loadLightingProfiles();
        }
    });
}

function openProfileDetails(id) {
    const modal = $('profileDetailsModal');
    $('pd_lampBody').innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-dim);">Chargement…</td></tr>';
    $('pd_problems').style.display = 'none';
    modal.classList.remove('hidden');

    fetch(`/api/lighting-profiles/${id}/details`).then(r => r.json()).then(d => {
        const p = d.profile;
        $('pd_name').textContent = p.name;
        $('pd_desc').textContent = p.description || '';
        $('pd_target').textContent = `🎯 ${p.target_type === 'lcu' ? 'Gateway' : p.target_type === 'zone' ? 'Zone' : 'Groupe'}: ${p.target_value}`;
        $('pd_schedule').textContent = `🕐 ${p.schedules?.map(s => `${s.start_time}–${s.end_time} · ${s.intensity}%`).join('  |  ') || 'Aucun horaire'}`;
        $('pd_total').textContent = `💡 ${d.total} lampadaire${d.total !== 1 ? 's' : ''}`;

        if (d.problematic > 0) {
            $('pd_problems').textContent = `⚠ ${d.problematic} avec problème`;
            $('pd_problems').style.display = 'inline-flex';
        }

        const lamps = d.lamps || [];
        if (lamps.length === 0) {
            $('pd_lampBody').innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-dim);">Aucun lampadaire trouvé pour cette cible.</td></tr>';
            return;
        }

        $('pd_lampBody').innerHTML = lamps.map(l => {
            const etatBadge = `<span class="badge ${l.etat}">${l.etat}</span>`;
            const probCell = l.problem
                ? `<span style="color:#f87171;font-size:12px;">⚠ ${l.problem}</span>`
                : `<span style="color:#4ade80;font-size:12px;">✓ OK</span>`;
            return `<tr style="${l.problem ? 'background:rgba(239,68,68,0.05);' : ''}">
                <td><strong>${l.reference}</strong></td>
                <td><small>${l.zone || '—'}</small></td>
                <td>${etatBadge}</td>
                <td>${l.intensite}%</td>
                <td>${probCell}</td>
            </tr>`;
        }).join('');
    }).catch(() => {
        $('pd_lampBody').innerHTML = '<tr><td colspan="5" style="text-align:center;color:#f87171;">Erreur de chargement.</td></tr>';
    });
}

function applyProfile(id) {
    fetch(`/api/lighting-profiles/${id}/apply`, {method: 'POST'}).then(r => r.json().then(d => {
        if (!r.ok) { showToast('Erreur: ' + (d.error || r.status)); return; }
        showToast(`Profil appliqué à ${d.count} lampadaire(s).`);
    }));
}

function toggleProfile(id, current) {
    fetch(`/api/lighting-profiles/${id}/${current ? 'disable' : 'enable'}`, {method: 'POST'}).then(() => loadLightingProfiles());
}

// --- Interventions ---
async function loadInterventions() {
    fetch('/api/interventions').then(r => r.json()).then(data => {
        $('interventionListBody').innerHTML = data.map(i => `<tr>
            <td>#${i.id}</td>
            <td><strong>${esc(i.title)}</strong></td>
            <td>${i.alert_id ? '#' + i.alert_id : '—'}</td>
            <td>#${i.lampadaire_id}</td>
            <td>${esc(i.assigned_to)}</td>
            <td><span class="badge ${esc(i.priority)}">${esc(i.priority)}</span></td>
            <td><span class="badge ${esc(i.status)}">${esc(i.status)}</span></td>
            <td>${fmt(i.created_at)}</td>
            <td>
                ${i.status === 'open' ? `<button class="btn btn-primary btn-sm" onclick="updateInterventionStatus(${i.id}, 'start')">▶ Démarrer</button>` : ''}
                ${i.status === 'in_progress' ? `<button class="btn btn-success btn-sm" onclick="updateInterventionStatus(${i.id}, 'resolve')">✔️ Résoudre</button>` : ''}
                ${(i.status === 'resolved' || i.status === 'in_progress') ? `<button class="btn btn-secondary btn-sm" onclick="closeIntervention(${i.id})">🔒 Clôturer</button>` : ''}
            </td>
        </tr>`).join('') || '<tr><td colspan="9" style="text-align:center;color:var(--text-dim);">Aucune intervention en cours.</td></tr>';
    }).catch(e => showToast('Erreur chargement interventions: ' + e.message));
}

function openInterventionModal(alertId = null) {
    $('int_alert_id').value = alertId || '';
    $('int_title').value = alertId ? `Intervention sur alerte #${alertId}` : '';
    $('interventionModal').classList.remove('hidden');
    
    // Populate tech list
    fetch('/api/users').then(r => r.json()).then(users => {
        const sel = $('int_assigned_to');
        sel.innerHTML = '<option value="">Non assigné</option>' + 
            users.filter(u => u.role === 'technicien' || u.role === 'admin').map(u => `<option value="${u.id}">${u.full_name}</option>`).join('');
    });
}

function saveIntervention() {
    const alertId = $('int_alert_id').value;
    const url = alertId ? `/api/alerts/${alertId}/intervention` : '/api/interventions';
    const body = {
        title: $('int_title').value,
        description: $('int_desc').value,
        priority: $('int_priority').value,
        assigned_to: $('int_assigned_to').value ? parseInt($('int_assigned_to').value) : null
    };
    
    fetch(url, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(body)
    }).then(r => {
        if(r.ok) {
            $('interventionModal').classList.add('hidden');
            loadInterventions();
        }
    });
}

function updateInterventionStatus(id, action) {
    fetch(`/api/interventions/${id}/${action}`, {method: 'POST'}).then(() => loadInterventions());
}

function closeIntervention(id) {
    if(confirm("Voulez-vous clôturer cette intervention ? Cela résoudra également l'alerte liée.")) {
        fetch(`/api/interventions/${id}/close`, {method: 'POST'}).then(() => loadInterventions());
    }
}

// --- Assisted Localization ---
function loadMissingLocation() {
    fetch('/api/lampadaires/missing-location').then(r => r.json()).then(data => {
        $('missingLocationBody').innerHTML = data.map(l => `<tr>
            <td><strong>${l.reference}</strong></td>
            <td>${l.lcu_id || '—'}</td>
            <td><code>${l.device_uid || '—'}</code></td>
            <td>${l.node_address || '—'}</td>
            <td>${l.zone || '—'}</td>
            <td><span class="badge ${l.etat}">${l.etat}</span></td>
            <td>
                <button class="btn btn-primary btn-sm" onclick="startPlacementMode(${l.id})">📍 Localiser sur carte</button>
            </td>
        </tr>`).join('') || '<tr><td colspan="7" style="text-align:center;color:var(--text-dim);">Tous les lampadaires sont localisés.</td></tr>';
    });
}

function startPlacementMode(id) {
    placementLampID = id;
    currentMapMode = 'place_missing_lampadaire';
    document.querySelectorAll('.nav-btn[data-page="carte"]')[0].click();
    $('placementBanner').classList.remove('hidden');
    $('mapModeIndicator').textContent = "Mode: Positionnement de l'équipement";
    $('mapModeIndicator').style.background = "rgba(239,68,68,0.1)";
    $('mapModeIndicator').style.borderColor = "rgba(239,68,68,0.2)";
    $('mapModeIndicator').style.color = "#ef4444";
}

function cancelPlacementMode() {
    placementLampID = null;
    currentMapMode = 'view';
    $('placementBanner').classList.add('hidden');
    $('mapModeIndicator').textContent = "Mode: Consultation";
    $('mapModeIndicator').style.background = "rgba(34,197,94,0.1)";
    $('mapModeIndicator').style.borderColor = "rgba(34,197,94,0.2)";
    $('mapModeIndicator').style.color = "var(--accent)";
}

function confirmPlacement(lat, lng) {
    if (!placementLampID) return;
    if (confirm(`Confirmer la position (${lat}, ${lng}) pour le lampadaire #${placementLampID} ?`)) {
        fetch(`/api/lampadaires/${placementLampID}/location`, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({latitude: parseFloat(lat), longitude: parseFloat(lng)})
        }).then(r => r.json()).then(d => {
            alert("Emplacement enregistré !");
            cancelPlacementMode();
            window.location.reload();
        });
    }
}

// ── Generic modal helper ────────────────────────────────────────────
function closeModal(id) { $(id).classList.add('hidden'); }

// ── Leaflet layer groups ────────────────────────────────────────────
let layerGroups = {};
function getLayerGroup(name) {
    if (!layerGroups[name]) layerGroups[name] = L.layerGroup().addTo(map);
    return layerGroups[name];
}
function toggleLayer(name) {
    const cb = document.getElementById('layer' + name.charAt(0).toUpperCase() + name.slice(1));
    if (!layerGroups[name]) return;
    if (cb && cb.checked) { map.addLayer(layerGroups[name]); }
    else { map.removeLayer(layerGroups[name]); }
}

// ── Basestations ────────────────────────────────────────────────────
let basestations = [];
async function loadBasestations() {
    const res = await fetch('/api/basestations');
    basestations = await res.json() || [];
    renderBasestationCards();
    addBasestationMarkers();
}

function renderBasestationCards() {
    const el = $('basestationList');
    if (!el) return;
    if (!basestations.length) { el.innerHTML = '<p style="color:var(--text-dim)">Aucune basestation enregistrée.</p>'; return; }
    el.innerHTML = basestations.map(bs => `
        <div class="panel" style="min-width:220px;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
                <strong>${esc(bs.reference)}</strong>
                <span class="badge ${bs.status}">${bs.status}</span>
            </div>
            <div style="font-size:13px;color:var(--text-dim);">${esc(bs.name || '')} — ${esc(bs.zone || '')}</div>
            <div style="font-size:12px;margin-top:6px;display:grid;grid-template-columns:1fr 1fr;gap:4px;">
                <span>Réseau: <b>${esc(bs.network_type)}</b></span>
                <span>Backhaul: <b>${esc(bs.primary_backhaul)}</b></span>
                <span>Connectés: <b>${bs.connected_nodes_count}</b></span>
                <span>Signal: <b>${bs.signal_quality_avg.toFixed(0)}%</b></span>
            </div>
            <div style="display:flex;gap:6px;margin-top:10px;">
                <button class="btn btn-secondary btn-sm" onclick="simulateBasestationOffline(${bs.id})">📴 Sim. offline</button>
            </div>
        </div>`).join('');
}

function addBasestationMarkers() {
    const lg = getLayerGroup('basestations');
    lg.clearLayers();
    basestations.forEach(bs => {
        if (!bs.latitude || !bs.longitude) return;
        const icon = L.divIcon({
            className: `marker-basestation status-${bs.status}`,
            iconSize: [20, 20], iconAnchor: [10, 10]
        });
        L.marker([bs.latitude, bs.longitude], {icon})
            .bindPopup(`<b>📡 ${esc(bs.reference)}</b><br>${esc(bs.name||'')} — ${bs.status}<br>Réseau: ${bs.network_type}`)
            .addTo(lg);
    });
}

function openBasestationModal() { $('basestationModal').classList.remove('hidden'); }

async function saveBasestation() {
    const body = {
        reference: $('bs_reference').value.trim(),
        name: $('bs_name').value.trim(),
        zone: $('bs_zone').value.trim(),
        network_type: $('bs_network_type').value,
        primary_backhaul: $('bs_backhaul').value,
        address: $('bs_address').value.trim(),
        latitude: $('bs_lat').value ? parseFloat($('bs_lat').value) : null,
        longitude: $('bs_lng').value ? parseFloat($('bs_lng').value) : null,
    };
    if (!body.reference) { showToast('La référence est obligatoire', 'error'); return; }
    const res = await fetch('/api/basestations', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)});
    if (res.ok) { closeModal('basestationModal'); showToast('Basestation créée', 'success'); loadBasestations(); }
    else { const d = await res.json(); showToast(d.error || 'Erreur', 'error'); }
}

async function simulateBasestationOffline(id) {
    if (!confirm('Simuler la basestation hors ligne ?')) return;
    const res = await fetch(`/api/basestations/${id}/simulate-offline`, {method:'POST'});
    if (res.ok) { showToast('Basestation mise offline', 'success'); loadBasestations(); }
}

// ── Cabinets ────────────────────────────────────────────────────────
let cabinets = [];
async function loadCabinets() {
    const res = await fetch('/api/cabinets');
    cabinets = await res.json() || [];
    renderCabinetCards();
    addCabinetMarkers();
}

function renderCabinetCards() {
    const el = $('cabinetList');
    if (!el) return;
    if (!cabinets.length) { el.innerHTML = '<p style="color:var(--text-dim)">Aucune armoire enregistrée.</p>'; return; }
    el.innerHTML = cabinets.map(cab => `
        <div class="panel" style="min-width:220px;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
                <strong>${esc(cab.reference)}</strong>
                <span class="badge ${cab.status}">${cab.status}</span>
            </div>
            <div style="font-size:13px;color:var(--text-dim);">${esc(cab.name||'')} — ${esc(cab.zone||'')}</div>
            <div style="font-size:12px;margin-top:6px;display:grid;grid-template-columns:1fr 1fr;gap:4px;">
                <span>Porte: <b class="${cab.door_status==='open'?'wo-priority-urgent':''}">${cab.door_status}</b></span>
                <span>Alimentation: <b>${cab.power_status}</b></span>
            </div>
            <div style="display:flex;gap:6px;margin-top:10px;flex-wrap:wrap;">
                <button class="btn btn-secondary btn-sm" onclick="loadCabinetCircuits(${cab.id}, '${esc(cab.reference)}')">📋 Circuits</button>
                <button class="btn btn-warning btn-sm" onclick="simulateDoorOpen(${cab.id})">🚪 Porte ouverte</button>
                <button class="btn btn-danger btn-sm" onclick="simulatePowerFailure(${cab.id})">⚡ Panne alim.</button>
            </div>
        </div>`).join('');
}

function addCabinetMarkers() {
    const lg = getLayerGroup('cabinets');
    lg.clearLayers();
    cabinets.forEach(cab => {
        if (!cab.latitude || !cab.longitude) return;
        const icon = L.divIcon({
            className: `marker-cabinet status-${cab.status}${cab.door_status==='open'?' door-open':''}`,
            iconSize: [18, 18], iconAnchor: [9, 9]
        });
        L.marker([cab.latitude, cab.longitude], {icon})
            .bindPopup(`<b>🔌 ${esc(cab.reference)}</b><br>${esc(cab.name||'')} — ${cab.status}<br>Porte: ${cab.door_status}`)
            .addTo(lg);
    });
}

function openCabinetModal() { $('cabinetModal').classList.remove('hidden'); }

async function saveCabinet() {
    const body = {
        reference: $('cab_reference').value.trim(),
        name: $('cab_name').value.trim(),
        zone: $('cab_zone').value.trim(),
        address: $('cab_address').value.trim(),
        notes: $('cab_notes').value.trim(),
        latitude: $('cab_lat').value ? parseFloat($('cab_lat').value) : null,
        longitude: $('cab_lng').value ? parseFloat($('cab_lng').value) : null,
    };
    if (!body.reference) { showToast('La référence est obligatoire', 'error'); return; }
    const res = await fetch('/api/cabinets', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)});
    if (res.ok) { closeModal('cabinetModal'); showToast('Armoire créée', 'success'); loadCabinets(); }
    else { const d = await res.json(); showToast(d.error || 'Erreur', 'error'); }
}

async function loadCabinetCircuits(id, ref) {
    const res = await fetch(`/api/cabinets/${id}/circuits`);
    const circuits = await res.json() || [];
    const panel = $('cabinetCircuitsPanel');
    $('cabinetCircuitsTitle').textContent = `Circuits — Armoire ${ref}`;
    $('cabinetCircuitsList').innerHTML = circuits.length ? `
        <table class="data-table"><thead><tr><th>Nom</th><th>Phase</th><th>Circuit</th><th>Statut</th><th>Contacteur</th><th>Disjoncteur</th><th>Courant</th></tr></thead>
        <tbody>${circuits.map(cc => `<tr>
            <td>${esc(cc.name)}</td><td>${cc.phase}</td><td>${cc.circuit_number}</td>
            <td><span class="badge ${cc.status}">${cc.status}</span></td>
            <td>${cc.contactor_status}</td><td>${cc.breaker_status}</td>
            <td>${cc.measured_current ? cc.measured_current.toFixed(1)+' A' : '—'}</td>
        </tr>`).join('')}</tbody></table>` : '<p style="color:var(--text-dim)">Aucun circuit.</p>';
    panel.style.display = 'block';
    panel.scrollIntoView({behavior:'smooth'});
}

function closeCabinetCircuits() { $('cabinetCircuitsPanel').style.display = 'none'; }

async function simulateDoorOpen(id) {
    const res = await fetch(`/api/cabinets/${id}/simulate-door-open`, {method:'POST'});
    if (res.ok) { showToast('Porte ouverte simulée — alerte créée', 'success'); loadCabinets(); }
}

async function simulatePowerFailure(id) {
    if (!confirm('Simuler une coupure alimentation ?')) return;
    const res = await fetch(`/api/cabinets/${id}/simulate-power-failure`, {method:'POST'});
    if (res.ok) { showToast('Panne alimentation simulée — alerte critique créée', 'success'); loadCabinets(); }
}

// ── Controllers ─────────────────────────────────────────────────────
async function loadControllers() {
    const res = await fetch('/api/controllers');
    const controllers = await res.json() || [];
    const tbody = $('controllerList');
    if (!tbody) return;
    if (!controllers.length) { tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-dim);">Aucun contrôleur.</td></tr>'; return; }
    tbody.innerHTML = controllers.map(ctrl => `<tr>
        <td style="font-family:monospace;font-size:12px;">${esc(ctrl.controller_uid)}</td>
        <td>${esc(ctrl.type)}</td>
        <td>${ctrl.basestation_id ? '#'+ctrl.basestation_id : '—'}</td>
        <td>${ctrl.lampadaire_id ? '#'+ctrl.lampadaire_id : '—'}</td>
        <td><span class="badge ${ctrl.communication_status==='ok'?'online':ctrl.communication_status==='lost'?'offline':'maintenance'}">${ctrl.communication_status}</span></td>
        <td>${ctrl.signal_quality}%</td>
        <td><span class="badge ${ctrl.installation_status==='commissioned'?'online':ctrl.installation_status==='associated'?'maintenance':'unknown'}">${ctrl.installation_status}</span></td>
        <td>
            <button class="btn btn-secondary btn-sm" onclick="associateControllerModal(${ctrl.id})">🔗 Associer</button>
        </td>
    </tr>`).join('');
}

function openControllerModal() { $('controllerModal').classList.remove('hidden'); }

async function saveController() {
    const body = {
        controller_uid: $('ctrl_uid').value.trim(),
        serial_number: $('ctrl_serial').value.trim(),
        type: $('ctrl_type').value,
        firmware_version: $('ctrl_firmware').value.trim(),
        metering_enabled: true, dimming_enabled: true,
    };
    if (!body.controller_uid) { showToast('L\'UID est obligatoire', 'error'); return; }
    const res = await fetch('/api/controllers', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)});
    if (res.ok) { closeModal('controllerModal'); showToast('Contrôleur créé', 'success'); loadControllers(); }
    else { const d = await res.json(); showToast(d.error || 'Erreur', 'error'); }
}

async function associateControllerModal(ctrlId) {
    const lampId = prompt('ID du lampadaire à associer :');
    if (!lampId || isNaN(parseInt(lampId))) return;
    const res = await fetch(`/api/controllers/${ctrlId}/associate`, {
        method:'POST', headers:{'Content-Type':'application/json'},
        body: JSON.stringify({lampadaire_id: parseInt(lampId)})
    });
    if (res.ok) { showToast('Association réussie', 'success'); loadControllers(); }
    else { const d = await res.json(); showToast(d.error || 'Erreur', 'error'); }
}

// ── Work Orders ─────────────────────────────────────────────────────
async function loadWorkOrders() {
    const status = ($('woFilterStatus') || {}).value || '';
    const priority = ($('woFilterPriority') || {}).value || '';
    let url = '/api/workorders';
    const res = await fetch(url);
    let workOrders = await res.json() || [];
    if (status) workOrders = workOrders.filter(w => w.status === status);
    if (priority) workOrders = workOrders.filter(w => w.priority === priority);
    const tbody = $('workOrderList');
    if (!tbody) return;
    if (!workOrders.length) { tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-dim);">Aucun work order.</td></tr>'; return; }
    tbody.innerHTML = workOrders.map(wo => `<tr>
        <td>#${wo.id}</td>
        <td style="max-width:200px;">${esc(wo.title)}</td>
        <td><span class="wo-priority-${wo.priority}">${wo.priority.toUpperCase()}</span></td>
        <td><span class="badge ${woStatusClass(wo.status)}">${wo.status}</span></td>
        <td>${wo.crew_type}</td>
        <td>${wo.assigned_to ? '#'+wo.assigned_to : '—'}</td>
        <td style="font-size:12px;color:var(--text-dim);">${fmt(wo.created_at)}</td>
        <td style="display:flex;gap:4px;flex-wrap:wrap;">
            ${wo.status==='created'||wo.status==='assigned' ? `<button class="btn btn-secondary btn-sm" onclick="startWorkOrder(${wo.id})">▶ Démarrer</button>` : ''}
            ${wo.status==='in_progress' ? `<button class="btn btn-primary btn-sm" onclick="resolveWorkOrder(${wo.id})">✓ Résoudre</button>` : ''}
            ${wo.status==='resolved' ? `<button class="btn btn-secondary btn-sm" onclick="closeWorkOrder(${wo.id})">✕ Clôturer</button>` : ''}
        </td>
    </tr>`).join('');
}

function woStatusClass(s) {
    return {created:'unknown',assigned:'maintenance',in_progress:'warning',resolved:'online',closed:'applied',cancelled:'offline'}[s] || 'unknown';
}

function openWorkOrderModal(alertIds) {
    $('wo_alert_ids').value = JSON.stringify(alertIds || []);
    $('wo_title').value = '';
    $('wo_description').value = '';
    $('wo_cause').value = '';
    $('wo_alerts_info').textContent = alertIds && alertIds.length ? `Lié à ${alertIds.length} alerte(s): ${alertIds.join(', ')}` : '';
    $('workOrderModal').classList.remove('hidden');
}

async function saveWorkOrder() {
    const alertIds = JSON.parse($('wo_alert_ids').value || '[]');
    const body = {
        title: $('wo_title').value.trim(),
        description: $('wo_description').value.trim(),
        priority: $('wo_priority').value,
        crew_type: $('wo_crew_type').value,
        probable_cause: $('wo_cause').value.trim(),
        source_alert_ids: alertIds,
    };
    if (!body.title) { showToast('Le titre est obligatoire', 'error'); return; }
    const url = alertIds.length ? '/api/workorders/from-alerts' : '/api/workorders';
    const apiBody = alertIds.length ? {alert_ids: alertIds, title: body.title, priority: body.priority, crew_type: body.crew_type} : body;
    const res = await fetch(url, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(apiBody)});
    if (res.ok) { closeModal('workOrderModal'); showToast('Work order créé', 'success'); loadWorkOrders(); }
    else { const d = await res.json(); showToast(d.error || 'Erreur', 'error'); }
}

async function startWorkOrder(id) {
    const res = await fetch(`/api/workorders/${id}/start`, {method:'POST'});
    if (res.ok) { showToast('Work order démarré', 'success'); loadWorkOrders(); }
}

async function resolveWorkOrder(id) {
    const note = prompt('Note de résolution :') || '';
    const res = await fetch(`/api/workorders/${id}/resolve`, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({note})});
    if (res.ok) { showToast('Work order résolu', 'success'); loadWorkOrders(); }
}

async function closeWorkOrder(id) {
    const res = await fetch(`/api/workorders/${id}/close`, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({note:''})});
    if (res.ok) { showToast('Work order clôturé', 'success'); loadWorkOrders(); }
}

// ── Commissioning ───────────────────────────────────────────────────
async function loadCommissioning() {
    const res = await fetch('/api/lampadaires/missing-location');
    const all = await fetch('/api/lampadaires/' + (window.LAMPADAIRES && window.LAMPADAIRES.length ? '' : ''));
    // Use window.LAMPADAIRES for commissioning pipeline
    const lamps = window.LAMPADAIRES || [];
    renderCommissioningPipeline(lamps);
    renderCommissioningTable(lamps);
}

function renderCommissioningPipeline(lamps) {
    const el = $('commissioningPipeline');
    if (!el) return;
    const steps = ['discovered','located','configured','tested','commissioned','failed'];
    const labels = ['Découvert','Localisé','Configuré','Testé','Commissioning','Échec'];
    const colors = ['#3b82f6','#8b5cf6','#f59e0b','#06b6d4','#22c55e','#ef4444'];
    el.innerHTML = steps.map((s, i) => {
        const count = lamps.filter(l => l.commissioning_status === s).length;
        return `<div class="commission-step-card">
            <div style="font-size:11px;font-weight:600;color:${colors[i]};text-transform:uppercase;">${labels[i]}</div>
            <div class="step-count" style="color:${colors[i]};">${count}</div>
            <div class="step-label">${s}</div>
        </div>`;
    }).join('');
}

function renderCommissioningTable(lamps) {
    const tbody = $('commissioningList');
    if (!tbody) return;
    const filtered = lamps.filter(l => l.commissioning_status !== 'commissioned');
    if (!filtered.length) { tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--accent);">Tous les lampadaires sont commissionnés !</td></tr>'; return; }
    tbody.innerHTML = filtered.map(l => `<tr>
        <td>${esc(l.reference)}</td>
        <td>${esc(l.zone||'')}</td>
        <td>${l.commissioning_step || 0}</td>
        <td><span class="badge ${l.commissioning_status==='commissioned'?'online':l.commissioning_status==='failed'?'offline':'maintenance'}">${l.commissioning_status}</span></td>
        <td>${l.test_comm_status||'pending'}</td>
        <td>${l.test_dimming_status||'pending'}</td>
        <td style="display:flex;gap:4px;flex-wrap:wrap;">
            <button class="btn btn-secondary btn-sm" onclick="advanceCommissioning(${l.id})">→ Avancer</button>
            <button class="btn btn-primary btn-sm" onclick="validateCommissioning(${l.id})">✓ Valider</button>
            <button class="btn btn-danger btn-sm" onclick="failCommissioning(${l.id})">✕ Échec</button>
        </td>
    </tr>`).join('');
}

async function advanceCommissioning(id) {
    const res = await fetch(`/api/commissioning/${id}/advance`, {method:'POST'});
    const d = await res.json();
    if (res.ok) { showToast(`Étape ${d.step}: ${d.status}`, 'success'); window.location.reload(); }
    else showToast(d.error || 'Erreur', 'error');
}

async function validateCommissioning(id) {
    if (!confirm('Valider le commissioning complet de ce lampadaire ?')) return;
    const res = await fetch(`/api/commissioning/${id}/validate`, {method:'POST'});
    if (res.ok) { showToast('Commissioning validé !', 'success'); window.location.reload(); }
}

async function failCommissioning(id) {
    const notes = prompt('Motif de l\'échec :') || '';
    const res = await fetch(`/api/commissioning/${id}/fail`, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({notes})});
    if (res.ok) { showToast('Marqué en échec', 'success'); window.location.reload(); }
}
