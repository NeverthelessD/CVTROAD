/* CVTROAD — 앱 화면: 공통 · 변환 화면 · 작업 목록. made by. Nevertheless_D */
var App = {
  token: new URLSearchParams(location.search).get('t') || '',
  st: null,           // /api/state
  kinds: [],
  settings: {},
  files: [],          // 넣은 파일 (FileInfo)
  op: 'convert',      // convert | tool
  choice: {},         // 종류 → 고른 형식
  toolChoice: {},     // 종류 → 고른 도구
  opts: {},
  jobs: [],
  isMac: true
};

// ───────────────────────── 공통
function $(s, r) { return (r || document).querySelector(s); }
function $$(s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); }
function esc(s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
function api(path, body, opt) {
  var o = { method: body === undefined ? 'GET' : 'POST', headers: { 'x-token': App.token } };
  if (body !== undefined) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(body); }
  if (opt && opt.raw) { o.body = opt.raw; delete o.headers['Content-Type']; o.method = 'POST'; }
  return fetch(path, o).then(function (r) {
    return r.json().catch(function () { return { ok: false, error: '응답을 읽지 못했어요' }; }).then(function (j) {
      if (!r.ok && j && j.ok === undefined) j.ok = false;
      return j;
    });
  }).catch(function () { return { ok: false, error: '앱 엔진과 연결이 끊겼어요. CVTROAD를 다시 열어 주세요', offline: true }; });
}
function withToken(url) { return url + (url.indexOf('?') < 0 ? '?' : '&') + 't=' + encodeURIComponent(App.token); }
function humanSize(n) {
  if (!n && n !== 0) return '';
  if (n >= 1 << 30) return (n / (1 << 30)).toFixed(1) + 'GB';
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + 'MB';
  if (n >= 1 << 10) return Math.round(n / 1024) + 'KB';
  return n + 'B';
}
function kindOf(id) {
  for (var i = 0; i < App.kinds.length; i++) if (App.kinds[i].id === id) return App.kinds[i];
  return App.kinds[App.kinds.length - 1] || { id: 'other', label: '파일', color: '#8a94a3', targets: [], tools: [] };
}
var toastTimer = null;
function toast(msg, ms) {
  var t = $('#toast');
  t.textContent = msg;
  t.classList.add('on');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(function () { t.classList.remove('on'); }, ms || 2600);
}
function openModal(html, wide) {
  var m = $('#modal'), s = $('#sheet');
  s.className = 'sheet' + (wide ? ' wide' : '');
  s.innerHTML = html;
  m.hidden = false;
  var f = s.querySelector('[autofocus], button, input');
  if (f) setTimeout(function () { f.focus(); }, 30);
  return s;
}
function closeModal() { $('#modal').hidden = true; $('#sheet').innerHTML = ''; App.onModalClose && App.onModalClose(); App.onModalClose = null; }
document.addEventListener('keydown', function (e) { if (e.key === 'Escape' && !$('#modal').hidden) closeModal(); });
$('#modal').addEventListener('mousedown', function (e) { if (e.target.id === 'modal') closeModal(); });
function finderWord() { return App.isMac ? 'Finder' : '탐색기'; }

// ───────────────────────── 화면 모드
function applyTheme(t) {
  if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
  else document.documentElement.removeAttribute('data-theme');
  try { localStorage.setItem('cvt-theme', t || 'system'); } catch (e) {}
  var icon = t === 'light' ? 'sun' : t === 'dark' ? 'moon' : 'auto';
  $('#themeBtn').innerHTML = ART.icon(icon);
  $('#themeBtn').title = '화면 모드: ' + (t === 'light' ? '라이트' : t === 'dark' ? '다크' : 'macOS 설정 따르기') + ' (눌러서 바꾸기)';
  var fr = $('#ringFrame');
  if (fr && fr.contentWindow && fr.contentWindow.CVT) fr.contentWindow.CVT.theme(t);
}
$('#themeBtn').addEventListener('click', function () {
  var order = ['system', 'light', 'dark'];
  var next = order[(order.indexOf(App.settings.appearance || 'system') + 1) % 3];
  saveSettings({ appearance: next });
  applyTheme(next);
});

