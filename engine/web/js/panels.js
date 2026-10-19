/* CVTROAD — 도우미 · 설정 · 업데이트 · 처음 안내 · 시작. made by. Nevertheless_D */

// ───────────────────────── 도우미 화면
var demo = { kind: 'image', mode: 'convert', ready: false, obs: null };

function ringWin() { var f = $('#ringFrame'); return f && f.contentWindow && f.contentWindow.CVT ? f.contentWindow : null; }

function demoShow() {
  var w = ringWin();
  if (!w) return;
  var k = kindOf(demo.kind);
  var items = (demo.mode === 'tools' ? k.tools : k.targets) || [];
  var names = { image: 'IMG_2041.HEIC', audio: '녹음 0412.wav', video: '여행.mov', pdf: '계약서.pdf' };
  w.CVT.show({ how: 'demo', mode: demo.mode, theme: App.settings.appearance, kind: { id: k.id, label: k.label, color: k.color }, count: 1, name: names[demo.kind],
    items: items.map(function (t) { return { id: t.id, label: t.label, sub: t.sub || '', icon: t.icon || '' }; }) });
}

function wireDemo() {
  var f = $('#ringFrame');
  if (f.getAttribute('data-wired')) { demoShow(); return; }
  f.setAttribute('data-wired', '1');
  f.addEventListener('load', function () {
    var w = ringWin();
    if (!w) return;
    demoShow();
    var last = 0;
    var check = function () {
      var t = f.contentDocument && f.contentDocument.title || '';
      var m = t.match(/^ev:(\d+):(.*)$/);
      if (!m || +m[1] <= last) return;
      last = +m[1];
      var ev = m[2];
      if (ev.indexOf('pick:') === 0) {
        var k = kindOf(demo.kind);
        var it = ((demo.mode === 'tools' ? k.tools : k.targets) || [])[+ev.slice(5)];
        w.CVT.done({ ok: true, message: it ? (demo.mode === 'tools' ? '‘' + it.label + '’ 도구로 새 파일을 만들어요' : it.label + ' 파일이 원본 옆에 생겨요') : '좋아요' });
        setTimeout(demoShow, 2200);
      } else if (ev === 'cancel') {
        demoShow();
      } else if (ev === 'mode') {
        setDemoMode(demo.mode === 'tools' ? 'convert' : 'tools');
      }
    };
    demo.obs = new MutationObserver(check);
    demo.obs.observe(f.contentDocument.querySelector('title'), { childList: true, characterData: true, subtree: true });
  });
  f.src = '/ring/ring.html#mode=ring&embed=1&theme=' + encodeURIComponent(App.settings.appearance || 'system');
  $$('#demoSeg button').forEach(function (b) {
    b.addEventListener('click', function () {
      demo.kind = b.getAttribute('data-k');
      $$('#demoSeg button').forEach(function (x) { x.classList.toggle('on', x === b); });
      demoShow();
    });
  });
  $$('#demoMode button').forEach(function (b) { b.addEventListener('click', function () { setDemoMode(b.getAttribute('data-m')); }); });
}
function setDemoMode(m) {
  demo.mode = m;
  $$('#demoMode button').forEach(function (x) { x.classList.toggle('on', x.getAttribute('data-m') === m); });
  demoShow();
}

function keys(list) { return list.map(function (k) { return k === '+' ? '<span>+</span>' : '<span class="kbd">' + esc(k) + '</span>'; }).join(''); }

