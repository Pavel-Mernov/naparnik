#!/usr/bin/env python3
"""Слои маскота для анимации (frontend/src/shared/assets/mascot): figure.png — персонаж без круга-подложки
и без поднятой лапы, paw.png — лапа. Круг рисуется стилями, чтобы персонаж мог лечь внутри него.

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
body_rgba = body  # тело с кругом — промежуточный слой, в вёрстке используется figure.png
print("paw crop", x0,y0,x1,y1, "size", W, H)

# 5) Персонаж без круга-подложки: круг рисуется стилями и стоит на месте, а персонаж может лечь внутри.
#    Фон — светло-голубые пиксели внутри круга, связанные с самим кругом (белки глаз не задеваются).
#    Цвет круга с его градиентом восстанавливаем по всему кругу; край меха «отмываем» от него (unmix).
from scipy import ndimage
bf = body.astype(float)
Rb, Gb, Bb, Ab = bf[...,0], bf[...,1], bf[...,2], bf[...,3]
bluish = (Bb >= 215) & (Gb >= 190) & (Bb - Rb >= 15) & (Ab > 0)
lab_bg, _ = ndimage.label(bluish)
seed_ids = np.unique(lab_bg[circ & bluish]); seed_ids = seed_ids[seed_ids > 0]
core = np.isin(lab_bg, seed_ids)
# Внешний край круга по всем светло-голубым пикселям, включая полупрозрачные: подгоняем окружность заново.
edge_px = (Bb >= 170) & (Bb - Rb >= 15) & (Gb >= 140) & (Ab > 0) & ~paw_d
ey, ex = np.nonzero(edge_px)
pts = []
for y in np.unique(ey):
    row = ex[ey == y]; pts += [(row.min(), y), (row.max(), y)]
P2 = np.array(pts, float)
keep_pts = np.sqrt((P2[:,0]-cx)**2 + (P2[:,1]-cy)**2) > r - 30  # только точки у края, не внутренние пятна
P2 = P2[keep_pts]
A2 = np.c_[2*P2[:,0], 2*P2[:,1], np.ones(len(P2))]; b2 = (P2**2).sum(1)
cx2, cy2, c2 = np.linalg.lstsq(A2, b2, rcond=None)[0]; r2 = np.sqrt(c2 + cx2*cx2 + cy2*cy2)
print("outer circle", round(cx2,1), round(cy2,1), round(r2,1))
dist = np.sqrt((xx - cx2) ** 2 + (yy - cy2) ** 2)
ring = (dist >= r2 - 12) & (dist <= r2 + 6) & (Bb >= 180) & (Bb - Rb >= 15) & (Gb >= 150)  # сглаженный край круга
core |= ring | (paw_d & ~fur & (dist <= r2 + 6))  # место под лапой — это фон круга
core &= ~fur
disc = cv2.dilate(core.astype(np.uint8), np.ones((25,25), np.uint8)).astype(bool)  # где искать край
holes = (~core & disc).astype(np.uint8) * 255
field = cv2.inpaint(bf[...,:3].astype(np.uint8), holes, 7, cv2.INPAINT_TELEA).astype(float)  # цвет круга везде
FUR = np.array([10, 70, 185], float)
band = cv2.dilate(core.astype(np.uint8), np.ones((7,7), np.uint8)).astype(bool) & ~core & (Bb - Rb >= 25)
v = field - FUR
t = ((bf[...,:3] - FUR) * v).sum(-1) / np.maximum((v * v).sum(-1), 1)
a_edge = np.clip(1 - t, 0, 1)
alpha_c = Ab.copy()
alpha_c[core] = 0
alpha_c[band] = (a_edge * Ab)[band]
safe = np.maximum(a_edge, 0.05)[...,None]
unmixed = np.clip((bf[...,:3] - (1 - a_edge)[...,None] * field) / safe, 0, 255)
cut = bf.copy()
cut[...,:3] = np.where(band[...,None], unmixed, bf[...,:3])
cut[...,3] = alpha_c
# Остатки круга (заливка под лапой, ореол края) вне глаз — прозрачные: при повороте фигуры они видны пятном.
eyes_box = (xx >= 150) & (xx <= 440) & (yy >= 80) & (yy <= 310)
leftover = (cut[...,2] >= 190) & (cut[...,1] >= 160) & (cut[...,2] - cut[...,0] >= 12) & ~eyes_box
cut[leftover, 3] = 0
print("leftover removed", int(leftover.sum()))
Image.fromarray(cut.astype(np.uint8)).save(f"{out}/figure.png")
print("circle box", round(cx2,1), round(cy2,1), round(r2,1))
