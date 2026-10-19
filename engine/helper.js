// CVTROAD helper — macOS 기본 기능(PDFKit · Vision · Core Image · 인쇄)을 쓰는 작은 도우미
// osascript -l JavaScript helper.js <mode> <args.json>
//   mode: check | pdftext | pdfimg | pdfcompress | docpdf | qr | heicstrip | upright
// 결과는 한 줄에 하나씩 JSON으로 내보내요. 파일은 이 컴퓨터 밖으로 나가지 않아요.
// made by. Nevertheless_D
ObjC.import('Foundation');
ObjC.import('AppKit');
try { ObjC.import('PDFKit'); } catch (e) { try { ObjC.import('Quartz'); } catch (e2) {} }
var HAS_VISION = true;
try { ObjC.import('Vision'); } catch (e) { HAS_VISION = false; }
try { ObjC.import('CoreImage'); } catch (e) { try { ObjC.import('QuartzCore'); } catch (e2) {} }

var OUT = $.NSFileHandle.fileHandleWithStandardOutput;

function emit(obj) {
  var s = JSON.stringify(obj) + '\n';
  OUT.writeData($(s).dataUsingEncoding($.NSUTF8StringEncoding));
}

function readArgs(file) {
  var s = $.NSString.stringWithContentsOfFileEncodingError(file, $.NSUTF8StringEncoding, null);
  var js = ObjC.unwrap(s);
  return js ? JSON.parse(js) : {};
}

function str(o) {
  try { var v = ObjC.unwrap(o); return typeof v === 'string' ? v : ''; } catch (e) { return ''; }
}

function fileURL(p) { return $.NSURL.fileURLWithPath(p); }

function openPDF(p) {
  var doc = $.PDFDocument.alloc.initWithURL(fileURL(p));
  if (!doc || doc.isNil()) throw new Error('PDF를 열 수 없어요');
  if (doc.isLocked) throw new Error('암호가 걸린 PDF는 바꿀 수 없어요');
  return doc;
}

function pad(n, w) { var s = String(n); while (s.length < w) s = '0' + s; return s; }

// ───────────────────────── PDF 글자
function pdfText(a) {
  var doc = openPDF(a.in);
  var n = doc.pageCount, parts = [];
  for (var i = 0; i < n; i++) {
    var s = str(doc.pageAtIndex(i).string);
    parts.push(s);
  }
  emit({ ok: true, text: parts.join('\n\n'), pages: n });
}

// ───────────────────────── PDF → 그림 (쪽마다)
var FILETYPE = { tiff: 0, jpg: 3, png: 4 };

function renderPage(pg, scale) {
  var box = 1; // CropBox
  var b = pg.boundsForBox(box);
  var w = b.size.width, h = b.size.height;
  if (!(w > 0 && h > 0)) { box = 0; b = pg.boundsForBox(0); w = b.size.width; h = b.size.height; }
  var rot = pg.rotation;
  if (rot === 90 || rot === 270) { var t = w; w = h; h = t; }
  var W = Math.max(1, Math.round(w * scale)), H = Math.max(1, Math.round(h * scale));
  var rep = $.NSBitmapImageRep.alloc.initWithBitmapDataPlanesPixelsWidePixelsHighBitsPerSampleSamplesPerPixelHasAlphaIsPlanarColorSpaceNameBytesPerRowBitsPerPixel(
    null, W, H, 8, 4, true, false, $.NSCalibratedRGBColorSpace, 0, 0);
  try {
    var gc = $.NSGraphicsContext.graphicsContextWithBitmapImageRep(rep);
    $.NSGraphicsContext.saveGraphicsState;
    $.NSGraphicsContext.setCurrentContext(gc);
    $.NSColor.whiteColor.setFill;
    $.NSRectFill($.NSMakeRect(0, 0, W, H));
    var tf = $.NSAffineTransform.transform;
    tf.scaleBy(scale);
    tf.concat;
    pg.drawWithBoxToContext(box, gc.CGContext);
    gc.flushGraphics;
    $.NSGraphicsContext.restoreGraphicsState;
    return rep;
  } catch (e) {
    try { $.NSGraphicsContext.restoreGraphicsState; } catch (e2) {}
    // 다른 방법: 미리 보기 그림
    var img = pg.thumbnailOfSizeForBox({ width: W, height: H }, box);
    return $.NSBitmapImageRep.imageRepWithData(img.TIFFRepresentation);
  }
}

