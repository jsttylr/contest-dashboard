'use strict';
// One operator's details.

const op = (params.get('op') || '').toUpperCase();
let runspChart = null, countryChart = null, lastData = null;

function render(d) {
  lastData = d;
  const s = d.stats;
  el('op-name').textContent = op || 'Operator';
  document.title = `${op} · Operator`;
  el('contest-name').textContent = (d.active ? d.active.name : '') + ((s.stations || []).length ? ' · at ' + s.stations.join(', ') : '');
  liveBadge(d.active, d.active && Date.now() - new Date(d.active.last) < 6 * 3600e3);
  el('empty').hidden = s.qsos > 0;

  el('tiles').innerHTML =
    tile('QSOs', fmt(s.qsos), `${Math.round(s.share)}% of the station's`, true) +
    tile('Points', fmt(s.points)) +
    tile('Mults', fmt(s.mults), 'new multipliers worked') +
    tile('Run rate', fmt(Math.round(s.run_rate)), `${fmt(s.run_qsos)} QSOs in ${hours(s.run_hours)} running`) +
    tile('S&P rate', fmt(Math.round(s.sp_rate)), `${fmt(s.sp_qsos)} QSOs in ${hours(s.sp_hours)} S&P`) +
    tile('Average rate', fmt(Math.round(s.avg_rate)), `over ${hours(s.on_hours)} on the air`) +
    tile('Best hour', fmt(s.best60), 'any 60 minutes') +
    tile('Best 10 min', fmt(s.best10 * 6), `${s.best10} QSOs, as an hourly rate`) +
    tile('Last hour', fmt(s.last60)) +
    tile('Countries', fmt(s.countries));

  const tl = d.timeline || [];
  el('rate-note').textContent = d.bucket_min ? `QSOs per ${d.bucket_min} minutes, running or S&P` : '';
  const surface = css('--surface-1');
  runspChart = upsertChart(runspChart, el('runsp-chart'), {
    type: 'bar',
    data: {
      labels: tl.map(b => hhmm(b.t)),
      datasets: [
        { label: 'Run', data: tl.map(b => b.run), backgroundColor: seriesColor(0), borderColor: surface, borderWidth: { top: 1 }, borderRadius: 2, borderSkipped: 'bottom' },
        { label: 'S&P', data: tl.map(b => b.sp), backgroundColor: seriesColor(1), borderColor: surface, borderWidth: { top: 1 }, borderRadius: 2, borderSkipped: 'bottom' },
      ]
    },
    options: {
      animation: false, maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { stacked: true, grid: { display: false }, ticks: { autoSkip: true, maxRotation: 0 } },
        y: { stacked: true, beginAtZero: true, ticks: { precision: 0 }, grid: { color: css('--border') } }
      },
      plugins: { legend: legendOpts, tooltip: { filter: i => i.parsed.y > 0 } }
    }
  });

  bandModeTable(el('bandmode'), d.bands || [], d.modes || [], d.band_mode || {});
  recentTable(el('recent'), d.recent, d.units, false);
  countryChart = countryBars(countryChart, el('country-chart'), d.top_countries);
  hourlyTable(el('hourly'), d.hourly, d.bands || []);
}

async function loadPicker() {
  const ops = (await (await fetch(q('api/operators'))).json()).operators || [];
  const names = ops.map(o => o.name);
  if (op && !names.includes(op)) names.unshift(op);
  el('op-pick').innerHTML = names.map(n => `<option${n === op ? ' selected' : ''}>${esc(n)}</option>`).join('');
}
el('op-pick').onchange = e => { location.href = q('operator.html', { op: e.target.value }); };

onThemeToggle(() => { runspChart?.destroy(); countryChart?.destroy(); runspChart = countryChart = null; if (lastData) render(lastData); });
startClock();
el('dash-link').href = q('./');
el('ops-link').href = q('operators.html');
followUpdates(async () => {
  render(await (await fetch(q('api/operator', { op }))).json());
  await loadPicker();
});