function renderHelper() {
  wireDemo();
  var a = App.st.agent || {};
  var s = App.settings;
  var card = $('#agentCard');
  if (!a.supported) {
    card.innerHTML = '<div class="status-line"><span class="dot"></span><div class="grow"><b>이 컴퓨터에서는 도우미를 쓸 수 없어요</b><small>도우미는 macOS에서 동작해요. 앱 창에서는 모든 변환을 할 수 있어요.</small></div></div>';
  } else {
    var on = a.enabled, live = a.running || a.alive;
    card.innerHTML = '<div class="status-line"><span class="dot' + (on && live ? ' on' : '') + '"></span><div class="grow"><b>백그라운드 도우미 ' + (on ? (live ? '켜짐' : '켜는 중…') : '꺼짐') + '</b>' +
      '<small>' + (on ? '메뉴 막대에 CVTROAD 아이콘이 있어요. 로그인할 때 저절로 켜져요.' : '켜면 앱을 열지 않아도 Shift-드래그 · 단축키로 바꿀 수 있어요.') + '</small></div>' +
      '<label class="switch" style="padding:0"><input type="checkbox" id="agOn"' + (on ? ' checked' : '') + ' aria-label="백그라운드 도우미"></label></div>' +
      '<div class="toggles">' +
      '<label class="switch"><div class="grow">Shift-드래그 원형 메뉴<small>파일을 끌다가 Shift(⇧)를 누르면 커서 주변에 메뉴가 떠요</small></div><input type="checkbox" id="agShift"' + (s.shiftDrag ? ' checked' : '') + (on ? '' : ' disabled') + '></label>' +
      '<label class="switch"><div class="grow">떠 있는 변환 창<small>화면 구석의 둥근 표지판에 파일을 놓으면 메뉴가 떠요. 끌어서 옮길 수 있어요</small></div><input type="checkbox" id="agBubble"' + (s.bubble ? ' checked' : '') + (on ? '' : ' disabled') + '></label>' +
      '</div>' +
      '<div class="row gap-s" style="flex-wrap:wrap"><button class="btn small" id="agDemo" type="button"' + (on ? '' : ' disabled') + '>화면에서 체험해 보기</button>' +
      '<button class="btn small ghost" id="agFix" type="button"' + (on ? '' : ' disabled') + '>빠른 동작 · 단축키 다시 설치</button></div>';
    var act = function (action) {
      return api('/api/agent', { action: action }).then(function (j) { if (j.agent) App.st.agent = j.agent; renderHelper(); return j; });
    };
    $('#agOn').onchange = function (e) {
      act(e.target.checked ? 'on' : 'off').then(function () {
        toast(e.target.checked ? '도우미를 켰어요. 메뉴 막대를 확인해 보세요' : '도우미를 껐어요');
        App.settings.agent = e.target.checked;
        setTimeout(refreshAgent, 2500);
      });
    };
    $('#agShift').onchange = function (e) { saveSettings({ shiftDrag: e.target.checked }); };
    $('#agBubble').onchange = function (e) { saveSettings({ bubble: e.target.checked }).then(function () { toast(e.target.checked ? '화면 오른쪽 아래에 떠 있는 변환 창을 띄웠어요' : '떠 있는 변환 창을 숨겼어요'); }); };
    $('#agDemo').onclick = function () { act('demo').then(function () { toast('화면 가운데에 원형 메뉴가 떠요'); }); };
    $('#agFix').onclick = function () { act('services').then(function () { toast('빠른 동작과 단축키를 다시 설치했어요'); }); };
  }
  var sc = (a.quickActions || []);
  var menuKey = (sc[0] && sc[0].key) || '⌃⌥C', toolKey = (sc[1] && sc[1].key) || '⌃⌥T';
  $('#ways').innerHTML =
    '<li><div class="keys">' + keys(['⇧', '+', '끌기']) + '</div><div><b>변환 메뉴</b><span>Finder · 바탕 화면에서 파일을 끌다가 Shift를 누르세요. 원하는 표지판 위에서 손을 떼면 원본 옆에 새 파일이 생겨요. 가운데에서 떼면 취소예요.</span></div></li>' +
    '<li><div class="keys">' + keys(['⇧', '⌥', '+', '끌기']) + '</div><div><b>도구 메뉴</b><span>Option(⌥)까지 누르면 용량 줄이기 · 정보 지우기 · PDF 나누기 같은 도구가 떠요.</span></div></li>' +
    '<li><div class="keys">' + keys(menuKey.split('')) + '</div><div><b>고른 파일로 메뉴 띄우기</b><span>Finder에서 파일을 고르고 ' + esc(menuKey) + '(도구는 ' + esc(toolKey) + '). 마우스나 방향키 · Enter · 숫자로 골라요.</span></div></li>' +
    '<li><div class="keys">' + keys(['오른쪽 클릭']) + '</div><div><b>빠른 동작</b><span>파일을 오른쪽 클릭 → 빠른 동작 → ‘CVTROAD → JPG/PDF/MP3로 바꾸기’로 바로 바꿔요.</span></div></li>' +
    '<li><div class="keys"><span class="kbd" style="border-radius:50%;width:26px;height:26px;padding:0;line-height:24px;background:#0f6b4f;color:#fff;border-color:#0c5a42">◎</span></div><div><b>떠 있는 변환 창</b><span>켜 두면 화면 구석의 둥근 표지판에 파일을 놓기만 하면 돼요.</span></div></li>' +
    '<li><div class="keys">' + keys(['메뉴 막대']) + '</div><div><b>도우미 아이콘</b><span>잠시 멈추기, 원형 메뉴 체험, CVTROAD 열기, 도우미 끄기를 할 수 있어요.</span></div></li>';
}

