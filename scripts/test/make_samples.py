#!/usr/bin/env python3
"""시험용 파일 만들기: make_samples.py <dir>"""
import os, subprocess, sys, zipfile, tarfile, io
from PIL import Image, ImageDraw
d = sys.argv[1]; os.makedirs(d, exist_ok=True)
def p(n): return os.path.join(d, n)
def ff(*a): subprocess.run(['ffmpeg', '-v', 'error', '-y', *a], check=True)
# 이미지: EXIF(방향 6 + GPS 비슷한 정보) JPG, 투명 PNG, WEBP, GIF(움짤)
im = Image.new('RGB', (1600, 1000), (40, 120, 200)); dr = ImageDraw.Draw(im); dr.rectangle([100, 100, 700, 500], fill=(250, 200, 40)); dr.text((120, 120), 'TOP-LEFT', fill=(0, 0, 0))
ex = Image.Exif(); ex[0x0112] = 6; ex[0x010F] = 'TestCam'; ex[0x0110] = 'Model X'
im.save(p('사진 (회전).jpg'), quality=92, exif=ex.tobytes())
rgba = Image.new('RGBA', (800, 600), (0, 0, 0, 0)); ImageDraw.Draw(rgba).ellipse([100, 100, 700, 500], fill=(255, 90, 60, 255))
from PIL import PngImagePlugin
info = PngImagePlugin.PngInfo(); info.add_text('Author', 'Someone'); info.add_text('Comment', 'secret')
rgba.save(p('logo.png'), pnginfo=info)
rgba.save(p('sticker.webp'))
frames = [Image.new('RGB', (200, 200), (i * 40 % 255, 100, 200)) for i in range(6)]
frames[0].save(p('anim.gif'), save_all=True, append_images=frames[1:], duration=120, loop=0)
# QR
import cv2, numpy as np
q = cv2.QRCodeEncoder.create().encode('https://example.com/cvtroad')
q = cv2.resize(q, (q.shape[1] * 8, q.shape[0] * 8), interpolation=cv2.INTER_NEAREST); q = cv2.copyMakeBorder(q, 40, 40, 40, 40, cv2.BORDER_CONSTANT, value=255)
cv2.imwrite(p('qr.png'), q)
# 영상 · 오디오
ff('-f', 'lavfi', '-i', 'testsrc2=size=1920x1080:rate=30:duration=6', '-f', 'lavfi', '-i', 'sine=frequency=440:duration=6', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-c:a', 'aac', '-metadata', 'location=+37.5665+126.9780/', '-metadata', 'title=Clip', p('영상.mp4'))
ff('-f', 'lavfi', '-i', 'testsrc=size=640x360:rate=25:duration=4', '-f', 'lavfi', '-i', 'sine=frequency=660:duration=4', '-c:v', 'mpeg4', '-c:a', 'pcm_s16le', p('old.avi'))
ff('-f', 'lavfi', '-i', 'anullsrc=r=44100:cl=stereo', '-f', 'lavfi', '-i', 'sine=frequency=523:duration=5', '-filter_complex', '[0][1]concat=n=2:v=0:a=1[a]', '-map', '[a]', '-t', '8', '-c:a', 'pcm_s24le', p('노래 24bit.wav'))
Image.new('RGB', (300, 300), (200, 30, 90)).save(p('_cover.jpg'))
ff('-f', 'lavfi', '-i', 'sine=frequency=330:duration=50', '-i', p('_cover.jpg'), '-map', '0:a', '-map', '1:v', '-c:a', 'libmp3lame', '-b:a', '192k', '-c:v', 'copy', '-id3v2_version', '3', '-disposition:v', 'attached_pic', '-metadata', 'artist=가수', '-metadata', 'title=노래', p('노래.mp3'))
os.remove(p('_cover.jpg'))
ff('-f', 'lavfi', '-i', 'sine=frequency=880:duration=3', '-c:a', 'flac', p('tone.flac'))
# PDF 3쪽 (글자 포함)
from reportlab.pdfgen import canvas
c = canvas.Canvas(p('문서.pdf')); c.setTitle('비밀 제목'); c.setAuthor('작성자 이름')
for i in range(3):
    c.setFont('Helvetica', 28); c.drawString(72, 700, f'Page {i+1} hello CVTROAD'); c.drawImage(p('logo.png'), 72, 300, 300, 225, mask='auto'); c.showPage()
c.save()
c = canvas.Canvas(p('한쪽.pdf')); c.drawString(72, 700, 'one page'); c.save()
# 문서
open(p('메모.txt'), 'wb').write('안녕하세요\nCVTROAD 시험 문서예요.\n'.encode('cp949'))
open(p('readme.md'), 'w').write('# 제목\n\n- 하나\n- 둘\n')
open(p('page.html'), 'w').write('<html><body><h1>제목</h1><p>본문 <b>굵게</b></p><script>x()</script></body></html>')
import docx
doc = docx.Document(); doc.add_heading('보고서', 1); doc.add_paragraph('첫 문단입니다.'); doc.add_paragraph('둘째 문단.'); doc.save(p('보고서.docx'))
open(p('letter.rtf'), 'w').write(r'{\rtf1\ansi{\fonttbl\f0 Helvetica;}\f0 Hello \b bold\b0\par 呢0?䑔4? text\par}')
# 압축: Windows식(CP949 이름) ZIP, 폴더 하나가 든 ZIP, tar.gz
# Windows 탐색기처럼 CP949 이름 + UTF-8 표시 없음
class CP949Info(zipfile.ZipInfo):
    def _encodeFilenameFlags(self):
        return self.filename.encode('cp949'), self.flag_bits
buf = zipfile.ZipFile(p('윈도우.zip'), 'w')
buf.writestr(CP949Info('한글파일.txt'), 'hello'); buf.writestr(CP949Info('폴더/안쪽.txt'), 'inner'); buf.close()
os.makedirs(p('묶을 폴더/sub'), exist_ok=True)
open(p('묶을 폴더/a.txt'), 'w').write('a'); open(p('묶을 폴더/sub/b.txt'), 'w').write('b'); open(p('묶을 폴더/.DS_Store'), 'w').write('x')
z = zipfile.ZipFile(p('one-folder.zip'), 'w'); z.write(p('묶을 폴더/a.txt'), 'proj/a.txt'); z.write(p('묶을 폴더/sub/b.txt'), 'proj/sub/b.txt'); z.close()
with tarfile.open(p('backup.tar.gz'), 'w:gz') as t: t.add(p('묶을 폴더'), arcname='묶을 폴더')
print('samples in', d)
