'use strict';
// Contest Dashboard page logic. Works for both the main dashboard and the full-screen map.

const isMapView = document.body.classList.contains('view-map');
let lastMapVersion = -1;
let summary = null;
let rateChart = null, countryChart = null;

onThemeToggle(() => {
  setTiles(true);
  if (land) land.setStyle(landStyle());
  if (summary) render(summary);
  lastMapVersion = -1; refreshMap();
});
startClock();

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
function render(data) {
  const s = data.summary;
  summary = data;
  const a = s.active;
  const title = data.title || (s.mycall ? s.mycall : 'Contest Dashboard');
  el('title').textContent = title;
  document.title = a ? `${title} · ${a.name}` : title;
  el('contest-name').textContent = a ? a.name : '';
  liveBadge(a, s.live);
  if (mapTilesMode !== data.map_tiles) { mapTilesMode = data.map_tiles; setTiles(true); }

  const T = s.totals, R = s.rates;
  const scoreHint = T.score_source === 'logger' ? 'from N1MM' : 'estimate: no score from N1MM yet';
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

  recentTable(el('recent'), s.recent, s.units, true);

  bandModeTable(el('bandmode'), s.bands || [], s.modes || [], s.band_mode, s.band_mults);

  // operators
  el('ops').innerHTML = '<tr><th>Operator</th><th class="r">QSOs</th><th class="r">Last hour</th><th class="r">Run %</th><th class="r">Mults</th><th class="r">Share</th></tr>' +
    (s.operators || []).map(o => `<tr><td class="call"><a class="oplink" href="${q('operator.html', { op: o.name })}">${esc(o.name)}</a></td><td class="r">${fmt(o.qsos)}</td><td class="r">${fmt(o.last60)}</td>
      <td class="r">${o.qsos ? Math.round(100 * o.run / o.qsos) : 0}%</td><td class="r">${fmt(o.mults)}</td>
      <td class="r">${T.qsos ? Math.round(100 * o.qsos / T.qsos) : 0}%</td></tr>`).join('');
  el('ops-link').href = el('ops-nav').href = q('operators.html');

  renderRateChart(s);
  countryChart = countryBars(countryChart, el('country-chart'), s.top_countries);
  hourlyTable(el('hourly'), s.hourly, s.bands || []);
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
          legend: legendOpts,
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

// ---- data loading ----
async function refresh() {
  const r = await fetch(q('api/summary'));
  render(await r.json());
  await refreshMap();
}

if (el('contest-pick')) el('contest-pick').onchange = e => {
  contest = e.target.value;
  const u = new URL(location.href);
  if (contest) u.searchParams.set('contest', contest); else u.searchParams.delete('contest');
  history.replaceState(null, '', u);
  lastMapVersion = -1;
  update();
};
if (el('dash-link')) el('dash-link').href = q('./');

initMap();
setTiles(true);
const update = followUpdates(refresh);
