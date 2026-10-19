// CVTROAD 백그라운드 도우미 (macOS · JXA)
// made by. Nevertheless_D
//
//  · Shift를 누른 채 파일을 끌면 → 커서 주변에 원형 "변환" 메뉴, Shift+Option → "도구" 메뉴
//    원하는 칸 위에서 손을 떼면 바꿔요. 가운데에서 떼면 취소예요. (원본 파일은 그대로)
//  · 메뉴 막대 아이콘, 화면에 떠 있는 변환 창(선택), Finder 빠른 동작 · 단축키(⌃⌥C / ⌃⌥T)
//
// 실제 변환은 cvtroad-engine --run 이 해요. 이 스크립트는 화면에 메뉴를 띄우고 진행 상황만 보여 줘요.
// 권한: 손쉬운 사용 · 입력 모니터링 권한 없이 동작하도록 마우스 · 키 "상태"만 살펴봐요.

ObjC.import('Cocoa');
ObjC.import('WebKit');
try { ObjC.import('CoreGraphics'); } catch (e) {}

var FM = $.NSFileManager.defaultManager;

function str(o) {
  try { var v = ObjC.unwrap(o); return typeof v === 'string' ? v : ''; } catch (e) { return ''; }
}
function log(msg) {
  try {
    var line = new Date().toISOString() + ' ' + msg + '\n';
    $.NSFileHandle.fileHandleWithStandardError.writeData($(line).dataUsingEncoding($.NSUTF8StringEncoding));
  } catch (e) {}
}
function readFile(p) {
  var s = $.NSString.stringWithContentsOfFileEncodingError(p, $.NSUTF8StringEncoding, null);
  return s && !s.isNil() ? ObjC.unwrap(s) : null;
}
function writeFile(p, s) {
  $(s).writeToFileAtomicallyEncodingError(p, true, $.NSUTF8StringEncoding, null);
}
function exists(p) { return FM.fileExistsAtPath(p); }
function mkdirs(p) { FM.createDirectoryAtPathWithIntermediateDirectoriesAttributesError(p, true, $(), null); }
function mtime(p) {
  try {
    var a = FM.attributesOfItemAtPathError(p, null);
    if (!a || a.isNil()) return 0;
    return a.objectForKey('NSFileModificationDate').timeIntervalSince1970;
  } catch (e) { return 0; }
}
function listDir(p) {
  var out = [];
  try {
    var arr = FM.contentsOfDirectoryAtPathError(p, null);
    if (!arr || arr.isNil()) return out;
    for (var i = 0; i < arr.count; i++) out.push(str(arr.objectAtIndex(i)));
  } catch (e) {}
  return out;
}
function removeFile(p) { try { FM.removeItemAtPathError(p, null); } catch (e) {} }
function fileURL(p) { return $.NSURL.fileURLWithPath(p); }
function now() { return Date.now(); }

// ───────────────────────── 경로 · 설정
var ENV = $.NSProcessInfo.processInfo.environment;
var SUPPORT = str(ENV.objectForKey('CVTROAD_SUPPORT')) || (str($.NSHomeDirectory()) + '/Library/Application Support/CVTROAD');
var STATE = SUPPORT + '/agent';
var HERE = SUPPORT + '/bin/agent';
(function () {
  var args = $.NSProcessInfo.processInfo.arguments;
  for (var i = 0; i < args.count; i++) {
    var a = str(args.objectAtIndex(i));
    if (/agent\.js$/.test(a)) HERE = a.replace(/\/agent\.js$/, '');
  }
})();
var MENU = STATE + '/menu.json';
var INBOX = STATE + '/inbox';
var JOBS = STATE + '/jobs';
mkdirs(INBOX); mkdirs(JOBS);

var cfg = { kinds: [], exts: {}, shiftDrag: true, bubble: false, bubbleSide: 'right', appearance: 'system', engine: SUPPORT + '/bin/cvtroad-engine', bundleID: 'com.nevertheless-d.cvtroad' };
var cfgTime = -1;
function loadConfig() {
  var t = mtime(MENU);
  if (t === cfgTime) return false;
  cfgTime = t;
  var s = readFile(MENU);
  if (!s) return false;
  try {
    var c = JSON.parse(s);
    for (var k in c) cfg[k] = c[k];
    return true;
  } catch (e) { log('menu.json: ' + e); }
  return false;
}
loadConfig();