function saveSettings(patch) {
  for (var k in patch) App.settings[k] = patch[k];
  return api('/api/settings', patch).then(function (j) {
    if (j.ok && j.settings) App.settings = j.settings;
    return j;
  });
}

// ───────────────────────── 화면 이동
function route() {
  var v = (location.hash || '#convert').slice(1).split('?')[0];
  if (['convert', 'tools', 'helper', 'settings'].indexOf(v) < 0) v = 'convert';
  $$('.view').forEach(function (s) { s.hidden = s.id !== 'view-' + v; });
  $$('.nav a').forEach(function (a) { a.classList.toggle('on', a.getAttribute('data-view') === v); });
  App.view = v;
  // 작업 목록은 변환 · 도구 화면 아래에 함께 보여 줘요
  var js = $('#jobsSec');
  if (v === 'tools') $('#view-tools').appendChild(js);
  else if (v === 'convert') $('#view-convert').appendChild(js);
  if (v === 'helper' && window.renderHelper) renderHelper();
  if (v === 'settings' && window.renderSettings) renderSettings();
  if (v === 'tools' && window.renderTools) renderTools();
  updateJobsPill();
}
window.addEventListener('hashchange', route);

// ───────────────────────── 파일 넣기
function addPaths(paths) {
  paths = (paths || []).filter(Boolean);
  if (!paths.length) return Promise.resolve([]);
  return api('/api/info', { paths: paths }).then(function (j) {
    if (!j.ok) { toast(j.error || '파일을 읽지 못했어요'); return []; }
    return addInfos(j.files);
  });
}
function addInfos(list) {
  var have = {};
  App.files.forEach(function (f) { have[f.path] = 1; });
  var added = [];
  list.forEach(function (f) { if (!have[f.path] && !f.missing) { App.files.push(f); have[f.path] = 1; added.push(f); } });
  renderFiles();
  renderExits();
  return added;
}

function pickFiles(folder) {
  if (!App.isMac) { $('#fileInput').click(); return; }
  api('/api/choose-files', { folder: !!folder }).then(function (j) {
    if (j.ok) addPaths(j.paths);
    else if (j.error) toast(j.error);
  });
}
$('#pickFiles').addEventListener('click', function () { pickFiles(false); });
$('#pickFolder').addEventListener('click', function () { pickFiles(true); });
$('#addMore').addEventListener('click', function () { pickFiles(false); });
$('#clearFiles').addEventListener('click', function () { App.files = []; App.choice = {}; App.toolChoice = {}; renderFiles(); renderExits(); });
$('#fileInput').addEventListener('change', function (e) { takeDropped(e.target.files, []); e.target.value = ''; });

// 끌어다 놓은 파일: 진짜 위치를 찾아서(원본 옆에 저장) — 못 찾으면 앱으로 올려서
function takeDropped(fileList, items, onDone) {
  var files = Array.prototype.slice.call(fileList || []);
  var dirs = 0;
  if (items && items.length) {
    files = [];
    Array.prototype.forEach.call(items, function (it, i) {
      var en = it.webkitGetAsEntry && it.webkitGetAsEntry();
      if (en && en.isDirectory) dirs++;
      else if (fileList[i]) files.push(fileList[i]);
    });
  }
  if (dirs) toast('폴더는 ‘폴더 고르기’로 넣어 주세요');
  if (!files.length) return Promise.resolve([]);
  var req = files.map(function (f) { return { name: f.name, size: f.size, mtime: f.lastModified }; });
  return api('/api/resolve', { files: req }).then(function (j) {
    var paths = (j.paths || []);
    var found = [], up = [];
    files.forEach(function (f, i) { if (paths[i]) found.push(paths[i]); else up.push(f); });
    var p = found.length ? api('/api/info', { paths: found }).then(function (r) { return r.files || []; }) : Promise.resolve([]);
    return p.then(function (infos) {
      if (!up.length) return infos;
      toast('파일 ' + up.length + '개를 앱으로 옮기고 있어요…', 5000);
      return up.reduce(function (pr, f) {
        return pr.then(function (acc) {
          return api(withToken('/api/upload?name=' + encodeURIComponent(f.name) + '&mtime=' + f.lastModified), undefined, { raw: f }).then(function (r) {
            if (r.ok && r.file) acc.push(r.file); else toast(f.name + ': ' + (r.error || '옮기지 못했어요'));
            return acc;
          });
        });
      }, Promise.resolve(infos)).then(function (all) {
        toast('위치를 알 수 없는 파일은 결과를 ‘' + (App.settings.outDirLabel || '다운로드') + '’ 폴더에 저장해요', 4200);
        return all;
      });
    });
  }).then(function (infos) {
    if (onDone) return onDone(infos);
    return addInfos(infos);
  });
}

