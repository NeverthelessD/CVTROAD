#!/usr/bin/env python3
"""모든 변환 · 도구를 엔진 CLI(--run)로 돌려 보고 결과 파일을 확인해요.
usage: run_matrix.py <engine> <samples dir> <out dir>"""
import json, os, subprocess, sys, tempfile, shutil, zipfile
engine, S, OUT = sys.argv[1:4]
shutil.rmtree(OUT, ignore_errors=True); os.makedirs(OUT)
env = dict(os.environ)
env.setdefault('CVTROAD_SUPPORT', tempfile.mkdtemp(prefix='cvsup-'))
env['CVTROAD_HELPER_CMD'] = 'python3 ' + os.path.join(os.path.dirname(__file__), 'fakehelper.py')
env['CVTROAD_TEST_CLIPBOARD'] = os.path.join(OUT, 'clipboard.txt')
def s(n): return os.path.join(S, n)
cases = [
  ('convert', t, ['사진 (회전).jpg']) for t in ['png', 'webp', 'avif', 'tiff', 'gif', 'bmp', 'pdf', 'ico', 'heic']
] + [('convert', t, ['logo.png']) for t in ['jpg', 'webp', 'bmp']] + [
  ('convert', 'png', ['sticker.webp']), ('convert', 'webp', ['anim.gif']), ('convert', 'jpg', ['anim.gif']),
  ('tool', 'half', ['사진 (회전).jpg']), ('tool', 'fit1920', ['사진 (회전).jpg']), ('tool', 'fit1920', ['logo.png']),
  ('tool', 'compress', ['사진 (회전).jpg']), ('tool', 'compress', ['logo.png']), ('tool', 'strip', ['사진 (회전).jpg']),
  ('tool', 'strip', ['logo.png']), ('tool', 'rotate', ['logo.png']), ('tool', 'pdfmerge', ['사진 (회전).jpg', 'logo.png', 'sticker.webp']),
  ('tool', 'qr', ['qr.png']), ('tool', 'zip', ['logo.png', 'qr.png']),
] + [('convert', t, ['영상.mp4']) for t in ['mov', 'webm', 'mkv', 'gif', 'mp3', 'm4a', 'wav']] + [
  ('convert', 'mp4', ['old.avi']),
  ('tool', 'compress', ['영상.mp4']), ('tool', '720p', ['영상.mp4']), ('tool', 'mute', ['영상.mp4']), ('tool', 'audio', ['영상.mp4']), ('tool', 'strip', ['영상.mp4']),
] + [('convert', t, ['노래.mp3']) for t in ['m4a', 'wav', 'flac', 'aiff', 'ogg', 'opus', 'm4r']] + [
  ('convert', 'mp3', ['노래 24bit.wav']), ('convert', 'flac', ['노래 24bit.wav']), ('convert', 'm4a', ['tone.flac']),
  ('tool', 'compress', ['노래 24bit.wav']), ('tool', 'loud', ['노래.mp3']), ('tool', 'trim', ['노래 24bit.wav']), ('tool', 'strip', ['노래.mp3']),
] + [('convert', t, ['문서.pdf']) for t in ['png', 'jpg', 'txt', 'docx', 'rtf']] + [
  ('convert', 'png', ['한쪽.pdf']),
  ('tool', 'compress', ['문서.pdf']), ('tool', 'split', ['문서.pdf']), ('tool', 'split', ['한쪽.pdf']), ('tool', 'merge', ['문서.pdf', '한쪽.pdf']),
  ('tool', 'rotate', ['문서.pdf']), ('tool', 'strip', ['문서.pdf']),
] + [('convert', t, ['보고서.docx']) for t in ['txt', 'html', 'rtf', 'pdf']] + [
  ('convert', 'docx', ['메모.txt']), ('convert', 'html', ['readme.md']), ('convert', 'txt', ['page.html']), ('convert', 'txt', ['letter.rtf']),
  ('convert', 'unzip', ['윈도우.zip']), ('convert', 'unzip', ['one-folder.zip']), ('convert', 'unzip', ['backup.tar.gz']),
  ('convert', 'tgz', ['윈도우.zip']), ('convert', 'zip', ['backup.tar.gz']),
  ('convert', 'zip', ['묶을 폴더']), ('convert', 'tgz', ['묶을 폴더']),
  ('convert', 'jpg', ['logo.png', '노래.mp3']),  # 섞여 있으면 건너뛰기
]
# 페이지 정리 (3,1 회전 90, 2 삭제)
cases.append(('tool', 'pages', ['문서.pdf'], [{'page': 3, 'rotate': 0}, {'page': 1, 'rotate': 90}]))
fails = 0
for c in cases:
    op, to, files = c[0], c[1], c[2]
    req = {'op': op, 'to': to, 'files': [s(f) for f in files], 'outDir': OUT}
    if len(c) > 3: req['pages'] = c[3]
    rq = os.path.join(OUT, '_req.json'); json.dump(req, open(rq, 'w'), ensure_ascii=False)
    pf = os.path.join(OUT, '_prog.jsonl')
    r = subprocess.run([engine, '--run', rq, pf], env=env, capture_output=True, text=True, timeout=300)
    lines = [json.loads(l) for l in open(pf) if l.strip()]
    res = lines[-1]['result']
    outs = [os.path.basename(o) for o in res['outputs']]
    status = 'OK ' if (outs or res.get('text')) else ('SKIP' if res['skipped'] and not res['errors'] else 'FAIL')
    if status == 'FAIL': fails += 1
    print(f"{status} {op:7} {to:8} {', '.join(files)[:34]:34} → {', '.join(outs)[:60]} {res.get('text','')[:40]} | {res['message'][:70]}{' | ' + '; '.join(res['errors'])[:120] if res['errors'] else ''}{' | notes: ' + '; '.join(res['notes']) if res['notes'] else ''}")
print('fails', fails)