function kindOf(path) {
  var isDir = Ref();
  if (FM.fileExistsAtPathIsDirectory(path, isDir) && isDir[0] && !/\.rtfd$/i.test(path)) return 'folder';
  var n = path.toLowerCase().split('/').pop();
  if (/\.tar\.gz$/.test(n)) return 'archive';
  var m = n.match(/\.([a-z0-9]+)$/);
  var e = m ? m[1] : '';
  return cfg.exts[e] || 'other';
}
function kindByID(id) {
  for (var i = 0; i < cfg.kinds.length; i++) if (cfg.kinds[i].id === id) return cfg.kinds[i];
  return cfg.kinds[cfg.kinds.length - 1] || { id: 'other', label: '파일', color: '#9aa4b2', targets: [], tools: [] };
}
function mainKind(files) {
  var count = {};
  files.forEach(function (f) { var k = kindOf(f); count[k] = (count[k] || 0) + 1; });
  var best = 'other', n = -1;
  cfg.kinds.forEach(function (k) { if (count[k.id] > n && count[k.id] > 0) { best = k.id; n = count[k.id]; } });
  return best;
}
function baseName(p) { return p.split('/').pop(); }

// ───────────────────────── 화면 (원형 메뉴 · 떠 있는 창)
var SIZE = 480;          // 원형 메뉴 창 크기 (점)
var R_IN = 50, R_OUT = 205, R_HIT = 236;

var T = null;            // 타이머 · 메뉴 대상
var panel = null, web = null, webReady = false;
var bubble = null, bweb = null, bReady = false;
var statusItem = null, menuItems = {};

ObjC.registerSubclass({
  name: 'CVTPanel',
  superclass: 'NSPanel',
  methods: {
    'canBecomeKeyWindow': { types: ['bool', []], implementation: function () { return true; } },
    'canBecomeMainWindow': { types: ['bool', []], implementation: function () { return false; } }
  }
});

function makePanel(w, h) {
  var cls = $.CVTPanel || $.NSPanel;
  var p = cls.alloc.initWithContentRectStyleMaskBackingDefer($.NSMakeRect(0, 0, w, h), 128 /* nonactivating */, 2, false);
  p.opaque = false;
  p.backgroundColor = $.NSColor.clearColor;
  p.hasShadow = false;
  p.level = 101; // 메뉴처럼 맨 위
  p.collectionBehavior = 1 | 16 | 64 | 256; // 모든 데스크톱 · 전체 화면 위
  p.hidesOnDeactivate = false;
  p.releasedWhenClosed = false;
  try { p.floatingPanel = true; } catch (e) {}
  try { p.animationBehavior = 2; } catch (e) {}
  var conf = $.WKWebViewConfiguration.alloc.init;
  var wv = $.WKWebView.alloc.initWithFrameConfiguration($.NSMakeRect(0, 0, w, h), conf);
  try { wv.setValueForKey($(false), 'drawsBackground'); } catch (e) { log('drawsBackground: ' + e); }
  try { wv.underPageBackgroundColor = $.NSColor.clearColor; } catch (e) {}
  wv.autoresizingMask = 18;
  p.contentView = wv;
  return { panel: p, web: wv };
}

function loadPage(wv, query) {
  var url = $.NSURL.URLWithString(fileURL(HERE + '/ring.html').absoluteString.js + '#' + query);
  wv.loadFileURLAllowingReadAccessToURL(url, fileURL(HERE));
}

function js(wv, code) {
  try { wv.evaluateJavaScriptCompletionHandler(code, null); } catch (e) { log('js: ' + e); }
}

function theme() { return cfg.appearance || 'system'; }

function screenFor(pt) {
  var scr = $.NSScreen.screens;
  for (var i = 0; i < scr.count; i++) {
    var s = scr.objectAtIndex(i);
    var f = s.frame;
    if (pt.x >= f.origin.x && pt.x <= f.origin.x + f.size.width && pt.y >= f.origin.y && pt.y <= f.origin.y + f.size.height) return s;
  }
  return $.NSScreen.mainScreen;
}

// ───────────────────────── 원형 메뉴 상태
var ring = {
  visible: false,
  how: '',         // drag | click | busy | demo
  mode: 'convert', // convert | tools
  files: [],
  kind: null,
  items: [],
  hover: -1,
  cx: 0, cy: 0,    // 화면 좌표의 가운데
  lastEv: 0,
  shownAt: 0,
  keyLost: 0
};