function wireDrop(el, onFiles) {
  var depth = 0;
  el.addEventListener('dragenter', function (e) { e.preventDefault(); depth++; el.classList.add('over'); });
  el.addEventListener('dragover', function (e) { e.preventDefault(); e.dataTransfer.dropEffect = 'copy'; });
  el.addEventListener('dragleave', function () { depth = Math.max(0, depth - 1); if (!depth) el.classList.remove('over'); });
  el.addEventListener('drop', function (e) {
    e.preventDefault(); depth = 0; el.classList.remove('over');
    takeDropped(e.dataTransfer.files, e.dataTransfer.items, onFiles);
  });
}
// 창 어디에 놓아도 브라우저가 파일을 열지 않게
window.addEventListener('dragover', function (e) { e.preventDefault(); });
window.addEventListener('drop', function (e) { e.preventDefault(); });
wireDrop($('#drop'));
$('#drop').addEventListener('mouseenter', function () { if (!App.files.length) ART.playHero(); });

// ───────────────────────── 파일 목록
function groups() {
  var by = {}, order = [];
  App.files.forEach(function (f) {
    if (!by[f.kind]) { by[f.kind] = []; order.push(f.kind); }
    by[f.kind].push(f);
  });
  var kOrder = App.kinds.map(function (k) { return k.id; });
  order.sort(function (a, b) { return kOrder.indexOf(a) - kOrder.indexOf(b); });
  return order.map(function (k) { return { kind: kindOf(k), files: by[k] }; });
}

function routeFor(kindID) {
  var k = kindOf(kindID);
  if (App.op === 'tool') {
    var tid = App.toolChoice[kindID];
    var t = (k.tools || []).filter(function (x) { return x.id === tid; })[0];
    return t ? '<span class="to tool">' + esc(t.label) + '</span>' : '<span class="to none">도구 고르기</span>';
  }
  var id = App.choice[kindID];
  var tg = (k.targets || []).filter(function (x) { return x.id === id; })[0];
  return tg ? '<span class="to">' + esc(tg.label) + '</span>' : '<span class="to none">형식 고르기</span>';
}

function renderFiles() {
  var has = App.files.length > 0;
  $('#dropEmpty').hidden = has;
  $('#files').hidden = !has;
  $('#filesTitle').textContent = '파일 ' + App.files.length + '개';
  var ul = $('#fileList');
  ul.innerHTML = App.files.map(function (f, i) {
    var k = kindOf(f.kind);
    var thumb = (f.kind === 'image' || f.kind === 'video' || f.kind === 'pdf') && !f.dir ? '<img alt="" loading="lazy" src="' + esc(withToken('/api/thumb?p=' + encodeURIComponent(f.path))) + '" onerror="this.remove()">' : '';
    var label = f.dir ? 'DIR' : (f.ext || '?');
    var meta = esc(k.label) + ' · ' + humanSize(f.size) + (f.uploaded ? ' · 앱으로 옮긴 파일' : '');
    return '<li class="file" style="--k:' + k.color + '">' +
      '<div class="thumb" style="--k:' + k.color + '">' + ART.fileBadge(label) + thumb + '</div>' +
      '<div style="min-width:0"><div class="file-name" title="' + esc(f.path) + '">' + esc(f.name) + '</div><div class="file-meta"><span class="kdot"></span>' + meta + '</div></div>' +
      '<div class="route">' + ART.icon('arrow') + routeFor(f.kind) + '</div>' +
      '<button class="x" type="button" data-rm="' + i + '" title="목록에서 빼기" aria-label="목록에서 빼기">' + ART.icon('x') + '</button></li>';
  }).join('');
  $$('[data-rm]', ul).forEach(function (b) {
    b.addEventListener('click', function () { App.files.splice(+b.getAttribute('data-rm'), 1); renderFiles(); renderExits(); });
  });
}

