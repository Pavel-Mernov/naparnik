#!/usr/bin/env python3
"""Слои маскота для анимации: тело без поднятой лапы и отдельная лапа (frontend/src/shared/assets/mascot).

Запуск из корня: python3 scripts/mascot_layers.py frontend/src/shared/assets/maskot2.png frontend/src/shared/assets/mascot
Нужны numpy, opencv-python, Pillow. Лапа вырезается по тёплому цвету, место под ней заливается фоном круга,
под запястьем — мехом предплечья. Координаты лапы для вёрстки печатаются в конце (paw crop).
"""
import sys

import cv2
import numpy as np
from PIL import Image
src, out = sys.argv[1], sys.argv[2]
im = np.array(Image.open(src).convert("RGBA")).astype(np.int32)
H, W = im.shape[:2]
R, G, B, A = im[...,0], im[...,1], im[...,2], im[...,3]
yy, xx = np.mgrid[0:H, 0:W]

# 1) Круг-подложка: пиксели цвета #b2d9fa → окружность по МНК.
circ = (abs(R-0xb2)<14)&(abs(G-0xd9)<14)&(abs(B-0xfa)<12)&(A>250)
ys, xs = np.nonzero(circ)
# граница круга: крайние точки по строкам
pts=[]
for y in np.unique(ys):
    row = xs[ys==y]; pts += [(row.min(),y),(row.max(),y)]
P=np.array(pts,float)
Am=np.c_[2*P[:,0],2*P[:,1],np.ones(len(P))]; bm=(P**2).sum(1)
cx,cy,c=np.linalg.lstsq(Am,bm,rcond=None)[0]; r=np.sqrt(c+cx*cx+cy*cy)
print("circle", round(cx,1), round(cy,1), round(r,1))
inside = (xx-cx)**2+(yy-cy)**2 <= (r+2)**2

# 2) Лапа: тёплые тона в левой верхней зоне + сглаженные края.
box = (xx<160)&(yy>190)&(yy<338)
warm = (R>150)&(R-B>40)&(G>80)&(A>8)
paw = box & warm
paw = cv2.morphologyEx(paw.astype(np.uint8), cv2.MORPH_CLOSE, np.ones((5,5),np.uint8))
n, lab, stats, _ = cv2.connectedComponentsWithStats(paw)
keep = 1+np.argmax(stats[1:,cv2.CC_STAT_AREA]); paw = (lab==keep)
paw_d = cv2.dilate(paw.astype(np.uint8), np.ones((7,7),np.uint8)).astype(bool) & box
print("paw px", paw.sum(), "bbox", xs.min() if False else "", np.nonzero(paw)[1].min(), np.nonzero(paw)[1].max(), np.nonzero(paw)[0].min(), np.nonzero(paw)[0].max())

# 3) Слой лапы: исходные пиксели в расширенной маске, альфа мягкая по краю.
soft = cv2.GaussianBlur(paw_d.astype(np.float32), (5,5), 0)
paw_rgba = im.copy(); paw_rgba[...,3] = (A*np.clip(soft*1.2,0,1)).astype(np.int32)
y0,y1 = np.nonzero(paw_d)[0].min(), np.nonzero(paw_d)[0].max()+1
x0,x1 = np.nonzero(paw_d)[1].min(), np.nonzero(paw_d)[1].max()+1
Image.fromarray(paw_rgba[y0:y1,x0:x1].astype(np.uint8)).save(f"{out}/paw.png")

# 4) Тело без лапы: внутри круга — заливка по соседям (inpaint), снаружи — прозрачно.
rgb = im[...,:3].astype(np.uint8).copy()
rgb[A<10] = (0xb2,0xd9,0xfa)   # прозрачное вокруг — цветом круга, чтобы заливка не тянула тёмное или белое
hole = paw_d.astype(np.uint8)*255
filled = cv2.inpaint(rgb, hole, 9, cv2.INPAINT_TELEA)
# Под запястьем мех предплечья продолжается вверх: заливаем снизу мехом, а не фоном.
fur = paw_d & (yy>=292) & (xx>=30)
furfill = cv2.inpaint(im[...,:3].astype(np.uint8), (paw_d & ~(yy>=330)).astype(np.uint8)*255, 15, cv2.INPAINT_NS)
filled[fur] = furfill[fur]
body = im.copy()
body[...,:3] = np.where(paw_d[...,None], filled, rgb)
alpha = A.copy()
alpha[paw_d & ~inside] = 0
alpha[paw_d & inside] = 255
alpha[fur] = 255
body[...,3] = alpha
Image.fromarray(body.astype(np.uint8)).save(f"{out}/body.png")
print("paw crop", x0,y0,x1,y1, "size", W, H)