function itemsFor(kind, mode) {
  var k = kindByID(kind);
  var list = mode === 'tools' ? (k.tools || []) : (k.targets || []);
  return list.map(function (t) { return { id: t.id, label: t.label, sub: t.sub || '', icon: t.icon || '', multi: !!t.multi, app: !!t.app }; });
}

function placePanel(x, y) {
  var s = screenFor({ x: x, y: y });
  var v = s.visibleFrame;
  var ox = Math.round(x - SIZE / 2), oy = Math.round(y - SIZE / 2);
  ox = Math.max(v.origin.x, Math.min(ox, v.origin.x + v.size.width - SIZE));
  oy = Math.max(v.origin.y, Math.min(oy, v.origin.y + v.size.height - SIZE));
  panel.setFrameDisplay($.NSMakeRect(ox, oy, SIZE, SIZE), true);
  ring.cx = ox + SIZE / 2;
  ring.cy = oy + SIZE / 2;
}

function showRing(how, mode, files, at) {
  if (!panel) return;
  var kind = files.length ? mainKind(files) : 'image';
  ring.how = how;
  ring.mode = mode;
  ring.files = files;
  ring.kind = kind;
  ring.items = itemsFor(kind, mode);
  ring.hover = -1;
  ring.shownAt = now();
  ring.keyLost = 0;
  placePanel(at.x, at.y);
  panel.ignoresMouseEvents = false;
  var k = kindByID(kind);
  var name = files.length === 1 ? baseName(files[0]) : (files.length > 1 ? baseName(files[0]) + ' 외 ' + (files.length - 1) + '개' : '체험해 보기');
  var data = {
    how: how, mode: mode, theme: theme(),
    kind: { id: k.id, label: k.label, color: k.color },
    count: files.length, name: name, items: ring.items,
    offset: { x: at.x - ring.cx, y: ring.cy - at.y },
    shortcuts: cfg.shortcuts || {}
  };
  js(web, 'CVT.show(' + JSON.stringify(data) + ')');
  if (how === 'click' || how === 'demo') {
    $.NSApplication.sharedApplication.activateIgnoringOtherApps(true);
    panel.makeKeyAndOrderFront(null);
    try { panel.makeFirstResponder(web); } catch (e) {}
  } else {
    panel.orderFrontRegardless;
  }
  ring.visible = true;
}

function hideRing() {
  if (!panel) return;
  js(web, 'CVT.hide()');
  panel.orderOut(null);
  ring.visible = false;
  ring.how = '';
  ring.hover = -1;
}

function hoverAt(pt) {
  var dx = pt.x - ring.cx, dy = pt.y - ring.cy;
  var r = Math.sqrt(dx * dx + dy * dy);
  var n = ring.items.length;
  if (r < R_IN || r > R_HIT || n === 0) return -1;
  var ang = Math.atan2(dx, dy); // 위쪽이 0, 시계 방향
  if (ang < 0) ang += Math.PI * 2;
  var step = Math.PI * 2 / n;
  return Math.floor((ang + step / 2) / step) % n;
}

function switchMode(mode) {
  if (ring.mode === mode || !ring.visible) return;
  ring.mode = mode;
  ring.items = itemsFor(ring.kind, mode);
  ring.hover = -1;
  js(web, 'CVT.items(' + JSON.stringify({ mode: mode, items: ring.items }) + ')');
}

// ───────────────────────── 고른 일 하기
var jobs = [];
var busyJob = null;