// ───────────────────────── 출구(형식 · 도구) 고르기
$$('#opSeg button').forEach(function (b) {
  b.addEventListener('click', function () {
    App.op = b.getAttribute('data-op');
    $$('#opSeg button').forEach(function (x) { x.classList.toggle('on', x === b); });
    renderFiles(); renderExits();
  });
});

function renderExits() {
  var box = $('#exitGroups');
  var gs = groups();
  if (!gs.length) {
    box.innerHTML = '<div class="empty-exits">파일을 넣으면 바꿀 수 있는 형식이 여기에 나와요</div>';
    $('#options').innerHTML = '';
    renderSaveTo();
    updateGo();
    return;
  }
  box.innerHTML = gs.map(function (g) {
    var k = g.kind;
    var list = App.op === 'tool' ? (k.tools || []) : (k.targets || []);
    var cur = App.op === 'tool' ? App.toolChoice[k.id] : App.choice[k.id];
    var plates = list.map(function (t) {
      if (App.op === 'tool') {
        return '<button type="button" class="plate tool' + (cur === t.id ? ' on' : '') + '" data-k="' + k.id + '" data-id="' + t.id + '">' + ART.icon(t.icon) +
          '<div><b>' + esc(t.label) + '</b><span>' + esc(t.sub) + '</span></div></button>';
      }
      return '<button type="button" class="plate' + (cur === t.id ? ' on' : '') + '" data-k="' + k.id + '" data-id="' + t.id + '"><b>' + esc(t.label) + '</b><span>' + esc(t.sub) + '</span></button>';
    }).join('');
    return '<div class="exit-group"><h3><span class="kdot" style="--k:' + k.color + '"></span>' + esc(k.label) + ' <span class="n">' + g.files.length + '개</span></h3>' +
      '<div class="plates' + (App.op === 'tool' ? ' tools' : '') + '">' + plates + '</div></div>';
  }).join('');
  $$('.plate', box).forEach(function (p) {
    p.addEventListener('click', function () {
      var k = p.getAttribute('data-k'), id = p.getAttribute('data-id');
      var map = App.op === 'tool' ? App.toolChoice : App.choice;
      map[k] = map[k] === id ? undefined : id;
      renderExits(); renderFiles();
    });
  });
  renderOptions();
  renderSaveTo();
  updateGo();
}

function segHTML(name, items, cur) {
  return '<div class="seg small" data-opt="' + name + '">' + items.map(function (it) {
    return '<button type="button" data-v="' + it[0] + '" class="' + (String(cur) === String(it[0]) ? 'on' : '') + '">' + it[1] + '</button>';
  }).join('') + '</div>';
}

