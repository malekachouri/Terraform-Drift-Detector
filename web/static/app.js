const API = '/api/v1';

async function fetchJSON(url, opts) {
  const res = await fetch(url, opts);
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  Object.entries(attrs).forEach(([k, v]) => {
    if (k === 'className') node.className = v;
    else if (k === 'text') node.textContent = v;
    else if (k.startsWith('on')) node.addEventListener(k.slice(2).toLowerCase(), v);
    else node.setAttribute(k, v);
  });
  children.forEach(c => node.append(c));
  return node;
}

async function loadWorkspaces() {
  const container = document.getElementById('workspaces');
  try {
    const workspaces = await fetchJSON(`${API}/workspaces`);
    container.innerHTML = '';
    if (!workspaces.length) {
      container.textContent = 'No workspaces configured.';
      return;
    }
    workspaces.forEach(ws => {
      const card = el('div', { className: 'card' }, [
        el('h3', { text: ws.name }),
        el('div', { className: 'meta', text: `Provider: ${ws.provider} · Regions: ${(ws.regions || []).join(', ') || 'default'}` }),
        el('button', { text: 'Run Scan', onClick: () => triggerScan(ws.id) }),
      ]);
      container.append(card);
    });
  } catch (e) {
    container.textContent = `Error: ${e.message}`;
  }
}

async function triggerScan(workspaceId) {
  try {
    const report = await fetchJSON(`${API}/workspaces/${workspaceId}/scans`, { method: 'POST' });
    await loadScans();
    showReport(report);
  } catch (e) {
    alert(`Scan failed: ${e.message}`);
  }
}

async function loadScans() {
  const container = document.getElementById('scans');
  try {
    const all = await fetchJSON(`${API}/scans?limit=20`);

    if (!all.length) {
      container.textContent = 'No scans yet.';
      return;
    }

    const table = el('table');
    table.append(el('thead', {}, [el('tr', {}, [
      el('th', { text: 'Scan ID' }),
      el('th', { text: 'Workspace' }),
      el('th', { text: 'Status' }),
      el('th', { text: 'Findings' }),
      el('th', { text: 'Started' }),
    ])]));

    const tbody = el('tbody');
    all.forEach(scan => {
      const link = el('a', { className: 'link', text: scan.scan_id.slice(0, 8) + '…' });
      link.addEventListener('click', () => showReportById(scan.scan_id));
      tbody.append(el('tr', {}, [
        el('td', {}, [link]),
        el('td', { text: scan.workspace || '—' }),
        el('td', {}, [el('span', { className: `badge ${scan.status}`, text: scan.status })]),
        el('td', { text: String(scan.summary?.total_findings ?? 0) }),
        el('td', { text: new Date(scan.started_at).toLocaleString() }),
      ]));
    });
    table.append(tbody);
    container.innerHTML = '';
    container.append(table);
  } catch (e) {
    container.textContent = `Error: ${e.message}`;
  }
}

async function showReportById(scanId) {
  const report = await fetchJSON(`${API}/scans/${scanId}`);
  showReport(report);
}

function showReport(report) {
  document.getElementById('report-panel').hidden = false;
  document.getElementById('report-scan-id').textContent = report.scan_id;

  const summary = document.getElementById('summary');
  summary.innerHTML = '';
  const stats = [
    ['Resources', report.summary?.total_resources],
    ['Missing', report.summary?.missing_in_cloud],
    ['Extra', report.summary?.extra_in_cloud],
    ['Attributes', report.summary?.attribute_changes],
    ['Tags', report.summary?.tag_changes],
    ['Total', report.summary?.total_findings],
  ];
  stats.forEach(([label, value]) => {
    summary.append(el('div', { className: 'stat' }, [
      el('div', { className: 'value', text: String(value ?? 0) }),
      el('div', { className: 'label', text: label }),
    ]));
  });

  const notes = document.getElementById('notes');
  notes.innerHTML = '';
  (report.errors || []).forEach(msg => notes.append(el('li', { text: msg })));

  const findingsEl = document.getElementById('findings');
  findingsEl.innerHTML = '';
  if (!report.findings?.length) {
    findingsEl.textContent = report.status === 'completed'
      ? 'No drift detected. Cloud matches Terraform state.'
      : `Scan ${report.status}.`;
    return;
  }

  const table = el('table');
  table.append(el('thead', {}, [el('tr', {}, [
    el('th', { text: 'Kind' }),
    el('th', { text: 'Severity' }),
    el('th', { text: 'Resource' }),
    el('th', { text: 'Field' }),
    el('th', { text: 'Expected' }),
    el('th', { text: 'Actual' }),
  ])]));
  const tbody = el('tbody');
  report.findings.forEach(f => {
    tbody.append(el('tr', {}, [
      el('td', { text: f.kind }),
      el('td', { className: `severity-${f.severity}`, text: f.severity }),
      el('td', { text: f.address || `${f.resource_type}.${f.resource_name || f.resource_id}` }),
      el('td', { text: f.field || '—' }),
      el('td', { text: f.expected != null ? JSON.stringify(f.expected) : '—' }),
      el('td', { text: f.actual != null ? JSON.stringify(f.actual) : '—' }),
    ]));
  });
  table.append(tbody);
  findingsEl.append(table);
}

loadWorkspaces();
loadScans();