function choose(i) {
  var it = ring.items[i];
  if (!it) { hideRing(); return; }
  if (ring.how === 'demo') {
    js(web, 'CVT.done(' + JSON.stringify({ ok: true, message: '이렇게 바꿔요! (체험이라 실제 파일은 그대로예요)' }) + ')');
    ring.how = 'busy';
    panel.ignoresMouseEvents = true;
    busyJob = { demo: true, doneAt: now() };
    return;
  }
  var files = ring.files.slice();
  if (it.app) {
    // 앱 창에서 이어서 해요 (PDF 페이지 정리)
    writeFile(SUPPORT + '/pending.json', JSON.stringify({ tool: it.id, files: files, at: now() }));
    openApp();
    hideRing();
    return;
  }
  var id = now() + '-' + Math.floor(Math.random() * 1e6);
  var reqPath = JOBS + '/' + id + '.json';
  var outPath = JOBS + '/' + id + '.out';
  var req = { op: ring.mode === 'tools' ? 'tool' : 'convert', to: it.id, files: files, notify: true, from: 'ring' };
  writeFile(reqPath, JSON.stringify(req));
  var task = $.NSTask.alloc.init;
  task.launchPath = cfg.engine;
  task.arguments = $(['--run', reqPath, outPath]);
  try {
    task.launch;
  } catch (e) {
    log('launch: ' + e);
    js(web, 'CVT.done(' + JSON.stringify({ ok: false, message: 'CVTROAD 앱을 한 번 열어 주세요' }) + ')');
    busyJob = { doneAt: now() };
    ring.how = 'busy';
    return;
  }
  var job = { id: id, task: task, out: outPath, label: it.label, mode: ring.mode, started: now(), lastSize: -1, doneAt: 0 };
  jobs.push(job);
  busyJob = job;
  ring.how = 'busy';
  panel.ignoresMouseEvents = true; // 진행 표시는 클릭을 막지 않아요
  js(web, 'CVT.busy(' + JSON.stringify({ label: it.label, mode: ring.mode, title: files.length > 1 ? files.length + '개 파일' : baseName(files[0]) }) + ')');
}

function readLastLine(p) {
  var s = readFile(p);
  if (!s) return null;
  var lines = s.trim().split('\n');
  for (var i = lines.length - 1; i >= 0; i--) {
    try { return JSON.parse(lines[i]); } catch (e) {}
  }
  return null;
}

function pollJobs() {
  for (var i = jobs.length - 1; i >= 0; i--) {
    var j = jobs[i];
    var running = false;
    try { running = j.task.isRunning; } catch (e) {}
    var last = readLastLine(j.out);
    if (last && last.done) {
      jobs.splice(i, 1);
      removeFile(j.out);
      if (busyJob === j && ring.visible && ring.how === 'busy') {
        var r = last.result || {};
        var ok = (r.outputs && r.outputs.length > 0) || !!r.text;
        js(web, 'CVT.done(' + JSON.stringify({ ok: ok, message: r.message || '', text: r.text || '', skipped: !ok && (!r.errors || r.errors.length === 0) }) + ')');
        j.doneAt = now();
        busyJob = j;
      }
      continue;
    }
    if (last && busyJob === j && ring.visible && ring.how === 'busy') {
      js(web, 'CVT.progress(' + JSON.stringify({ p: last.p || 0, label: last.label || '' }) + ')');
    }
    if (!running && !(last && last.done)) {
      if (!j.goneAt) j.goneAt = now();
      if (now() - j.goneAt > 1500) {
        jobs.splice(i, 1);
        if (busyJob === j && ring.visible) {
          js(web, 'CVT.done(' + JSON.stringify({ ok: false, message: '변환이 중간에 멈췄어요. 다시 해 보세요' }) + ')');
          j.doneAt = now();
        }
      }
    }
  }
  // 끝난 결과는 잠깐 보여 주고 닫아요
  if (busyJob && busyJob.doneAt && ring.visible && ring.how === 'busy') {
    var wait = busyJob.demo ? 2600 : 2400;
    if (now() - busyJob.doneAt > wait) {
      busyJob = null;
      hideRing();
    }
  }
}

function openApp() {
  try {
    var t = $.NSTask.alloc.init;
    t.launchPath = '/usr/bin/open';
    t.arguments = $(['-b', cfg.bundleID]);
    t.launch;
  } catch (e) { log('open app: ' + e); }
}

function runEngine(args) {
  try {
    var t = $.NSTask.alloc.init;
    t.launchPath = cfg.engine;
    t.arguments = $(args);
    t.launch;
    t.waitUntilExit;
  } catch (e) { log('engine ' + args.join(' ') + ': ' + e); }
}

// ───────────────────────── 끌기(드래그) 살펴보기
var dragPB = null;
try { dragPB = $.NSPasteboard.pasteboardWithName($.NSPasteboardNameDrag || 'Apple CFPasteboard drag'); } catch (e) {
  dragPB = $.NSPasteboard.pasteboardWithName('Apple CFPasteboard drag');
}
var drag = { down: false, cc: 0, files: null, tries: 0, startedAt: 0, ringDone: false };