function renderOptions() {
  var o = App.opts, c = App.choice, t = App.toolChoice, html = '';
  var conv = App.op === 'convert';
  var needImg = conv && ['jpg', 'heic', 'webp', 'avif'].indexOf(c.image) >= 0;
  var needAudio = conv && ((c.audio && ['mp3', 'm4a', 'ogg', 'opus'].indexOf(c.audio) >= 0) || ['mp3', 'm4a'].indexOf(c.video) >= 0);
  var needVideo = conv && ['mp4', 'mov', 'webm', 'mkv'].indexOf(c.video) >= 0;
  var needDpi = conv && ['png', 'jpg', 'tiff'].indexOf(c.pdf) >= 0;
  if (needImg) html += '<div class="opt"><label>이미지 화질 <span id="iqv">' + o.imageQuality + '</span></label><input type="range" min="50" max="100" step="1" value="' + o.imageQuality + '" id="iq"></div>';
  if (needAudio) {
    html += '<div class="opt"><div class="lab">음질</div>' + segHTML('audioQuality', [['128', '128k'], ['192', '192k'], ['320', '320k'], ['lossless', '무손실']], o.audioQuality) + '</div>';
    if (o.audioQuality === 'lossless' && (c.audio === 'mp3' || c.video === 'mp3')) html += '<p class="muted" style="font-size:12.5px">MP3에는 무손실이 없어서 가장 좋은 음질(320k)로 만들어요. 무손실은 M4A · FLAC · WAV를 골라 주세요.</p>';
  }
  if (needVideo) {
    html += '<div class="opt"><div class="lab">영상 화질</div>' + segHTML('videoQuality', [['high', '고화질'], ['normal', '보통'], ['small', '작게']], o.videoQuality) + '</div>';
    html += '<div class="opt"><div class="lab">영상 크기</div>' + segHTML('videoSize', [['same', '원본'], ['1080', '1080p'], ['720', '720p'], ['480', '480p']], o.videoSize) + '</div>';
  }
  if (needDpi) html += '<div class="opt"><div class="lab">그림 해상도</div>' + segHTML('pdfDpi', [[100, '100dpi'], [150, '150dpi'], [300, '300dpi']], o.pdfDpi) + '</div>';
  if (!conv && t.pdf === 'pages') html += '<p class="muted" style="font-size:12.5px">페이지 정리는 쪽 그림을 보면서 순서 · 회전 · 삭제를 정하는 창이 열려요.</p>';
  $('#options').innerHTML = html;
  var iq = $('#iq');
  if (iq) iq.addEventListener('input', function () { o.imageQuality = +iq.value; $('#iqv').textContent = iq.value; });
  $$('#options [data-opt]').forEach(function (s) {
    var name = s.getAttribute('data-opt');
    $$('button', s).forEach(function (b) {
      b.addEventListener('click', function () {
        var v = b.getAttribute('data-v');
        o[name] = name === 'pdfDpi' ? +v : v;
        renderOptions();
      });
    });
  });
}

function renderSaveTo() {
  var s = App.settings;
  var hasUpload = App.files.some(function (f) { return f.uploaded; });
  $('#saveTo').innerHTML = '<div class="lab">저장할 곳</div>' +
    '<label class="radio' + (s.outMode !== 'folder' ? ' on' : '') + '"><input type="radio" name="outMode" value="beside"' + (s.outMode !== 'folder' ? ' checked' : '') + '><div class="grow">원본 파일 옆<small>원본은 그대로 두고 같은 폴더에 새로 만들어요</small></div></label>' +
    '<label class="radio' + (s.outMode === 'folder' ? ' on' : '') + '"><input type="radio" name="outMode" value="folder"' + (s.outMode === 'folder' ? ' checked' : '') + '><div class="grow">기본 폴더<small title="' + esc(s.outDir) + '">' + esc(s.outDirLabel || s.outDir) + '</small></div><button type="button" class="link" id="chOut">바꾸기</button></label>' +
    (hasUpload ? '<p class="muted" style="font-size:12.5px">앱으로 옮긴 파일은 기본 폴더에 저장돼요.</p>' : '');
  $$('#saveTo input[name=outMode]').forEach(function (r) {
    r.addEventListener('change', function () { saveSettings({ outMode: r.value }).then(renderSaveTo); });
  });
  $('#chOut').addEventListener('click', function (e) { e.preventDefault(); chooseOutDir().then(renderSaveTo); });
}

function chooseOutDir() {
  return api('/api/choose-folder', { start: App.settings.outDir }).then(function (j) {
    if (j.ok && j.path) return saveSettings({ outDir: j.path, outMode: 'folder' }).then(function () { toast('‘' + (j.label || j.path) + '’ 폴더에 저장할게요'); });
    if (j.error) toast(j.error);
  });
}

function plannedRequests() {
  var reqs = [];
  groups().forEach(function (g) {
    var id = App.op === 'tool' ? App.toolChoice[g.kind.id] : App.choice[g.kind.id];
    if (!id) return;
    reqs.push({ op: App.op, to: id, files: g.files.map(function (f) { return f.path; }), kind: g.kind.id,
      opts: { imageQuality: App.opts.imageQuality, audioQuality: App.opts.audioQuality, videoQuality: App.opts.videoQuality, videoSize: App.opts.videoSize, pdfDpi: App.opts.pdfDpi } });
  });
  return reqs;
}

