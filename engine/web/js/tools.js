/* CVTROAD — 도구 화면 · 도구 창 · PDF 페이지 정리. made by. Nevertheless_D */

var TOOL_HELP = {
  'image:half': '가로 · 세로를 반으로 줄여요. 용량은 보통 1/4쯤이 돼요.',
  'image:fit1920': '긴 쪽이 1920px보다 큰 사진만 1920px로 맞춰요. 메신저 · 메일로 보내기 좋아요.',
  'image:compress': '눈으로는 거의 차이 없게 화질을 조금 낮춰서 용량을 줄여요. 형식은 그대로예요.',
  'image:strip': '찍은 위치(GPS) · 기기 · 시간 같은 숨은 정보를 지운 사본을 만들어요. 돌려 찍은 사진은 바로 세워서 저장해요.',
  'image:rotate': '오른쪽으로 90° 돌린 사본을 만들어요.',
  'image:pdfmerge': '고른 순서대로 사진을 한 장씩 담은 PDF 하나를 만들어요.',
  'image:qr': '사진 속 QR 코드를 읽어서 내용을 복사해요.',
  'video:compress': '메신저로 보내기 좋게 용량을 줄여요 (1080p까지, H.264).',
  'video:720p': '720p(HD) 크기로 줄여요.',
  'video:mute': '소리를 없앤 영상을 만들어요. 다시 압축하지 않아서 금방 끝나요.',
  'video:audio': '영상 속 소리만 원래 음질 그대로 꺼내요.',
  'video:strip': '촬영 위치 · 기기 정보를 지운 사본을 만들어요. 화질은 그대로예요.',
  'audio:compress': 'MP3 128kbps로 줄여요.',
  'audio:loud': '들쭉날쭉한 음량을 스트리밍 기준(-14 LUFS)으로 고르게 맞춰요.',
  'audio:trim': '앞뒤의 조용한 부분을 잘라내요.',
  'audio:strip': '노래 제목 · 가수 · 앨범 표지 같은 정보를 지운 사본을 만들어요.',
  'pdf:compress': '안에 든 그림의 화질을 낮춰 용량을 줄여요. 글자는 그대로예요.',
  'pdf:split': '한 쪽씩 나눈 PDF들을 폴더 하나에 담아요.',
  'pdf:merge': '고른 순서대로 PDF를 하나로 합쳐요.',
  'pdf:pages': '쪽 그림을 보면서 순서를 바꾸고, 돌리고, 지운 새 PDF를 만들어요.',
  'pdf:rotate': '모든 쪽을 오른쪽으로 90° 돌려요.',
  'pdf:strip': '작성자 · 만든 프로그램 같은 문서 정보를 지운 사본을 만들어요.',
  'archive:unzip': '같은 폴더에 풀어요. Windows에서 만든 한글 이름도 깨지지 않게 풀어요.',
  'other:zip': '고른 파일 · 폴더를 ZIP 하나로 묶어요. Windows에서도 이름이 깨지지 않아요.'
};

function renderTools() {
  var box = $('#toolGroups');
  if (box.childElementCount) return;
  var order = ['image', 'video', 'audio', 'pdf', 'archive', 'other'];
  box.innerHTML = order.map(function (id) {
    var k = kindOf(id);
    var tools = (k.tools || []).filter(function (t) { return id === 'other' || t.id !== 'zip'; });
    if (id === 'other') tools = [{ id: 'zip', label: 'ZIP으로 묶기', sub: '파일 · 폴더 모두', icon: 'zip', multi: true }];
    var title = id === 'other' ? '모든 파일' : k.label;
    return '<div class="card tool-group" style="--k:' + k.color + '"><h2><span class="sw"></span>' + esc(title) + '</h2><div class="tool-list">' +
      tools.map(function (t) {
        return '<button type="button" class="tool-item" data-k="' + id + '" data-t="' + t.id + '"><span class="ic">' + ART.icon(t.icon) + '</span>' +
          '<span><b>' + esc(t.label) + '</b><span>' + esc(TOOL_HELP[id + ':' + t.id] || t.sub) + '</span></span></button>';
      }).join('') + '</div></div>';
  }).join('');
  $$('.tool-item', box).forEach(function (b) {
    b.addEventListener('click', function () {
      var k = b.getAttribute('data-k'), t = b.getAttribute('data-t');
      if (k === 'pdf' && t === 'pages') openPageEditor(null);
      else openToolSheet(k, t);
    });
  });
}