function readDragFiles() {
  var out = [];
  try {
    var opts = $.NSMutableDictionary.dictionary;
    opts.setObjectForKey($(true), 'NSPasteboardURLReadingFileURLsOnlyKey');
    var arr = dragPB.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), opts);
    if (arr && !arr.isNil()) {
      for (var i = 0; i < arr.count; i++) {
        var u = arr.objectAtIndex(i);
        var p = str(u.path);
        if (p) out.push(p);
      }
    }
  } catch (e) { log('drag read: ' + e); }
  return out;
}

function escPressed() {
  try { return !!$.CGEventSourceKeyState(1, 53); } catch (e) { return false; }
}

var paused = false;

function tickDrag() {
  var buttons = $.NSEvent.pressedMouseButtons;
  var flags = $.NSEvent.modifierFlags;
  var left = (buttons & 1) !== 0;
  var shift = (flags & 0x20000) !== 0;
  var opt = (flags & 0x80000) !== 0;
  var pt = $.NSEvent.mouseLocation;

  if (left && !drag.down) {
    drag.down = true;
    drag.cc = dragPB.changeCount;
    drag.files = null;
    drag.tries = 0;
    drag.ringDone = false;
    drag.startedAt = now();
  }
  if (left && drag.down && drag.files === null && dragPB.changeCount !== drag.cc) {
    var f = readDragFiles();
    if (f.length || ++drag.tries > 15) drag.files = f;
  }
  if (left && drag.files && drag.files.length && shift && !ring.visible && !drag.ringDone && cfg.shiftDrag && !paused) {
    // 내 원형 메뉴 · 떠 있는 창을 끄는 중이면 무시
    drag.ringDone = true;
    showRing('drag', opt ? 'tools' : 'convert', drag.files, pt);
  }
  if (ring.visible && ring.how === 'drag') {
    if (left) {
      if (escPressed()) { hideRing(); }
      else {
        if (shift || opt) switchMode(opt ? 'tools' : 'convert');
        var h = hoverAt(pt);
        if (h !== ring.hover) {
          ring.hover = h;
          js(web, 'CVT.hover(' + h + ')');
        }
      }
    } else {
      var hv = hoverAt(pt);
      if (hv >= 0) choose(hv); else hideRing();
    }
  }
  if (!left && drag.down) {
    drag.down = false;
    drag.files = null;
  }
}

// ───────────────────────── 화면 쪽(웹) 신호 읽기: document.title = "ev:<번호>:<할 일>"
function readEvents(wv, lastKey, handler) {
  var t = str(wv.title);
  var m = t.match(/^ev:(\d+):(.*)$/);
  if (!m) return;
  var n = parseInt(m[1], 10);
  if (n <= ring[lastKey]) return;
  ring[lastKey] = n;
  handler(m[2]);
}
ring.lastB = 0;

function tickClick() {
  if (!ring.visible || (ring.how !== 'click' && ring.how !== 'demo')) return;
  readEvents(web, 'lastEv', function (ev) {
    if (ev.indexOf('pick:') === 0) choose(parseInt(ev.slice(5), 10));
    else if (ev === 'cancel') hideRing();
    else if (ev === 'mode') switchMode(ring.mode === 'tools' ? 'convert' : 'tools');
  });
  // 다른 곳을 누르면 닫기
  if (ring.visible && (ring.how === 'click' || ring.how === 'demo') && now() - ring.shownAt > 600) {
    if (!panel.isKeyWindow) {
      if (!ring.keyLost) ring.keyLost = now();
      else if (now() - ring.keyLost > 250) hideRing();
    } else ring.keyLost = 0;
  }
}

// ───────────────────────── 떠 있는 변환 창 (파일을 여기에 끌어다 놓기)
var BSIZE = 96;
var grab = null;
var agentState = {};
function loadState() { try { agentState = JSON.parse(readFile(STATE + '/state.json') || '{}'); } catch (e) { agentState = {}; } }
function saveState() { writeFile(STATE + '/state.json', JSON.stringify(agentState)); }
loadState();

function placeBubble() {
  var s = $.NSScreen.mainScreen;
  var v = s.visibleFrame;
  var x, y;
  if (agentState.bubble && typeof agentState.bubble.x === 'number') {
    x = agentState.bubble.x; y = agentState.bubble.y;
  } else {
    x = cfg.bubbleSide === 'left' ? v.origin.x + 24 : v.origin.x + v.size.width - BSIZE - 24;
    y = v.origin.y + 120;
  }
  var sc = screenFor({ x: x + BSIZE / 2, y: y + BSIZE / 2 });
  var vv = sc.visibleFrame;
  x = Math.max(vv.origin.x, Math.min(x, vv.origin.x + vv.size.width - BSIZE));
  y = Math.max(vv.origin.y, Math.min(y, vv.origin.y + vv.size.height - BSIZE));
  bubble.setFrameDisplay($.NSMakeRect(x, y, BSIZE, BSIZE), true);
}