function updateGo() {
  var b = $('#goBtn');
  var reqs = plannedRequests();
  var n = reqs.reduce(function (a, r) { return a + r.files.length; }, 0);
  if (!App.files.length) { b.disabled = true; b.textContent = '파일을 넣어 주세요'; return; }
  if (!reqs.length) { b.disabled = true; b.textContent = App.op === 'tool' ? '도구를 골라 주세요' : '바꿀 형식을 골라 주세요'; return; }
  b.disabled = false;
  b.textContent = App.op === 'tool' ? n + '개 파일에 도구 쓰기' : n + '개 파일 바꾸기';
}

$('#goBtn').addEventListener('click', function () {
  var reqs = plannedRequests();
  var pages = reqs.filter(function (r) { return r.op === 'tool' && r.to === 'pages'; });
  if (pages.length) { openPageEditor(pages[0].files[0]); return; }
  startJobs(reqs).then(function (n) {
    if (n) {
      toast(n > 1 ? '작업 ' + n + '개를 시작했어요' : '작업을 시작했어요');
      App.files = []; App.choice = {}; App.toolChoice = {};
      renderFiles(); renderExits();
    }
  });
});

function startJobs(reqs) {
  return reqs.reduce(function (p, r) {
    return p.then(function (n) {
      return api('/api/jobs/start', r).then(function (j) {
        if (!j.ok) { toast(j.error || '시작하지 못했어요'); return n; }
        return n + 1;
      });
    });
  }, Promise.resolve(0)).then(function (n) { pollSoon(); return n; });
}

// ───────────────────────── 작업 목록
var pollTimer = null;
function pollSoon() { clearTimeout(pollTimer); pollTimer = setTimeout(poll, 150); }
function poll() {
  api('/api/poll').then(function (j) {
    if (j.offline) { scheduleNext(4000); return; }
    App.jobs = j.jobs || [];
    if (j.update) { App.st.update = j.update; renderVersion(); }
    if (j.tools) { App.st.tools = j.tools; if (window.onToolsState) onToolsState(); }
    if (j.build && App.st.build && j.build !== App.st.build) { location.reload(); return; }
    renderJobs();
    if (j.pending) takePending();
    var busy = App.jobs.some(function (x) { return x.state === 'run' || x.state === 'wait'; }) || (j.tools && j.tools.running) ||
      (j.update && (j.update.status === 'downloading' || j.update.status === 'installing'));
    scheduleNext(busy ? 700 : 2500);
  });
}
function scheduleNext(ms) { clearTimeout(pollTimer); pollTimer = setTimeout(poll, ms); }

function etaText(s) {
  if (!s || s < 1) return '';
  if (s < 60) return '약 ' + Math.ceil(s) + '초 남음';
  return '약 ' + Math.ceil(s / 60) + '분 남음';
}

