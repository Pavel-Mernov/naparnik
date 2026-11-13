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

# 6) Закрытые и полуприкрытые глаза — слои поверх фигуры того же размера (eyes-closed.png, eyes-half.png).
#    Глаз = белок (самая большая белая область в зоне глаз, без зубов) + зрачки, которые его касаются.
#    Закрываем: глаз заливается мехом соседних участков (inpaint), сверху — фактура шерсти со лба;
#    по форме глаза рисуется тёмная линия сомкнутых век.
fig = cut.astype(np.uint8)
Rf, Gf, Bf, Af = [fig[...,i].astype(int) for i in range(4)]
zone = (xx >= 140) & (xx <= 450) & (yy >= 90) & (yy <= 270)
whites = ((Rf > 190) & (Gf > 190) & (Bf > 190) & (Af > 200) & zone).astype(np.uint8)
n_w, lab_w, st_w, _ = cv2.connectedComponentsWithStats(whites)
eye_white = lab_w == 1 + np.argmax(st_w[1:, cv2.CC_STAT_AREA])
dark = ((Rf < 60) & (Gf < 60) & (Bf < 70) & (Af > 200) & zone).astype(np.uint8)
n_d, lab_d, st_d, cen_d = cv2.connectedComponentsWithStats(dark)
near = cv2.dilate(eye_white.astype(np.uint8), np.ones((9, 9), np.uint8)).astype(bool)
pupils = np.zeros_like(eye_white)
for k in range(1, n_d):
    comp = lab_d == k
    rim = comp & ~cv2.erode(comp.astype(np.uint8), np.ones((3, 3), np.uint8)).astype(bool)
    touch = (rim & near).sum() / max(1, rim.sum())
    if st_d[k, cv2.CC_STAT_AREA] > 300 and touch > 0.45:   # зрачок почти весь окружён белком, нос — нет
        pupils |= comp
nose = np.zeros_like(eye_white)
for k in range(1, n_d):
    comp = lab_d == k
    if st_d[k, cv2.CC_STAT_AREA] > 300 and not (comp & pupils).any():
        nose |= comp
nose = cv2.dilate(nose.astype(np.uint8), np.ones((9, 9), np.uint8)).astype(bool)
eye = cv2.morphologyEx((eye_white | pupils).astype(np.uint8), cv2.MORPH_CLOSE, np.ones((9, 9), np.uint8)).astype(bool)
eye = ndimage.binary_fill_holes(eye)  # блики внутри зрачков — тоже глаз
eye = cv2.dilate(eye.astype(np.uint8), np.ones((5, 5), np.uint8)).astype(bool) & zone & ~nose
print("eye px", int(eye.sum()), "pupils px", int(pupils.sum()))

# Фактура шерсти: лоб над глазами, высокочастотная составляющая, плиткой.
fy0, fy1, fx0, fx1 = 370, 440, 210, 330   # ровный мех груди
patch = fig[fy0:fy1, fx0:fx1, :3].astype(np.float32)
detail = patch - cv2.GaussianBlur(patch, (0, 0), 3)
tile = np.tile(detail, (H // detail.shape[0] + 2, W // detail.shape[1] + 2, 1))[:H, :W]

FUR_TOP = np.array([30, 95, 205], np.float32)   # мех брови над глазом
FUR_LOW = np.array([16, 62, 160], np.float32)   # к краю века темнее

def lids_layer(fraction, name):
    layer = np.zeros((H, W, 4), np.uint8)
    for lo, hi in [(140, 300), (300, 451)]:          # каждый глаз отдельно
        eye1 = eye & (xx >= lo) & (xx < hi)
        cols = [x for x in range(lo, hi) if eye1[:, x].any()]
        if len(cols) < 20:
            continue
        cols = np.array(cols)
        top = np.array([np.nonzero(eye1[:, x])[0].min() for x in cols], float)
        bot = np.array([np.nonzero(eye1[:, x])[0].max() for x in cols], float)
        smooth = lambda v: np.convolve(np.pad(v, 12, mode="edge"), np.ones(25) / 25, mode="valid")
        frac_line = 0.62 if fraction > 0.9 else fraction
        edge = smooth(top + fraction * (bot - top))          # докуда опускается веко
        line = smooth(top + frac_line * (bot - top))          # где сомкнуты веки
        for x, t0, e in zip(cols, top, edge):
            ys_ = np.arange(int(t0), int(e) + 1)
            ys_ = ys_[eye1[ys_, x]]
            if not len(ys_):
                continue
            k = np.clip((ys_ - t0) / max(1, e - t0), 0, 1)[:, None]
            color = FUR_TOP * (1 - k) + FUR_LOW * k + tile[ys_, x] * 0.7
            layer[ys_, x, :3] = np.clip(color, 0, 255).astype(np.uint8)
            layer[ys_, x, 3] = 255
        sel = slice(5, -5)
        pts = np.c_[cols[sel], line[sel]].astype(np.int32).reshape(-1, 1, 2)
        if fraction > 0.9:
            cv2.polylines(layer, [pts + [0, 5]], False, (70, 130, 230, 170), 4, cv2.LINE_AA)  # нижнее веко чуть светлее
        cv2.polylines(layer, [pts], False, (10, 26, 80, 255), 8, cv2.LINE_AA)
    a = cv2.GaussianBlur(layer[..., 3].astype(np.float32), (5, 5), 0)
    layer[..., 3] = np.where(a > 10, np.maximum(a, np.where(layer[..., 3] > 0, 210, 0)), 0).astype(np.uint8)
    Image.fromarray(layer).save(f"{out}/{name}")

lids_layer(1.0, "eyes-closed.png")
lids_layer(0.5, "eyes-half.png")