function syncBubble() {
  if (cfg.bubble && !paused) {
    if (!bubble) {
      var b = makePanel(BSIZE, BSIZE);
      bubble = b.panel; bweb = b.web;
      bubble.level = 3; // 다른 창 위에 떠 있기
      loadPage(bweb, 'mode=bubble&theme=' + theme());
      placeBubble();
    }
    if (!bubble.isVisible) { placeBubble(); bubble.orderFrontRegardless; }
    js(bweb, 'CVT.theme && CVT.theme(' + JSON.stringify(theme()) + ')');
  } else if (bubble && bubble.isVisible) {
    bubble.orderOut(null);
  }
}

function tickBubble() {
  if (!bubble || !bubble.isVisible) return;
  readEvents(bweb, 'lastB', function (ev) {
    if (ev === 'drop') {
      var files = readDragFiles();
      if (files.length) {
        var f = bubble.frame;
        showRing('click', 'convert', files, { x: f.origin.x + BSIZE / 2, y: f.origin.y + BSIZE / 2 });
      }
    } else if (ev === 'grab') {
      var p = $.NSEvent.mouseLocation, fr = bubble.frame;
      grab = { mx: p.x, my: p.y, x: fr.origin.x, y: fr.origin.y, moved: false };
    }
  });
  if (grab) {
    var left = ($.NSEvent.pressedMouseButtons & 1) !== 0;
    var p2 = $.NSEvent.mouseLocation;
    var dx = p2.x - grab.mx, dy = p2.y - grab.my;
    if (Math.abs(dx) + Math.abs(dy) > 4) grab.moved = true;
    if (grab.moved) bubble.setFrameOrigin({ x: grab.x + dx, y: grab.y + dy });
    if (!left) {
      if (!grab.moved) openApp();
      else { var fr2 = bubble.frame; agentState.bubble = { x: fr2.origin.x, y: fr2.origin.y }; saveState(); }
      grab = null;
    }
  }
}

// ───────────────────────── 빠른 동작 · 단축키에서 온 부탁
function tickInbox() {
  var names = listDir(INBOX).filter(function (n) { return /\.json$/.test(n); }).sort();
  names.forEach(function (n) {
    var p = INBOX + '/' + n;
    var s = readFile(p);
    removeFile(p);
    if (!s) return;
    try {
      var req = JSON.parse(s);
      if (now() - (req.at || 0) > 60000) return; // 오래된 부탁은 버려요
      var pt = $.NSEvent.mouseLocation;
      if (req.mode === 'demo') {
        var v = $.NSScreen.mainScreen.visibleFrame;
        showRing('demo', 'convert', [], { x: v.origin.x + v.size.width / 2, y: v.origin.y + v.size.height / 2 });
      } else if (req.files && req.files.length) {
        showRing('click', req.mode === 'tools' ? 'tools' : 'convert', req.files, pt);
      }
    } catch (e) { log('inbox: ' + e); }
  });
}

// ───────────────────────── 메뉴 막대
function menuItem(menu, title, sel, key) {
  var it = $.NSMenuItem.alloc.initWithTitleActionKeyEquivalent(title, sel || null, '');
  if (sel) it.target = T;
  menu.addItem(it);
  if (key) menuItems[key] = it;
  return it;
}

function buildStatusItem() {
  statusItem = $.NSStatusBar.systemStatusBar.statusItemWithLength(-1);
  var img = $.NSImage.alloc.initWithContentsOfFile(HERE + '/status.png');
  if (img && !img.isNil()) {
    img.setSize({ width: 18, height: 18 });
    img.template = true;
    statusItem.button.image = img;
  } else {
    statusItem.button.title = 'CVT';
  }
  statusItem.button.toolTip = 'CVTROAD — Shift를 누른 채 파일을 끌어 보세요';
  var m = $.NSMenu.alloc.init;
  m.autoenablesItems = false;
  var head = menuItem(m, 'CVTROAD 변환 도우미', null);
  head.enabled = false;
  m.addItem($.NSMenuItem.separatorItem);
  menuItem(m, 'CVTROAD 열기', 'menuOpen:');
  m.addItem($.NSMenuItem.separatorItem);
  menuItem(m, 'Shift-드래그 원형 메뉴', 'menuShift:', 'shift');
  menuItem(m, '떠 있는 변환 창', 'menuBubble:', 'bubble');
  menuItem(m, '잠시 멈추기', 'menuPause:', 'pause');
  m.addItem($.NSMenuItem.separatorItem);
  menuItem(m, '원형 메뉴 체험하기', 'menuDemo:');
  menuItem(m, '사용법 보기', 'menuHelp:');
  m.addItem($.NSMenuItem.separatorItem);
  menuItem(m, '도우미 끄기', 'menuQuit:');
  statusItem.menu = m;
  refreshMenu();
}

