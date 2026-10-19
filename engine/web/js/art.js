/* CVTROAD — 그림과 아이콘 (SVG). made by. Nevertheless_D */
var ART = (function () {
  var S = 'stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"';
  var icons = {
    shrink: '<path d="M4 14h6v6M20 10h-6V4M14 10l7-7M3 21l7-7"/>',
    frame: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 9h4M7 9v4M17 15h-4M17 15v-4"/>',
    compress: '<path d="M12 3v7m0 0-3-3m3 3 3-3M12 21v-7m0 0-3 3m3-3 3 3M4 12h16"/>',
    eraser: '<path d="M7 21h10M5.5 14.5l7-7a2 2 0 0 1 2.8 0l2.2 2.2a2 2 0 0 1 0 2.8L12 18H8.6a2 2 0 0 1-1.4-.6l-1.7-1.7a2 2 0 0 1 0-2.8z"/><path d="m9 11 5 5"/>',
    rotate: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
    rotL: '<path d="M4 11a8 8 0 1 1 2.3 5.7"/><path d="M4 4v7h7"/>',
    stack: '<path d="m12 3 9 5-9 5-9-5z"/><path d="m3 13 9 5 9-5"/>',
    qr: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 14h3v3h-3zM20 14v.01M20 20h.01M17 20h.01M14 20h.01M20 17h.01"/>',
    zip: '<path d="M6 3h9l4 4v14H6z"/><path d="M11 3v2h2v2h-2v2h2v2h-2v3"/><rect x="10" y="14" width="4" height="4" rx="1"/>',
    unzip: '<path d="M4 8h16v12H4z"/><path d="M2 4h20v4H2zM10 12h4"/>',
    mute: '<path d="M4 9v6h4l5 4V5L8 9z"/><path d="m17 9 4 6M21 9l-4 6"/>',
    wave: '<path d="M3 12h2M7 8v8M11 5v14M15 9v6M19 7v10M21 12h0"/>',
    level: '<path d="M4 18V9M9 18V5M14 18v-7M19 18v-4"/><path d="M2 21h20"/>',
    trim: '<circle cx="6" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M8.1 8.1 20 20M14.5 9.5 20 4M8.1 15.9l3.4-3.4"/>',
    split: '<path d="M7 3h7l4 4v4M18 15v6H7V15"/><path d="M3 13h18" stroke-dasharray="2 2.5"/>',
    pages: '<rect x="3" y="4" width="7" height="9" rx="1"/><rect x="14" y="4" width="7" height="9" rx="1"/><path d="M6 17h12M15 15l3 2-3 2"/>',
    arrow: '<path d="M5 12h14M13 6l6 6-6 6"/>',
    x: '<path d="M6 6l12 12M18 6 6 18"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z"/>',
    auto: '<circle cx="12" cy="12" r="8.5"/><path d="M12 3.5v17A8.5 8.5 0 0 0 12 3.5z" fill="currentColor"/>',
    folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
    reveal: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M10 13h5M13 11l2 2-2 2"/>',
    open: '<path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
    trash: '<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/>',
    undo: '<path d="M9 14 4 9l5-5"/><path d="M4 9h10a6 6 0 0 1 0 12h-3"/>',
    copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3"/>',
    up: '<path d="M12 19V5M6 11l6-6 6 6"/>',
    down: '<path d="M12 5v14M6 13l6 6 6-6"/>'
  };
  function icon(name, extra) {
    return '<svg viewBox="0 0 24 24" ' + S + ' aria-hidden="true"' + (extra || '') + '>' + (icons[name] || icons.arrow) + '</svg>';
  }

  // 회전 교차로 표지판 모양 로고 (앱 아이콘과 같아요)
  function mark() {
    return '<svg viewBox="0 0 64 64" aria-hidden="true">' +
      '<rect x="2" y="2" width="60" height="60" rx="15" fill="#0f6b4f"/><rect x="5.5" y="5.5" width="53" height="53" rx="12" fill="none" stroke="#fff" stroke-width="3"/>' +
      '<circle cx="32" cy="32" r="10.5" fill="none" stroke="#fff" stroke-width="5"/>' +
      '<path d="M32 12v9.5M32 42.5V52M12 32h9.5" stroke="#fff" stroke-width="5" stroke-linecap="round"/>' +
      '<path d="M41 23l9-9m0 0v7m0-7h-7" stroke="#f5c431" stroke-width="4.2" stroke-linecap="round" stroke-linejoin="round" fill="none"/></svg>';
  }

  function sign(x, y, label, sub) {
    return '<g transform="translate(' + x + ' ' + y + ')">' +
      '<rect x="-2" y="30" width="4" height="22" fill="var(--ink-3)" opacity=".55"/>' +
      '<rect x="-36" y="-4" width="72" height="38" rx="6" fill="#0f6b4f"/><rect x="-33" y="-1" width="66" height="32" rx="4" fill="none" stroke="#fff" stroke-width="2"/>' +
      '<text x="0" y="16" text-anchor="middle" font-family="Overpass, sans-serif" font-weight="800" font-size="16" fill="#fff">' + label + '</text>' +
      '<text x="0" y="27" text-anchor="middle" font-family="Pretendard, sans-serif" font-weight="600" font-size="7.5" fill="rgba(255,255,255,.8)">' + sub + '</text></g>';
  }

  // 첫 화면: 파일이 회전 교차로로 들어와 PNG 출구로 나가는 그림
  function hero() {
    var cx = 220, cy = 158, r = 74;
    var road = function (x2, y2) {
      return '<line x1="' + cx + '" y1="' + cy + '" x2="' + x2 + '" y2="' + y2 + '" stroke="var(--road)" stroke-width="30"/>' +
        '<line x1="' + cx + '" y1="' + cy + '" x2="' + x2 + '" y2="' + y2 + '" stroke="#f5c431" stroke-width="2" stroke-dasharray="8 8"/>';
    };
    var a45 = Math.SQRT1_2;
    var tr = [cx + r * a45, cy - r * a45];
    var path = 'M' + cx + ',330 L' + cx + ',' + (cy + r) + ' A' + r + ',' + r + ' 0 0 0 ' + tr[0].toFixed(1) + ',' + tr[1].toFixed(1) + ' L' + (cx + 150 * a45).toFixed(1) + ',' + (cy - 150 * a45).toFixed(1);
    return '<svg viewBox="0 0 440 310" role="img" aria-label="파일이 교차로를 지나 다른 형식으로 바뀌는 그림" style="--road:var(--asphalt)">' +
      '<defs><radialGradient id="hf" cx="50%" cy="50%" r="62%"><stop offset=".62" stop-color="#fff"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></radialGradient>' +
      '<mask id="hm"><rect x="0" y="0" width="440" height="310" fill="url(#hf)"/></mask></defs>' +
      '<g mask="url(#hm)">' +
      road(cx - 210, cy - 210) + road(cx + 210, cy - 210) + road(-10, cy) + road(450, cy) + road(cx, 330) +
      '<circle cx="' + cx + '" cy="' + cy + '" r="' + r + '" fill="none" stroke="var(--road)" stroke-width="34"/>' +
      '<circle cx="' + cx + '" cy="' + cy + '" r="' + r + '" fill="none" stroke="#f5c431" stroke-width="2" stroke-dasharray="8 8"><animate attributeName="stroke-dashoffset" from="0" to="-32" dur="1.6s" repeatCount="indefinite"/></circle>' +
      '<circle cx="' + cx + '" cy="' + cy + '" r="52" fill="#0f6b4f" stroke="#fff" stroke-width="3"/>' +
      '<text x="' + cx + '" y="' + (cy + 7) + '" text-anchor="middle" font-family="Overpass, sans-serif" font-weight="800" font-size="21" fill="#fff" letter-spacing="1">CVT</text>' +
      '</g>' +
      sign(70, 22, 'JPG', '사진') + sign(370, 22, 'PNG', '투명 배경') + sign(46, 196, 'MP3', '소리') + sign(394, 196, 'PDF', '문서') +
      // 움직이는 파일 카드
      '<g id="heroCar" opacity="0"><set attributeName="opacity" to="1" begin="heroMove.begin"/><g transform="translate(-17 -21)">' +
      '<path d="M0 4a4 4 0 0 1 4-4h18l12 12v26a4 4 0 0 1-4 4H4a4 4 0 0 1-4-4z" fill="#f28c28" stroke="#fff" stroke-width="2.5"><animate id="heroColor" attributeName="fill" values="#f28c28;#f28c28;#0f6b4f" keyTimes="0;.8;1" dur="3.6s" begin="heroMove.begin" fill="freeze"/></path>' +
      '<path d="M22 0v8a4 4 0 0 0 4 4h8" fill="rgba(255,255,255,.35)"/>' +
      '<text x="17" y="31" text-anchor="middle" font-family="Overpass, sans-serif" font-weight="800" font-size="10" fill="#fff">HEIC<animate attributeName="opacity" values="1;1;0" keyTimes="0;.78;.8" dur="3.6s" begin="heroColor.begin" fill="freeze"/></text>' +
      '<text x="17" y="31" text-anchor="middle" font-family="Overpass, sans-serif" font-weight="800" font-size="10" fill="#fff" opacity="0">PNG<animate attributeName="opacity" values="0;0;1" keyTimes="0;.8;.82" dur="3.6s" begin="heroColor.begin" fill="freeze"/></text>' +
      '</g><animateMotion id="heroMove" dur="3.6s" begin="0.7s" fill="freeze" keyPoints="0;1" keyTimes="0;1" calcMode="spline" keySplines=".45 0 .35 1" path="' + path + '"/></g>' +
      '</svg>';
  }

  function playHero(root) {
    var svg = (root || document).querySelector('#heroArt svg');
    if (!svg) return;
    var mv = svg.querySelector('#heroMove'), col = svg.querySelector('#heroColor');
    if (window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    try { mv.beginElement(); } catch (e) {}
  }

  // 파일 종류별 큰 표시 (미리 보기 그림이 없을 때)
  function fileBadge(ext) {
    return '<span>' + String(ext || '?').toUpperCase().slice(0, 4) + '</span>';
  }

  return { icon: icon, mark: mark, hero: hero, playHero: playHero, fileBadge: fileBadge };
})();