function refreshAgent() {
  api('/api/state').then(function (j) { if (j.agent) { App.st.agent = j.agent; if (App.view === 'helper') renderHelper(); } });
}

// ───────────────────────── 설정 화면
function renderSettings() {
  var s = App.settings, t = App.st.tools || {}, u = App.st.update || {};
  var seg = function (name, items, cur) {
    return '<div class="seg small" data-set="' + name + '">' + items.map(function (it) {
      return '<button type="button" data-v="' + it[0] + '" class="' + (String(cur) === String(it[0]) ? 'on' : '') + '">' + it[1] + '</button>';
    }).join('') + '</div>';
  };
  var html = '';
  html += '<div class="card set-card"><h2>화면</h2>' +
    '<div class="set-row"><div class="lab">화면 모드<small>처음에는 macOS 설정을 따라가요</small></div>' + seg('appearance', [['system', '시스템'], ['light', '라이트'], ['dark', '다크']], s.appearance) + '</div></div>';
  html += '<div class="card set-card"><h2>저장</h2>' +
    '<div class="set-row"><div class="lab">저장할 곳</div>' + seg('outMode', [['beside', '원본 파일 옆'], ['folder', '기본 폴더']], s.outMode) + '</div>' +
    '<div class="set-row"><div class="lab">기본 폴더<small>위치를 알 수 없는 파일도 여기에 저장해요</small></div><div class="row gap-s"><span class="folder-chip" title="' + esc(s.outDir) + '">' + ART.icon('folder') + esc(s.outDirLabel || s.outDir) + '</span><button class="btn small" id="setOut" type="button">바꾸기</button></div></div>' +
    '<label class="switch"><div class="grow">끝나면 ' + finderWord() + '에서 보기<small>만든 파일을 바로 보여 줘요</small></div><input type="checkbox" data-sw="reveal"' + (s.reveal ? ' checked' : '') + '></label>' +
    '<label class="switch"><div class="grow">끝나면 알림<small>도우미로 바꿀 때와 오래 걸린 작업이 끝났을 때 알려 줘요</small></div><input type="checkbox" data-sw="notify"' + (s.notify ? ' checked' : '') + '></label></div>';
  html += '<div class="card set-card"><h2>기본 품질</h2><p class="muted" style="font-size:12.5px">도우미(Shift-드래그 · 단축키)로 바꿀 때 쓰는 값이에요. 앱 창에서는 바꿀 때마다 고를 수 있어요.</p>' +
    '<div class="opt"><label>이미지 화질 <span id="sIq">' + s.imageQuality + '</span></label><input type="range" min="50" max="100" value="' + s.imageQuality + '" id="setIq"></div>' +
    '<div class="set-row"><div class="lab">음질</div>' + seg('audioQuality', [['128', '128k'], ['192', '192k'], ['320', '320k'], ['lossless', '무손실']], s.audioQuality) + '</div>' +
    '<div class="set-row"><div class="lab">영상 화질</div>' + seg('videoQuality', [['high', '고화질'], ['normal', '보통'], ['small', '작게']], s.videoQuality) + '</div>' +
    '<div class="set-row"><div class="lab">영상 크기</div>' + seg('videoSize', [['same', '원본'], ['1080', '1080p'], ['720', '720p'], ['480', '480p']], s.videoSize) + '</div>' +
    '<div class="set-row"><div class="lab">PDF → 그림 해상도</div>' + seg('pdfDpi', [[100, '100'], [150, '150'], [300, '300dpi']], s.pdfDpi) + '</div></div>';
  html += '<div class="card set-card" id="engineCard"></div>';
  html += '<div class="card set-card" id="updCard"></div>';
  html += '<div class="card set-card"><h2>정보</h2><p><b style="font-family:var(--sign);letter-spacing:.04em">CVTROAD</b> v' + esc(App.st.build) + '</p>' +
    '<p class="muted">모든 변환은 이 Mac 안에서만 해요. 파일을 어디에도 보내지 않아요.</p>' +
    '<p class="muted" style="font-size:12.5px">함께 쓴 공개 소프트웨어: FFmpeg (LGPL/GPL) · pdfcpu (Apache 2.0) · PDF.js (Apache 2.0) · Pretendard · Overpass (SIL OFL)</p>' +
    '<p style="margin-top:6px">made by. Nevertheless_D</p></div>';
  $('#settingsBox').innerHTML = html;
  $$('#settingsBox [data-set]').forEach(function (g) {
    var name = g.getAttribute('data-set');
    $$('button', g).forEach(function (b) {
      b.onclick = function () {
        var v = b.getAttribute('data-v');
        var patch = {}; patch[name] = name === 'pdfDpi' ? +v : v;
        saveSettings(patch).then(function () {
          if (name === 'appearance') applyTheme(v);
          if (App.opts[name] !== undefined) App.opts[name] = patch[name];
          renderSettings(); renderSaveTo();
        });
      };
    });
  });
  $$('#settingsBox [data-sw]').forEach(function (c) { c.onchange = function () { var p = {}; p[c.getAttribute('data-sw')] = c.checked; saveSettings(p); }; });
  var iq = $('#setIq');
  iq.oninput = function () { $('#sIq').textContent = iq.value; };
  iq.onchange = function () { saveSettings({ imageQuality: +iq.value }); App.opts.imageQuality = +iq.value; };
  $('#setOut').onclick = function () { chooseOutDir().then(renderSettings); };
  onToolsState();
  renderUpdateCard($('#updCard'));
}