function toolInfo(kindID, toolID) {
  if (kindID === 'other') return { id: 'zip', label: 'ZIP으로 묶기', sub: '', icon: 'zip', multi: true };
  return (kindOf(kindID).tools || []).filter(function (t) { return t.id === toolID; })[0] || { id: toolID, label: toolID };
}

// ───────────────────────── 도구 창
function openToolSheet(kindID, toolID, preset) {
  var t = toolInfo(kindID, toolID);
  var k = kindOf(kindID);
  var list = [];
  var accept = function (f) { return kindID === 'other' || f.kind === kindID || (kindID === 'archive' && f.kind === 'archive'); };
  var s = openModal('<div class="sheet-head"><div><h2>' + esc(t.label) + '</h2><p class="muted" style="margin-top:4px">' + esc(TOOL_HELP[kindID + ':' + toolID] || '') + '</p></div>' +
    '<button class="x" type="button" id="tsClose" aria-label="닫기">' + ART.icon('x') + '</button></div>' +
    '<div class="mini-drop" id="tsDrop"><div>' + (kindID === 'other' ? '파일 · 폴더' : esc(k.label) + ' 파일') + '을 여기에 끌어다 놓거나</div>' +
    '<div class="row gap-s"><button class="btn small primary" id="tsPick" type="button">파일 고르기</button>' + (kindID === 'other' ? '<button class="btn small" id="tsPickDir" type="button">폴더 고르기</button>' : '') + '</div>' +
    '<ul class="mini-list" id="tsList"></ul></div>' +
    '<div class="sheet-foot"><button class="btn ghost" id="tsCancel" type="button">닫기</button><button class="btn go" id="tsRun" type="button" style="height:44px;font-size:15px" disabled>파일을 넣어 주세요</button></div>');
  function draw() {
    var ul = $('#tsList', s);
    ul.innerHTML = list.map(function (f, i) {
      return '<li><span class="num">' + (i + 1) + '</span><span class="nm" title="' + esc(f.path) + '">' + esc(f.name) + '</span><span class="muted">' + humanSize(f.size) + '</span>' +
        '<span class="mv">' + (t.multi ? '<button data-up="' + i + '" title="앞으로" aria-label="앞으로">' + ART.icon('up') + '</button><button data-dn="' + i + '" title="뒤로" aria-label="뒤로">' + ART.icon('down') + '</button>' : '') +
        '<button data-rm="' + i + '" title="빼기" aria-label="빼기">' + ART.icon('x') + '</button></span></li>';
    }).join('');
    $$('[data-rm]', ul).forEach(function (b) { b.onclick = function () { list.splice(+b.getAttribute('data-rm'), 1); draw(); }; });
    $$('[data-up]', ul).forEach(function (b) { b.onclick = function () { var i = +b.getAttribute('data-up'); if (i > 0) { var x = list[i]; list[i] = list[i - 1]; list[i - 1] = x; draw(); } }; });
    $$('[data-dn]', ul).forEach(function (b) { b.onclick = function () { var i = +b.getAttribute('data-dn'); if (i < list.length - 1) { var x = list[i]; list[i] = list[i + 1]; list[i + 1] = x; draw(); } }; });
    var run = $('#tsRun', s);
    var need = (t.id === 'merge') ? 2 : 1;
    run.disabled = list.length < need;
    run.textContent = list.length < need ? (need === 2 ? 'PDF를 두 개 이상 넣어 주세요' : '파일을 넣어 주세요') : (t.multi ? list.length + '개로 ' + t.label : list.length + '개 파일 · ' + t.label);
  }
  function add(infos) {
    var have = {};
    list.forEach(function (f) { have[f.path] = 1; });
    var skipped = 0;
    infos.forEach(function (f) { if (f.missing || have[f.path]) return; if (!accept(f)) { skipped++; return; } list.push(f); have[f.path] = 1; });
    if (skipped) toast(skipped + '개는 ' + k.label + ' 파일이 아니라서 뺐어요');
    draw();
  }
  function pick(folder) {
    if (!App.isMac) { var inp = $('#fileInput'); var h = function (e) { takeDropped(e.target.files, [], function (inf) { add(inf); }); inp.removeEventListener('change', h); }; inp.addEventListener('change', h); inp.click(); return; }
    api('/api/choose-files', { folder: !!folder }).then(function (j) {
      if (j.ok && j.paths.length) api('/api/info', { paths: j.paths }).then(function (r) { add(r.files || []); });
    });
  }
  $('#tsPick', s).onclick = function () { pick(false); };
  if ($('#tsPickDir', s)) $('#tsPickDir', s).onclick = function () { pick(true); };
  $('#tsClose', s).onclick = closeModal;
  $('#tsCancel', s).onclick = closeModal;
  wireDrop($('#tsDrop', s), function (infos) { add(infos); });
  $('#tsRun', s).onclick = function () {
    var req = { op: 'tool', to: t.id, files: list.map(function (f) { return f.path; }) };
    startJobs([req]).then(function (n) { if (n) { closeModal(); toast('작업을 시작했어요 — 아래 ‘작업’에서 볼 수 있어요'); } });
  };
  if (preset && preset.length) add(preset);
  draw();
}