function renderJobs() {
  var list = App.jobs;
  $('#jobsSec').hidden = !list.length;
  var ul = $('#jobList');
  ul.innerHTML = list.map(function (j) {
    var k = kindOf(j.kind);
    var r = j.result || {};
    var sub = '', acts = '';
    if (j.state === 'run') {
      sub = Math.round(j.progress * 100) + '%' + (j.label ? ' · ' + esc(j.label) : '') + (j.eta ? ' · ' + etaText(j.eta) : '');
      acts = '<button class="btn small ghost" data-cancel="' + j.id + '">취소</button>';
    } else if (j.state === 'wait') {
      sub = '기다리는 중';
      acts = '<button class="btn small ghost" data-cancel="' + j.id + '">취소</button>';
    } else if (j.state === 'cancel') {
      sub = '<span class="badge">취소했어요</span>';
    } else {
      var made = (r.outputs || []).length > 0 || !!r.text;
      var badge = j.state === 'done' && made ? '<span class="badge ok">✓ ' + esc(r.message || '다 됐어요') + '</span>' :
        j.state === 'done' ? '<span class="badge">' + esc(r.message || '할 일이 없었어요') + '</span>' : '<span class="badge err">' + esc(r.message || '바꾸지 못했어요') + '</span>';
      sub = badge;
      var extra = (r.errors || []).length && j.state === 'done' ? '<div class="job-sub err">' + (r.errors || []).map(esc).join('<br>') + '</div>' : '';
      if (j.state === 'error' && (r.errors || []).length > 1) extra = '<div class="job-sub err">' + r.errors.map(esc).join('<br>') + '</div>';
      var notes = (r.notes || []).concat(r.skipped || []).filter(function (n) { return n !== r.message; });
      if (notes.length) extra += '<div class="job-sub" style="white-space:normal">' + notes.map(esc).join('<br>') + '</div>';
      if (r.text) extra += '<div class="qr-text">' + esc(r.text) + '</div>';
      sub += extra;
      if ((r.outputs || []).length) {
        acts = '<button class="btn small" data-reveal="' + esc(r.outputs[0]) + '">' + ART.icon('reveal') + finderWord() + '에서 보기</button>' +
          (r.outputs.length === 1 ? '<button class="btn small ghost" data-open="' + esc(r.outputs[0]) + '">열기</button>' : '');
      }
      if (r.text) acts = '<button class="btn small" data-copy="' + esc(r.text) + '">' + ART.icon('copy') + '복사</button>' + (/^https?:\/\//.test(r.text) ? '<button class="btn small ghost" data-url="' + esc(r.text) + '">주소 열기</button>' : '');
    }
    var pct = Math.round((j.progress || 0) * 100);
    return '<li class="job ' + j.state + '" style="--k:' + k.color + '"><span class="bar-k"></span>' +
      '<div style="min-width:0"><div class="job-title">' + esc(j.title) + '</div><div class="job-sub">' + sub + '</div></div>' +
      '<div class="job-acts">' + acts + '</div>' +
      '<div class="progress"><i style="width:' + Math.max(pct, j.state === 'run' ? 3 : 0) + '%"></i></div></li>';
  }).join('');
  $$('[data-cancel]', ul).forEach(function (b) { b.onclick = function () { api('/api/jobs/cancel', { id: b.getAttribute('data-cancel') }).then(pollSoon); }; });
  $$('[data-reveal]', ul).forEach(function (b) { b.onclick = function () { openPath(b.getAttribute('data-reveal'), 'reveal'); }; });
  $$('[data-open]', ul).forEach(function (b) { b.onclick = function () { openPath(b.getAttribute('data-open'), 'open'); }; });
  $$('[data-copy]', ul).forEach(function (b) { b.onclick = function () { api('/api/copy', { text: b.getAttribute('data-copy') }); toast('복사했어요'); }; });
  $$('[data-url]', ul).forEach(function (b) { b.onclick = function () { window.open(b.getAttribute('data-url'), '_blank'); }; });
  updateJobsPill();
}
function openPath(p, how) {
  api('/api/open', { path: p, how: how }).then(function (j) { if (!j.ok) toast(j.error || '열지 못했어요'); });
}
$('#clearJobs').addEventListener('click', function () { api('/api/jobs/clear', {}).then(pollSoon); });

function updateJobsPill() {
  var n = App.jobs.filter(function (x) { return x.state === 'run' || x.state === 'wait'; }).length;
  var pill = $('#jobsPill');
  pill.hidden = !n || App.view === 'convert';
  pill.textContent = '작업 ' + n + '개 진행 중';
}
$('#jobsPill').addEventListener('click', function () { location.hash = '#convert'; });

// 도우미가 앱에 넘긴 일 (PDF 페이지 정리 · 사용법)
function takePending() {
  api('/api/pending').then(function (j) {
    var p = j.pending;
    if (!p) return;
    if (p.tool === 'pages' && p.files && p.files.length) { location.hash = '#tools'; openPageEditor(p.files[0]); }
    else if (p.tool === 'help') location.hash = '#helper';
  });
}
window.addEventListener('focus', function () { pollSoon(); });