function onToolsState() {
  var box = $('#engineCard');
  if (!box) return;
  var t = App.st.tools || {};
  var body = '';
  if (t.ready) body = '<p>✓ 준비됐어요. 영상 · 오디오(MP3 포함) · WEBP · AVIF 변환에 써요.</p>';
  else if (t.running) {
    var pct = t.total ? Math.round(t.done / t.total * 100) : 0;
    body = '<div class="tool-state"><span>변환 엔진을 받고 있어요 (처음 한 번, 약 90MB)… ' + (pct ? pct + '%' : '') + '</span><div class="meter"><i style="width:' + pct + '%"></i></div></div>';
  } else if (t.error) body = '<p style="color:var(--stop)">' + esc(t.error) + '</p><button class="btn small" id="toolsRetry" type="button">다시 받기</button>';
  else body = '<p class="muted">영상 · 오디오를 처음 바꿀 때 자동으로 받아요.</p><button class="btn small" id="toolsRetry" type="button">지금 받기</button>';
  box.innerHTML = '<h2>변환 엔진</h2>' + body + '<p class="muted" style="font-size:12.5px">이미지 · 문서 · PDF · 압축은 macOS에 들어 있는 기능으로 바꿔서 엔진 없이도 돼요.</p>';
  var r = $('#toolsRetry');
  if (r) r.onclick = function () { api('/api/tools/install', {}).then(function (j) { if (j.offline) return; App.st.tools = j; onToolsState(); pollSoon(); }); };
}

// ───────────────────────── 버전 · 업데이트
function renderVersion() {
  var u = App.st.update || {};
  var b = $('#verBadge');
  b.textContent = 'v' + (App.st.build || '—');
  var avail = u.status === 'available' || u.status === 'ready';
  b.classList.toggle('new', avail);
  b.title = u.status === 'ready' ? '새 버전이 준비됐어요 — 눌러서 다시 시작' : avail ? '새 버전 ' + (u.latest && u.latest.version) + '이 있어요' : '버전 · 업데이트';
  var c = $('#updCard');
  if (c && App.view === 'settings') renderUpdateCard(c);
  var m = $('#updModalBody');
  if (m) renderUpdateCard(m, true);
}