// ───────────────────────── PDF 페이지 정리
function openPageEditor(path) {
  if (!path) {
    // 먼저 PDF 하나 고르기
    var s0 = openModal('<div class="sheet-head"><div><h2>페이지 정리</h2><p class="muted" style="margin-top:4px">' + esc(TOOL_HELP['pdf:pages']) + '</p></div><button class="x" type="button" id="peX" aria-label="닫기">' + ART.icon('x') + '</button></div>' +
      '<div class="mini-drop" id="peDrop"><div>PDF 하나를 여기에 끌어다 놓거나</div><button class="btn small primary" id="pePick" type="button">PDF 고르기</button></div>');
    $('#peX', s0).onclick = closeModal;
    var got = function (infos) {
      var f = (infos || []).filter(function (x) { return x.kind === 'pdf'; })[0];
      if (f) openPageEditor(f.path); else toast('PDF 파일을 넣어 주세요');
    };
    wireDrop($('#peDrop', s0), got);
    $('#pePick', s0).onclick = function () {
      if (!App.isMac) { var inp = $('#fileInput'); var h = function (e) { takeDropped(e.target.files, [], got); inp.removeEventListener('change', h); }; inp.addEventListener('change', h); inp.click(); return; }
      api('/api/choose-files', {}).then(function (j) { if (j.ok && j.paths.length) api('/api/info', { paths: j.paths }).then(function (r) { got(r.files); }); });
    };
    return;
  }
  var name = path.split('/').pop();
  var s = openModal('<div class="sheet-head"><div><h2>페이지 정리</h2><p class="muted" style="margin-top:4px">' + esc(name) + ' · 끌어서 순서 바꾸기, 버튼으로 돌리기 · 빼기</p></div><button class="x" type="button" id="peX" aria-label="닫기">' + ART.icon('x') + '</button></div>' +
    '<div class="pages-grid" id="peGrid"><p class="muted">쪽을 불러오는 중…</p></div>' +
    '<div class="sheet-foot"><span class="muted" id="peCount" style="margin-right:auto;align-self:center"></span><button class="btn ghost" id="peReset" type="button">' + ART.icon('undo') + '처음대로</button><button class="btn go" id="peSave" type="button" style="height:44px;font-size:15px">새 PDF로 저장</button></div>', true);
  $('#peX', s).onclick = closeModal;
  var pages = [];
  var pdfDoc = null;
  var canvases = {};
  api('/api/pdf/pages', { path: path }).then(function (j) {
    if (!j.ok) { $('#peGrid', s).innerHTML = '<p class="muted">' + esc(j.error || 'PDF를 열지 못했어요') + '</p>'; return; }
    pages = j.pages.map(function (d, i) { return { page: i + 1, rotate: 0, gone: false, w: d.w, h: d.h }; });
    draw();
    if (window.pdfjsLib) {
      pdfjsLib.GlobalWorkerOptions.workerSrc = 'vendor/pdf.worker.min.js';
      pdfjsLib.getDocument({ url: withToken('/api/file?p=' + encodeURIComponent(path)) }).promise.then(function (doc) {
        pdfDoc = doc;
        var i = 0;
        (function next() {
          if (i >= doc.numPages || $('#modal').hidden) return;
          var n = ++i;
          doc.getPage(n).then(function (pg) {
            var vp = pg.getViewport({ scale: 1 });
            var sc = Math.min(240 / vp.width, 300 / vp.height);
            var v2 = pg.getViewport({ scale: sc });
            var c = document.createElement('canvas');
            c.width = Math.round(v2.width); c.height = Math.round(v2.height);
            pg.render({ canvasContext: c.getContext('2d'), viewport: v2 }).promise.then(function () {
              canvases[n] = c;
              place(n);
              next();
            });
          });
        })();
      }).catch(function () { /* 그림 없이도 정리할 수 있어요 */ });
    }
  });
  function place(n) {
    var slot = s.querySelector('.pg[data-page="' + n + '"] .paper');
    if (!slot || !canvases[n]) return;
    var p = pages.filter(function (x) { return x.page === n; })[0];
    var c = canvases[n].cloneNode(true);
    c.getContext('2d').drawImage(canvases[n], 0, 0);
    c.style.transform = 'rotate(' + (p ? p.rotate : 0) + 'deg)';
    slot.innerHTML = '';
    slot.appendChild(c);
  }
  var dragFrom = -1;
  function draw() {
    var g = $('#peGrid', s);
    g.innerHTML = pages.map(function (p, i) {
      return '<div class="pg' + (p.gone ? ' gone' : '') + '" draggable="true" data-i="' + i + '" data-page="' + p.page + '">' +
        '<div class="paper"><div style="width:' + (p.w > p.h ? 120 : Math.round(120 * p.w / p.h)) + 'px;height:' + (p.w > p.h ? Math.round(120 * p.h / p.w) : 150) + 'px;background:#fff;box-shadow:0 1px 4px rgba(0,0,0,.2);transform:rotate(' + p.rotate + 'deg)"></div></div>' +
        '<div class="no">' + (i + 1) + ' · 원래 ' + p.page + '쪽</div>' +
        '<div class="acts"><button type="button" data-l="' + i + '" title="왼쪽으로 돌리기" aria-label="왼쪽으로 돌리기">' + ART.icon('rotL') + '</button>' +
        '<button type="button" data-r="' + i + '" title="오른쪽으로 돌리기" aria-label="오른쪽으로 돌리기">' + ART.icon('rotate') + '</button>' +
        '<button type="button" class="del" data-d="' + i + '" title="' + (p.gone ? '다시 넣기' : '빼기') + '" aria-label="빼기">' + ART.icon(p.gone ? 'undo' : 'trash') + '</button></div></div>';
    }).join('');
    pages.forEach(function (p) { place(p.page); });
    $$('[data-l]', g).forEach(function (b) { b.onclick = function () { var p = pages[+b.getAttribute('data-l')]; p.rotate = (p.rotate + 270) % 360; draw(); }; });
    $$('[data-r]', g).forEach(function (b) { b.onclick = function () { var p = pages[+b.getAttribute('data-r')]; p.rotate = (p.rotate + 90) % 360; draw(); }; });
    $$('[data-d]', g).forEach(function (b) { b.onclick = function () { var p = pages[+b.getAttribute('data-d')]; p.gone = !p.gone; draw(); }; });
    $$('.pg', g).forEach(function (el) {
      el.addEventListener('dragstart', function (e) { dragFrom = +el.getAttribute('data-i'); el.classList.add('dragging'); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', 'pg'); });
      el.addEventListener('dragend', function () { el.classList.remove('dragging'); });
      el.addEventListener('dragover', function (e) {
        if (dragFrom < 0) return;
        e.preventDefault(); e.stopPropagation();
        var r = el.getBoundingClientRect(), after = e.clientX > r.left + r.width / 2;
        $$('.pg', g).forEach(function (x) { x.classList.remove('drop-before', 'drop-after'); });
        el.classList.add(after ? 'drop-after' : 'drop-before');
      });
      el.addEventListener('drop', function (e) {
        if (dragFrom < 0) return;
        e.preventDefault(); e.stopPropagation();
        var to = +el.getAttribute('data-i');
        var r = el.getBoundingClientRect(), after = e.clientX > r.left + r.width / 2;
        var item = pages.splice(dragFrom, 1)[0];
        if (dragFrom < to) to--;
        pages.splice(after ? to + 1 : to, 0, item);
        dragFrom = -1;
        draw();
      });
    });
    var keep = pages.filter(function (p) { return !p.gone; }).length;
    $('#peCount', s).textContent = '남길 쪽 ' + keep + ' / ' + pages.length;
    $('#peSave', s).disabled = keep === 0;
  }
  $('#peReset', s).onclick = function () {
    pages.sort(function (a, b) { return a.page - b.page; });
    pages.forEach(function (p) { p.rotate = 0; p.gone = false; });
    draw();
  };
  $('#peSave', s).onclick = function () {
    var keep = pages.filter(function (p) { return !p.gone; }).map(function (p) { return { page: p.page, rotate: p.rotate }; });
    startJobs([{ op: 'tool', to: 'pages', files: [path], pages: keep }]).then(function (n) {
      if (n) { closeModal(); toast('새 PDF를 만들고 있어요'); }
    });
  };
}