function refreshMenu() {
  if (!statusItem) return;
  try {
    menuItems.shift.state = cfg.shiftDrag ? 1 : 0;
    menuItems.bubble.state = cfg.bubble ? 1 : 0;
    menuItems.pause.state = paused ? 1 : 0;
    statusItem.button.alphaValue = paused ? 0.45 : 1.0;
  } catch (e) {}
}

// ───────────────────────── 타이머
var tickN = 0;
ObjC.registerSubclass({
  name: 'CVTAgent',
  superclass: 'NSObject',
  methods: {
    'tick:': {
      types: ['void', ['id']],
      implementation: function (t) {
        tickN++;
        try {
          if (!webReady && str(web.title).indexOf('ready') === 0) webReady = true;
          tickDrag();
          tickClick();
          if (tickN % 2 === 0) tickBubble();
          if (tickN % 6 === 0) pollJobs();
          if (tickN % 9 === 0) tickInbox();
          if (tickN % 30 === 0) {
            if (loadConfig()) { refreshMenu(); syncBubble(); }
          }
          if (tickN % 150 === 0) writeFile(STATE + '/alive', String(now()));
        } catch (e) {
          if (tickN % 30 === 0) log('tick: ' + e);
        }
      }
    },
    'menuOpen:': { types: ['void', ['id']], implementation: function () { openApp(); } },
    'menuShift:': { types: ['void', ['id']], implementation: function () {
      cfg.shiftDrag = !cfg.shiftDrag; refreshMenu(); runEngine(['--set', 'shiftDrag', String(cfg.shiftDrag)]);
    } },
    'menuBubble:': { types: ['void', ['id']], implementation: function () {
      cfg.bubble = !cfg.bubble; refreshMenu(); syncBubble(); runEngine(['--set', 'bubble', String(cfg.bubble)]);
    } },
    'menuPause:': { types: ['void', ['id']], implementation: function () {
      paused = !paused; refreshMenu(); syncBubble();
    } },
    'menuDemo:': { types: ['void', ['id']], implementation: function () {
      var v = $.NSScreen.mainScreen.visibleFrame;
      showRing('demo', 'convert', [], { x: v.origin.x + v.size.width / 2, y: v.origin.y + v.size.height / 2 });
    } },
    'menuHelp:': { types: ['void', ['id']], implementation: function () {
      writeFile(SUPPORT + '/pending.json', JSON.stringify({ tool: 'help', files: [], at: now() }));
      openApp();
    } },
    'menuQuit:': { types: ['void', ['id']], implementation: function () {
      // 설정에서 도우미를 끄고 (로그인할 때도 켜지지 않게) 끝내요
      runEngine(['--set', 'agent', 'false']);
      $.NSApplication.sharedApplication.terminate(null);
    } }
  }
});

function run(argv) {
  var app = $.NSApplication.sharedApplication;
  app.setActivationPolicy(1); // Dock에 나타나지 않는 도우미
  T = $.CVTAgent.alloc.init;
  var p = makePanel(SIZE, SIZE);
  panel = p.panel; web = p.web;
  loadPage(web, 'mode=ring&theme=' + theme());
  buildStatusItem();
  syncBubble();
  writeFile(STATE + '/alive', String(now()));
  var timer = $.NSTimer.timerWithTimeIntervalTargetSelectorUserInfoRepeats(1 / 30, T, 'tick:', null, true);
  $.NSRunLoop.currentRunLoop.addTimerForMode(timer, $.NSRunLoopCommonModes || 'kCFRunLoopCommonModes');
  log('CVTROAD agent started (' + (cfg.build || '?') + ')');
  app.run;
}