function pdfImages(a) {
  var doc = openPDF(a.in);
  var n = doc.pageCount;
  var scale = (a.dpi || 150) / 72;
  var fmt = a.fmt || 'png';
  var type = FILETYPE[fmt];
  var w = String(n).length < 3 ? 3 : String(n).length;
  for (var i = 0; i < n; i++) {
    var pool = $.NSAutoreleasePool.alloc.init;
    var pg = doc.pageAtIndex(i);
    var rep = renderPage(pg, scale);
    var props = $.NSMutableDictionary.dictionary;
    if (fmt === 'jpg') props.setObjectForKey($(0.9), 'NSImageCompressionFactor');
    try { rep.setSize({ width: rep.pixelsWide * 72 / (a.dpi || 150), height: rep.pixelsHigh * 72 / (a.dpi || 150) }); } catch (e) {}
    var data = rep.representationUsingTypeProperties(type, props);
    var name = (n === 1 ? a.prefix : a.prefix + ' ' + pad(i + 1, w)) + '.' + fmt;
    var out = a.outDir + '/' + name;
    var ok = data && !data.isNil() && data.writeToFileAtomically(out, true);
    if (!ok) throw new Error((i + 1) + '쪽을 그림으로 저장하지 못했어요');
    emit({ tick: 1, page: i + 1, pages: n });
    pool.drain;
  }
  emit({ ok: true, pages: n });
}

// ───────────────────────── PDF 용량 줄이기
function pdfCompress(a) {
  var doc = openPDF(a.in);
  var opts = $.NSMutableDictionary.dictionary;
  var jpegKey = 'SaveImagesAsJPEG', screenKey = 'OptimizeImagesForScreen';
  try { if ($.PDFDocumentSaveImagesAsJPEGOption) jpegKey = $.PDFDocumentSaveImagesAsJPEGOption; } catch (e) {}
  try { if ($.PDFDocumentOptimizeImagesForScreenOption) screenKey = $.PDFDocumentOptimizeImagesForScreenOption; } catch (e) {}
  opts.setObjectForKey($(true), jpegKey);
  opts.setObjectForKey($(true), screenKey);
  var ok = doc.writeToURLWithOptions(fileURL(a.out), opts);
  if (!ok) {
    // 예전 방식: 맥의 "파일 크기 줄이기" 필터
    var f = $.QuartzFilter.quartzFilterWithURL(fileURL('/System/Library/Filters/Reduce File Size.qfilter'));
    var o2 = $.NSMutableDictionary.dictionary;
    o2.setObjectForKey(f, 'QuartzFilter');
    ok = doc.writeToURLWithOptions(fileURL(a.out), o2);
  }
  if (!ok) throw new Error('PDF 용량을 줄이지 못했어요');
  emit({ ok: true, out: a.out });
}

// ───────────────────────── 문서 → PDF (맥의 인쇄 기능으로 A4 PDF 저장)
function docPDF(a) {
  var err = Ref();
  var attr = $.NSAttributedString.alloc.initWithURLOptionsDocumentAttributesError(fileURL(a.in), $.NSDictionary.dictionary, null, err);
  if (!attr || attr.isNil()) throw new Error('문서를 열지 못했어요');
  var pw = 595.28, ph = 841.89, m = 56;
  var pi = $.NSPrintInfo.alloc.initWithDictionary($.NSDictionary.dictionary);
  pi.paperSize = { width: pw, height: ph };
  pi.topMargin = m; pi.bottomMargin = m; pi.leftMargin = m; pi.rightMargin = m;
  pi.horizontalPagination = 1; // 너비에 맞추기
  pi.verticalPagination = 0;   // 자동으로 쪽 나누기
  pi.horizontallyCentered = false;
  pi.verticallyCentered = false;
  pi.jobDisposition = 'NSPrintSaveJob';
  pi.dictionary.setObjectForKey(fileURL(a.out), 'NSJobSavingURL');
  var w = pw - m * 2;
  var tv = $.NSTextView.alloc.initWithFrame($.NSMakeRect(0, 0, w, ph - m * 2));
  tv.textStorage.setAttributedString(attr);
  tv.horizontallyResizable = false;
  tv.verticallyResizable = true;
  tv.textContainer.containerSize = { width: w, height: 10000000 };
  tv.textContainer.widthTracksTextView = true;
  tv.layoutManager.ensureLayoutForTextContainer(tv.textContainer);
  var used = tv.layoutManager.usedRectForTextContainer(tv.textContainer);
  tv.setFrame($.NSMakeRect(0, 0, w, Math.max(used.size.height + 20, ph - m * 2)));
  var op = $.NSPrintOperation.printOperationWithViewPrintInfo(tv, pi);
  op.showsPrintPanel = false;
  op.showsProgressPanel = false;
  var ok = op.runOperation;
  if (!ok || !$.NSFileManager.defaultManager.fileExistsAtPath(a.out)) throw new Error('PDF로 바꾸지 못했어요');
  emit({ ok: true, out: a.out });
}

