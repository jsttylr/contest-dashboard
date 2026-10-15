'use strict';
// Contest Dashboard page logic. Works for both the main dashboard and the full-screen map.

const isMapView = document.body.classList.contains('view-map');
const params = new URLSearchParams(location.search);
let contest = params.get('contest') || '';
let lastMapVersion = -1;
let summary = null;
let rateChart = null, countryChart = null;

const BANDS = ['160m', '80m', '40m', '20m', '15m', '10m', '6m'];
function bandVar(b) {
  const i = BANDS.indexOf(b);
  return i < 0 ? '--bother' : '--b' + b.replace('m', '');
}
function css(v) { return getComputedStyle(document.documentElement).getPropertyValue(v).trim(); }
function bandColor(b) { return css(bandVar(b)); }
function el(id) { return document.getElementById(id); }
function esc(s) { return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }
function fmt(n) { return Number(n || 0).toLocaleString(); }
function hhmm(t) { const d = new Date(t); return d.toISOString().slice(11, 16); }
function modeName(m) { return { CW: 'CW', PH: 'Phone', DG: 'Digital' }[m] || m || '?'; }
function q(path) { return contest ? path + '?contest=' + encodeURIComponent(contest) : path; }

// ---- theme ----
el('theme').onclick = () => {
  const cur = document.documentElement.dataset.theme ||
    (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
  const next = cur === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = next;
  try { localStorage.setItem('theme', next); } catch (e) { }
  setTiles(true);
  if (land) land.setStyle(landStyle());
  if (summary) render(summary);
  lastMapVersion = -1; refreshMap();
};
if (!document.documentElement.dataset.theme) document.documentElement.dataset.theme = 'dark';

setInterval(() => { el('clock').textContent = new Date().toISOString().slice(11, 19) + ' UTC'; }, 1000);

// ---- map ----
let map, tiles, dots, lines, homeMarker, land, mapTilesMode = 'online';
const landStyle = () => ({ color: css('--border'), weight: 0.6, fillColor: css('--land'), fillOpacity: 1 });
function initMap() {
  map = L.map('map', { preferCanvas: true, worldCopyJump: true, zoomSnap: 0.5, minZoom: 1, zoomControl: false }).setView([30, -40], 2);
  L.control.zoom({ position: isMapView ? 'bottomleft' : 'topleft' }).addTo(map);
  fetch('vendor/land.json').then(r => r.json()).then(g => {
    // Copies either side of the date line so the outline keeps going when the map is panned.
    const shift = dx => g.coordinates.map(p => p.map(r => r.map(([x, y]) => [x + dx, y])));
    g = { type: 'MultiPolygon', coordinates: [].concat(shift(-360), g.coordinates, shift(360)) };
    land = L.geoJSON(g, { style: landStyle, interactive: false }).addTo(map).bringToBack();
  });
  lines = L.layerGroup().addTo(map);
  dots = L.layerGroup().addTo(map);
}
function setTiles(force) {
  if (!map) return;
  if (tiles && !force) return;
  if (tiles) { map.removeLayer(tiles); tiles = null; }
  if (mapTilesMode !== 'online') return;
  const dark = document.documentElement.dataset.theme !== 'light';
  tiles = L.tileLayer('https://{s}.basemaps.cartocdn.com/' + (dark ? 'dark_all' : 'light_all') + '/{z}/{x}/{y}{r}.png', {
    subdomains: 'abcd', maxZoom: 10,
    attribution: '&copy; OpenStreetMap contributors &copy; CARTO'
  }).addTo(map);
}

// Great-circle path between two points, split where it crosses the date line.
function greatCircle(a, b, n = 48) {
  const rad = Math.PI / 180, [la1, lo1] = [a[0] * rad, a[1] * rad], [la2, lo2] = [b[0] * rad, b[1] * rad];
  const d = 2 * Math.asin(Math.sqrt(Math.sin((la2 - la1) / 2) ** 2 + Math.cos(la1) * Math.cos(la2) * Math.sin((lo2 - lo1) / 2) ** 2));
  if (d === 0) return [[a, b]];
  const segs = [[]];
  let prevLon = null;
  for (let i = 0; i <= n; i++) {
    const f = i / n, A = Math.sin((1 - f) * d) / Math.sin(d), B = Math.sin(f * d) / Math.sin(d);
    const x = A * Math.cos(la1) * Math.cos(lo1) + B * Math.cos(la2) * Math.cos(lo2);
    const y = A * Math.cos(la1) * Math.sin(lo1) + B * Math.cos(la2) * Math.sin(lo2);
    const z = A * Math.sin(la1) + B * Math.sin(la2);
    const lat = Math.atan2(z, Math.sqrt(x * x + y * y)) / rad, lon = Math.atan2(y, x) / rad;
    if (prevLon !== null && Math.abs(lon - prevLon) > 180) segs.push([]);
    segs[segs.length - 1].push([lat, lon]);
    prevLon = lon;
  }
  return segs;
}

async function refreshMap() {
  if (!map) return;
  let md;
  try { md = await (await fetch(q('api/map'))).json(); } catch (e) { return; }
  if (md.version === lastMapVersion) return;
  const first = lastMapVersion === -1;
  lastMapVersion = md.version;
  dots.clearLayers(); lines.clearLayers();
  const surface = css('--surface-1');
  const pts = md.points.slice().sort((a, b) => a.t - b.t);
  const newest = pts.slice(-25);
  if (md.home) {
    newest.forEach((p, i) => {
      greatCircle(md.home, [p.lat, p.lon]).forEach(seg => {
        L.polyline(seg, { color: bandColor(p.band), weight: 1.5, opacity: 0.25 + 0.6 * (i / newest.length), interactive: false }).addTo(lines);
      });
    });
  }
  pts.forEach((p, i) => {
    const isNew = i >= pts.length - 3;
    L.circleMarker([p.lat, p.lon], {
      radius: isNew ? 7 : 4, color: surface, weight: isNew ? 2 : 1,
      fillColor: bandColor(p.band), fillOpacity: 0.9
    }).bindTooltip(`<b>${esc(p.call)}</b> ${esc(p.band)} ${esc(modeName(p.mode))}<br>${esc(p.where)} · ${hhmm(p.t * 1000)} UTC`, { direction: 'top' })
      .addTo(dots);
  });
  if (md.home) {
    if (homeMarker) homeMarker.remove();
    homeMarker = L.circleMarker(md.home, { radius: 7, color: css('--text-primary'), weight: 2, fillColor: css('--accent'), fillOpacity: 1 })
      .bindTooltip('Home station', { direction: 'top' }).addTo(map);
    if (first) map.setView(md.home, isMapView ? 2.5 : 1.5);
  }
  const seen = [...new Set(pts.map(p => p.band))].sort((a, b) => bandSort(a) - bandSort(b));
  el('map-legend').innerHTML = seen.map(b => `<span><i style="background:${bandColor(b)}"></i>${esc(b || '?')}</span>`).join('');
  if (el('map-note')) el('map-note').textContent = `${fmt(pts.length)} contacts placed${md.home ? ', lines for the last ' + newest.length : ''}`;
}
const ALLBANDS = ['160m', '80m', '60m', '40m', '30m', '20m', '17m', '15m', '12m', '10m', '6m', '4m', '2m', '1.25m', '70cm', '33cm', '23cm', ''];
function bandSort(b) { const i = ALLBANDS.indexOf(b); return i < 0 ? 99 : i; }

// ---- dashboard ----
function tile(label, value, hint, hero) {
  return `<div class="tile${hero ? ' hero' : ''}"><div class="label">${esc(label)}</div><div class="value">${value}</div>${hint ? `<div class="hint">${hint}</div>` : '<div class="hint">&nbsp;</div>'}</div>`;
}

function render(data) {
  const s = data.summary;
  summary = data;
  const a = s.active;
  const title = data.title || (s.mycall ? s.mycall : 'Contest Dashboard');
  el('title').textContent = title;
  document.title = a ? `${title} · ${a.name}` : title;
  el('contest-name').textContent = a ? a.name : '';
  const live = el('live');
  live.classList.toggle('live', !!(a && s.live));
  el('live-text').textContent = !a ? 'Waiting for contacts' : s.live ? 'Live' : 'Finished';
  if (mapTilesMode !== data.map_tiles) { mapTilesMode = data.map_tiles; setTiles(true); }

  const T = s.totals, R = s.rates;
  const scoreHint = T.score_source === 'logger' ? 'from N1MM' : 'estimate (points × mults)';
  const tilesHtml = isMapView
    ? tile('Score', fmt(T.score)) + tile('QSOs', fmt(T.qsos)) + tile('Last hour', fmt(R.last60))
    : tile('Score', fmt(T.score), scoreHint, true) +
      tile('QSOs', fmt(T.qsos), R.hours ? `${R.hours.toFixed(1)} hours` : '') +
      tile('Mults', fmt(T.mults)) +
      tile('Points', fmt(T.points)) +
      tile('Last hour', fmt(R.last60), 'QSOs in 60 min') +
      tile('Last 10 min', fmt(R.last10 * 6), `${R.last10} QSOs, as an hourly rate`) +
      tile('Best hour', fmt(R.best60), 'any 60 minutes') +
      tile('Countries', fmt(T.countries), Object.entries(T.continents || {}).sort().map(([k, v]) => `${k} ${v}`).join(' · '));
  el('tiles').innerHTML = tilesHtml;

  if (isMapView) return;

  // contest picker
  const pick = el('contest-pick');
  const opts = ['<option value="">Latest contest</option>'].concat(s.contests.map(c =>
    `<option value="${esc(c.key)}">${esc(c.name)} · ${new Date(c.first).toISOString().slice(0, 10)} · ${fmt(c.qsos)} QSOs</option>`));
  if (pick.dataset.sig !== opts.join('')) { pick.innerHTML = opts.join(''); pick.dataset.sig = opts.join(''); }
  pick.value = contest;
  el('map-link').href = q('map.html');

  el('empty').hidden = !!a;

  // radios
  el('radios').innerHTML = (s.radios || []).length ? s.radios.map(r => `
    <div class="radio${r.tx ? ' tx' : ''}">
      <div class="meta"><b>${esc(r.station || 'Station')}</b> · radio ${r.radio}${r.tx ? ' <span class="tag tx">TX</span>' : ''}</div>
      <div class="freq">${(r.freq_khz || 0).toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })} <span class="muted" style="font-size:13px">kHz</span></div>
      <div class="meta"><span class="band-dot" style="background:${bandColor(r.band)}"></span>${esc(r.band || '?')} ${esc(r.mode)}
        · ${esc(r.op || '')} <span class="tag ${r.running ? 'run' : ''}">${r.running ? 'RUN' : 'S&amp;P'}</span>${r.antenna ? ' · ' + esc(r.antenna) : ''}</div>
    </div>`).join('') : '<div class="muted">No radio status from N1MM yet. Turn on "Radio" in N1MM\'s broadcast settings.</div>';

  // recent
  const unit = s.units === 'km' ? 'km' : 'mi';
  const dist = d => d ? fmt(Math.round(unit === 'km' ? d : d / 1.609344)) : '';
  el('recent').innerHTML = '<tr><th>UTC</th><th>Call</th><th>Band</th><th>Op</th><th>Country</th><th class="r">' + unit + '</th><th></th></tr>' +
    (s.recent || []).map(r => `<tr><td class="muted">${hhmm(r.time)}</td><td class="call">${esc(r.call)}</td>
      <td><span class="band-dot" style="background:${bandColor(r.band)}"></span>${esc(r.band)} ${esc(r.mode)}</td>
      <td>${esc(r.op)}</td><td>${esc(r.country)}</td><td class="r">${dist(r.dist)}</td>
      <td>${r.mult ? '<span class="tag mult">MULT</span> ' : ''}${r.run ? '<span class="tag run">RUN</span>' : ''}</td></tr>`).join('');

  // band x mode
  const modes = s.modes || [];
  let bm = '<tr><th>Band</th>' + modes.map(m => `<th class="r">${esc(modeName(m))}</th>`).join('') + '<th class="r">QSOs</th><th class="r">Mults</th></tr>';
  const colTot = {};
  (s.bands || []).forEach(b => {
    let tot = 0;
    bm += `<tr><td><span class="band-dot" style="background:${bandColor(b)}"></span>${esc(b || '?')}</td>` + modes.map(m => {
      const n = (s.band_mode[b] || {})[m] || 0; tot += n; colTot[m] = (colTot[m] || 0) + n;
      return `<td class="r">${n ? fmt(n) : '<span class="muted">·</span>'}</td>`;
    }).join('') + `<td class="r"><b>${fmt(tot)}</b></td><td class="r">${fmt(s.band_mults[b] || 0)}</td></tr>`;
  });
  if ((s.bands || []).length) bm += '<tr><td><b>Total</b></td>' + modes.map(m => `<td class="r"><b>${fmt(colTot[m])}</b></td>`).join('') + `<td class="r"><b>${fmt(T.qsos)}</b></td><td></td></tr>`;
  el('bandmode').innerHTML = bm;

  // operators
  el('ops').innerHTML = '<tr><th>Operator</th><th class="r">QSOs</th><th class="r">Last hour</th><th class="r">Run %</th><th class="r">Mults</th><th class="r">Share</th></tr>' +
    (s.operators || []).map(o => `<tr><td class="call">${esc(o.name)}</td><td class="r">${fmt(o.qsos)}</td><td class="r">${fmt(o.last60)}</td>
      <td class="r">${o.qsos ? Math.round(100 * o.run / o.qsos) : 0}%</td><td class="r">${fmt(o.mults)}</td>
      <td class="r">${T.qsos ? Math.round(100 * o.qsos / T.qsos) : 0}%</td></tr>`).join('');

  renderRateChart(s);
  renderCountryChart(s);
  renderHourly(s);
}

function chartDefaults() {
  Chart.defaults.color = css('--text-secondary');
  Chart.defaults.borderColor = css('--border');
  Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
}

function renderRateChart(s) {
  chartDefaults();
  const tl = s.timeline || [];
  el('rate-note').textContent = s.bucket_min ? `QSOs per ${s.bucket_min} minutes, by band` : '';
  const bands = (s.bands || []);
  const labels = tl.map(b => hhmm(b.t));
  const surface = css('--surface-1');
  const datasets = bands.map(b => ({
    label: b || '?', data: tl.map(x => x.by_band[b] || 0),
    backgroundColor: bandColor(b), borderColor: surface, borderWidth: { top: 1 }, borderRadius: 2, borderSkipped: 'bottom'
  }));
  if (!rateChart) {
    rateChart = new Chart(el('rate-chart'), {
      type: 'bar', data: { labels, datasets },
      options: {
        animation: false, maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        scales: {
          x: { stacked: true, grid: { display: false }, ticks: { autoSkip: true, maxRotation: 0 } },
          y: { stacked: true, beginAtZero: true, ticks: { precision: 0 }, grid: { color: css('--border') } }
        },
        plugins: {
          legend: { position: 'top', align: 'end', labels: { boxWidth: 10, boxHeight: 10, usePointStyle: true, pointStyle: 'circle' } },
          tooltip: { callbacks: { footer: items => 'Total ' + items.reduce((a, i) => a + i.parsed.y, 0) }, filter: i => i.parsed.y > 0 }
        }
      }
    });
  } else {
    rateChart.data.labels = labels; rateChart.data.datasets = datasets;
    rateChart.options.scales.y.grid.color = css('--border');
    rateChart.update('none');
  }
}

function renderCountryChart(s) {
  const tc = s.top_countries || [];
  const data = { labels: tc.map(c => c.name), datasets: [{ label: 'QSOs', data: tc.map(c => c.qsos), backgroundColor: css('--accent'), borderRadius: 4, borderSkipped: 'start', barThickness: 14 }] };
  if (!countryChart) {
    countryChart = new Chart(el('country-chart'), {
      type: 'bar', data,
      options: {
        indexAxis: 'y', animation: false, maintainAspectRatio: false,
        scales: { x: { beginAtZero: true, ticks: { precision: 0 }, grid: { color: css('--border') } }, y: { grid: { display: false } } },
        plugins: { legend: { display: false } }
      }
    });
  } else {
    countryChart.data = data;
    countryChart.update('none');
  }
}

function renderHourly(s) {
  const hours = s.hourly || [];
  const bands = s.bands || [];
  if (!hours.length) { el('hourly').innerHTML = ''; return; }
  const max = Math.max(1, ...hours.map(h => h.total));
  const accent = css('--accent');
  const cell = (n, m) => {
    if (!n) return '<td class="c muted">·</td>';
    const pct = Math.round(15 + 70 * n / m);
    return `<td class="c" style="background:color-mix(in srgb, ${accent} ${pct}%, transparent)">${n}</td>`;
  };
  let h = '<tr><th>Hour (UTC)</th>' + bands.map(b => `<th class="r">${esc(b)}</th>`).join('') + '<th class="r">Total</th><th class="r">Running total</th></tr>';
  let run = 0;
  hours.forEach(x => {
    run += x.total;
    const d = new Date(x.t);
    h += `<tr><td>${d.toISOString().slice(5, 10)} ${d.toISOString().slice(11, 13)}z</td>` +
      bands.map(b => cell(x.by_band[b] || 0, max)).join('') + cell(x.total, max) + `<td class="r muted">${fmt(run)}</td></tr>`;
  });
  el('hourly').innerHTML = h;
}

// ---- data loading ----
let loading = false, again = false;
async function refresh() {
  if (loading) { again = true; return; }
  loading = true;
  try {
    const r = await fetch(q('api/summary'));
    render(await r.json());
    await refreshMap();
  } catch (e) {
    el('live-text').textContent = 'Not connected';
    el('live').classList.remove('live');
  } finally {
    loading = false;
    if (again) { again = false; refresh(); }
  }
}

if (el('contest-pick')) el('contest-pick').onchange = e => {
  contest = e.target.value;
  const u = new URL(location.href);
  if (contest) u.searchParams.set('contest', contest); else u.searchParams.delete('contest');
  history.replaceState(null, '', u);
  lastMapVersion = -1;
  refresh();
};
if (el('dash-link')) el('dash-link').href = contest ? './?contest=' + encodeURIComponent(contest) : './';

initMap();
setTiles(true);
refresh();
// The server pushes a note whenever anything changes; fall back to polling if that fails.
try {
  const es = new EventSource('api/events');
  es.onmessage = () => refresh();
  es.onerror = () => { el('live-text').textContent = 'Reconnecting…'; };
} catch (e) {
  setInterval(refresh, 5000);
}