function updateText(u) {
  switch (u.status) {
    case 'checking': return '새 버전을 확인하고 있어요…';
    case 'uptodate': return '✓ 최신 버전이에요';
    case 'available': return '새 버전 ' + u.latest.version + '이(가) 나왔어요';
    case 'downloading': return '새 버전을 받고 있어요… ' + Math.round((u.progress || 0) * 100) + '%';
    case 'installing': return '새 버전을 설치하고 있어요…';
    case 'ready': return '✓ ' + u.installed + ' 버전을 설치했어요. 다시 시작하면 바뀌어요';
    case 'error': return u.error || '확인하지 못했어요';
  }
  return '새 버전이 나오면 앱이 알아서 받아요';
}

function renderUpdateCard(box, inModal) {
  var u = App.st.update || {};
  var notes = u.latest && u.latest.notes && u.latest.notes.length && (u.status === 'available' || u.status === 'ready') ?
    '<ul class="notes">' + u.latest.notes.map(function (n) { return '<li>' + esc(n) + '</li>'; }).join('') + '</ul>' : '';
  var last = u.lastCheck ? '<small class="muted">마지막 확인: ' + new Date(u.lastCheck).toLocaleString('ko-KR') + '</small>' : '';
  box.innerHTML = (inModal ? '' : '<h2>업데이트</h2>') +
    '<div class="set-row"><div class="lab">지금 버전 <span style="font-family:var(--sign)">v' + esc(App.st.build) + '</span><small>' + esc(updateText(u)) + '</small>' + last + '</div></div>' + notes +
    '<label class="switch"><div class="grow">자동 업데이트<small>새 버전이 나오면 알아서 받아서 설치해요</small></div><input type="checkbox" id="updAuto"' + (u.autoUpdate ? ' checked' : '') + '></label>' +
    '<div class="row gap-s" style="flex-wrap:wrap">' +
    (u.status === 'available' ? '<button class="btn small primary" id="updInstall" type="button">지금 설치</button>' : '') +
    (u.status === 'ready' ? '<button class="btn small primary" id="updRestart" type="button">다시 시작</button>' : '') +
    '<button class="btn small" id="updCheck" type="button"' + (u.status === 'checking' || u.status === 'downloading' || u.status === 'installing' ? ' disabled' : '') + '>업데이트 확인</button>' +
    (u.canRollback ? '<button class="btn small ghost" id="updBack" type="button">이전 버전(' + esc(u.backupVersion) + ')으로 되돌리기</button>' : '') + '</div>';
  var q = function (id) { return box.querySelector('#' + id); };
  q('updAuto').onchange = function (e) { saveSettings({ autoUpdate: e.target.checked }).then(function () { App.st.update.autoUpdate = e.target.checked; }); };
  q('updCheck').onclick = function () {
    App.st.update.status = 'checking'; renderVersion();
    api('/api/update/check', {}).then(function (j) { App.st.update = j; renderVersion(); });
  };
  if (q('updInstall')) q('updInstall').onclick = function () { api('/api/update/install', {}).then(function () { App.st.update.status = 'downloading'; renderVersion(); pollSoon(); }); };
  if (q('updRestart')) q('updRestart').onclick = function () {
    api('/api/update/restart', {}).then(function (j) {
      if (!j.ok) { toast(j.error || '다시 시작하지 못했어요'); return; }
      toast('다시 시작하고 있어요…', 8000);
      setTimeout(function wait() { api('/api/ping').then(function (p) { if (p.ok) location.reload(); else setTimeout(wait, 800); }); }, 1800);
    });
  };
  if (q('updBack')) q('updBack').onclick = function () {
    if (!confirm('이전 버전으로 되돌릴까요? 자동 업데이트는 꺼져요.')) return;
    api('/api/update/rollback', {}).then(function (j) {
      if (!j.ok) { toast(j.error || '되돌리지 못했어요'); return; }
      api('/api/update/state').then(function (s) { App.st.update = s; renderVersion(); });
    });
  };
}

