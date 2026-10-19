#!/usr/bin/env python3
"""Linux stand-in for engine/helper.js (시험용). usage: fakehelper.py <mode> <args.json>
pdftext → pdftotext · pdfimg → pdftoppm · pdfcompress → qpdf · docpdf → LibreOffice · qr → OpenCV · heicstrip → copy"""
import json, os, shutil, subprocess, sys, glob

def emit(o):
    print(json.dumps(o, ensure_ascii=False), flush=True)

mode = sys.argv[1]
if mode == 'check':
    emit({'ok': True, 'os': 'linux-fake'}); sys.exit(0)
a = json.load(open(sys.argv[2]))
try:
    if mode == 'pdftext':
        t = subprocess.run(['pdftotext', '-layout', a['in'], '-'], capture_output=True, text=True, check=True).stdout
        emit({'ok': True, 'text': t.replace('\f', '\n\n')})
    elif mode == 'pdfimg':
        fmt = a['fmt']; flag = {'png': '-png', 'jpg': '-jpeg', 'tiff': '-tiff'}[fmt]
        tmp = os.path.join(a['outDir'], '_raw'); os.makedirs(tmp, exist_ok=True)
        subprocess.run(['pdftoppm', flag, '-r', str(a.get('dpi', 150)), a['in'], os.path.join(tmp, 'p')], check=True)
        made = sorted(glob.glob(os.path.join(tmp, 'p*')))
        n = len(made); w = max(3, len(str(n)))
        for i, m in enumerate(made):
            ext = {'png': 'png', 'jpg': 'jpg', 'tiff': 'tiff'}[fmt]
            name = (a['prefix'] if n == 1 else f"{a['prefix']} {i+1:0{w}d}") + '.' + ext
            shutil.move(m, os.path.join(a['outDir'], name))
            emit({'tick': 1, 'page': i + 1, 'pages': n})
        shutil.rmtree(tmp)
        emit({'ok': True, 'pages': n})
    elif mode == 'pdfcompress':
        subprocess.run(['qpdf', '--recompress-flate', '--compression-level=9', '--object-streams=generate', a['in'], a['out']], check=True)
        emit({'ok': True, 'out': a['out']})
    elif mode == 'docpdf':
        out = a['out']; d = os.path.dirname(out) + '/_lo'
        subprocess.run(['soffice', '--headless', '--convert-to', 'pdf', '--outdir', d, a['in']], capture_output=True, check=True, timeout=120)
        made = glob.glob(d + '/*.pdf')
        shutil.move(made[0], out); shutil.rmtree(d)
        emit({'ok': True, 'out': out})
    elif mode == 'qr':
        import cv2
        img = cv2.imread(a['in'])
        t, _, _ = cv2.QRCodeDetector().detectAndDecode(img)
        texts = [t] if t else []
        for t in texts: emit({'ok': True, 'text': t})
        if not texts: emit({'ok': True, 'text': ''})
    elif mode == 'upright':
        from PIL import Image, ImageOps
        ImageOps.exif_transpose(Image.open(a['in'])).save(a['out']); emit({'ok': True})
    elif mode == 'heicstrip':
        shutil.copy(a['in'], a['out']); emit({'ok': True})
    else:
        emit({'ok': False, 'error': 'unknown ' + mode})
except Exception as e:
    emit({'ok': False, 'error': str(e)})