// ───────────────────────── QR 코드 읽기 (Vision)
function qr(a) {
  if (!HAS_VISION) throw new Error('이 맥에서는 QR 코드를 읽을 수 없어요');
  var handler = $.VNImageRequestHandler.alloc.initWithURLOptions(fileURL(a.in), $.NSDictionary.dictionary);
  var req = $.VNDetectBarcodesRequest.alloc.init;
  handler.performRequestsError($.NSArray.arrayWithObject(req), null);
  var res = req.results;
  var found = 0;
  if (res && !res.isNil()) {
    for (var i = 0; i < res.count; i++) {
      var s = str(res.objectAtIndex(i).payloadStringValue);
      if (s) { emit({ ok: true, text: s }); found++; }
    }
  }
  if (!found) emit({ ok: true, text: '' });
}

// ───────────────────────── HEIC 정보 지우기 (Core Image로 다시 저장 · 방향은 그림에 적용)
function heicStrip(a) {
  var opts = $.NSMutableDictionary.dictionary;
  opts.setObjectForKey($(true), 'kCIImageApplyOrientationProperty');
  var img = $.CIImage.imageWithContentsOfURLOptions(fileURL(a.in), opts);
  if (!img || img.isNil()) throw new Error('HEIC 사진을 열지 못했어요');
  try { img = img.imageBySettingProperties($.NSDictionary.dictionary); } catch (e) {}
  var ctx = $.CIContext.context;
  var cs = img.colorSpace;
  if (!cs || (cs.isNil && cs.isNil())) cs = $.CGColorSpaceCreateWithName($.kCGColorSpaceSRGB);
  var fmt = 0;
  try { fmt = $.kCIFormatRGBA8; } catch (e) {}
  var ok = ctx.writeHEIFRepresentationOfImageToURLFormatColorSpaceOptionsError(img, fileURL(a.out), fmt, cs, $.NSDictionary.dictionary, null);
  if (!ok) throw new Error('HEIC 사진을 저장하지 못했어요');
  emit({ ok: true, out: a.out });
}

// ───────────────────────── 돌려 찍은 사진 바로 세우기 (EXIF 방향 → 실제 그림, PNG로)
function upright(a) {
  var opts = $.NSMutableDictionary.dictionary;
  opts.setObjectForKey($(true), 'kCIImageApplyOrientationProperty');
  var img = $.CIImage.imageWithContentsOfURLOptions(fileURL(a.in), opts);
  if (!img || img.isNil()) throw new Error('사진을 열지 못했어요');
  var ctx = $.CIContext.context;
  var cs = img.colorSpace;
  if (!cs || (cs.isNil && cs.isNil())) cs = $.CGColorSpaceCreateWithName($.kCGColorSpaceSRGB);
  var fmt = 0;
  try { fmt = $.kCIFormatRGBA8; } catch (e) {}
  var ok = ctx.writePNGRepresentationOfImageToURLFormatColorSpaceOptionsError(img, fileURL(a.out), fmt, cs, $.NSDictionary.dictionary, null);
  if (!ok) throw new Error('사진을 바로 세우지 못했어요');
  emit({ ok: true, out: a.out });
}

function run(argv) {
  var mode = argv[0];
  if (mode === 'check') {
    var o = { ok: true, os: str($.NSProcessInfo.processInfo.operatingSystemVersionString) };
    try { o.pdfkit = !!$.PDFDocument; } catch (e) { o.pdfkit = false; }
    o.vision = HAS_VISION;
    emit(o);
    return;
  }
  var a = readArgs(argv[1]);
  try {
    if (mode === 'pdftext') pdfText(a);
    else if (mode === 'pdfimg') pdfImages(a);
    else if (mode === 'pdfcompress') pdfCompress(a);
    else if (mode === 'docpdf') docPDF(a);
    else if (mode === 'qr') qr(a);
    else if (mode === 'heicstrip') heicStrip(a);
    else if (mode === 'upright') upright(a);
    else emit({ ok: false, error: '알 수 없는 일이에요: ' + mode });
  } catch (e) {
    emit({ ok: false, error: String(e && e.message ? e.message : e) });
  }
}