$('#verBadge').addEventListener('click', function () {
  openModal('<div class="sheet-head"><h2>버전 · 업데이트</h2><button class="x" type="button" id="updX" aria-label="닫기">' + ART.icon('x') + '</button></div><div id="updModalBody" class="set-card" style="padding:0"></div>');
  $('#updX').onclick = closeModal;
  renderUpdateCard($('#updModalBody'), true);
});

// ───────────────────────── 처음 안내
function welcome() {
  var s = openModal('<div class="sheet-head"><div><h2>CVTROAD에 오신 걸 환영해요</h2><p class="muted" style="margin-top:4px">파일을 다른 형식으로 바꾸는 길이에요. 모든 변환은 이 Mac 안에서만 해요.</p></div></div>' +
    '<div class="welcome-steps">' +
    '<div><span class="sign">PNG</span><b>앱 창에 끌어다 놓기</b><span class="muted" style="font-size:13px">파일을 넣고 바꿀 형식 표지판을 누르면 원본 옆에 새 파일이 생겨요.</span></div>' +
    '<div><span class="sign yellow">⇧ + 끌기</span><b>앱을 열지 않아도</b><span class="muted" style="font-size:13px">Finder에서 파일을 끌다가 Shift를 누르면 커서 주변에 메뉴가 떠요.</span></div>' +
    '<div><span class="sign blue">⌃⌥C</span><b>단축키 · 빠른 동작</b><span class="muted" style="font-size:13px">파일을 고르고 단축키를 누르거나, 오른쪽 클릭 → 빠른 동작을 써요.</span></div>' +
    '</div>' +
    '<p class="muted" style="font-size:12.5px">처음 도우미가 켜질 때 ‘백그라운드 항목이 추가됨’ 알림이 뜰 수 있어요. CVTROAD 도우미라서 괜찮아요.</p>' +
    '<div class="sheet-foot"><button class="btn" id="wHelp" type="button">도우미 살펴보기</button><button class="btn primary" id="wGo" type="button" autofocus>시작하기</button></div>');
  var done = function () { saveSettings({ seen: true }); closeModal(); };
  $('#wGo', s).onclick = done;
  $('#wHelp', s).onclick = function () { done(); location.hash = '#helper'; };
}

// ───────────────────────── 시작
function init() {
  $('#brandMark').innerHTML = ART.mark();
  $('#heroArt').innerHTML = ART.hero();
  api('/api/state').then(function (j) {
    if (!j || j.ok === false || !j.build) {
      document.body.innerHTML = '<p style="padding:40px;font:15px -apple-system">CVTROAD 엔진과 연결하지 못했어요. 앱을 다시 실행해 주세요.</p>';
      return;
    }
    App.st = j;
    App.kinds = j.formats || [];
    App.settings = j.settings || {};
    App.isMac = j.os === 'darwin';
    var s = App.settings;
    App.opts = { imageQuality: s.imageQuality, audioQuality: s.audioQuality, videoQuality: s.videoQuality, videoSize: s.videoSize, pdfDpi: s.pdfDpi };
    applyTheme(s.appearance);
    $('#dropTip').innerHTML = App.isMac ?
      '앱을 열지 않아도 돼요 — ' + finderWord() + '에서 파일을 끌다가 <kbd>⇧</kbd>를 누르거나, 파일을 고르고 <kbd>⌃</kbd><kbd>⌥</kbd><kbd>C</kbd>를 눌러 보세요.' :
      '파일을 이 창에 끌어다 놓거나 ‘파일 고르기’를 누르세요.';
    if (!App.isMac) $('#pickFolder').hidden = true;
    renderVersion();
    renderFiles();
    renderExits();
    route();
    poll();
    if (!s.seen) welcome();
    takePending();
    if (j.crash) {
      toast('지난번에 엔진이 갑자기 멈췄어요. ' + (j.crash.saved ? '진단 파일을 ‘다운로드’ 폴더에 저장했어요 (CVTROAD 진단.txt).' : '진단 정보를 남겼어요.'), 9000);
    }
  });
}
init();
