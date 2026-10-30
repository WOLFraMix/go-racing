from PIL import Image, ImageDraw
import colorsys

SRC = "assets/player_red.png"
DST = "assets/generator.png"

img = Image.open(SRC).convert("RGBA")
W, H = img.size
px = img.load()

# Перекраска: красные оттенки корпуса -> фиолетовые.
# Защищены: свет фар (кремовый), стёкла, колёса, белый, стоп-сигналы.
for y in range(H):
    for x in range(W):
        r, g, b, a = px[x, y]
        if a == 0:
            continue
        if y >= 131 and r > 230 and g < 110:   # стоп-сигналы — не трогаем
            continue
        if r >= g + 10 and r >= b + 10:        # красная семья (корпус, тени, блик)
            h, s, v = colorsys.rgb_to_hsv(r/255, g/255, b/255)
            h, s, v = 282/360, min(1.0, s*0.88), v*0.98
            nr, ng, nb = colorsys.hsv_to_rgb(h, s, v)
            px[x, y] = (round(nr*255), round(ng*255), round(nb*255), a)
        # всё остальное (фары, колёса, стёкла, полосы, контуры) остаётся

d = ImageDraw.Draw(img)
NAVY = (26, 34, 48, 255)
WING = (96, 40, 130, 255)      # тёмно-фиолетовое антикрыло
LOUV = (86, 34, 112, 255)

# жалюзи на капоте: по три щели слева и справа от центральных полос
for x0 in (28, 65):
    for yy in (24, 30, 36):
        d.rectangle([x0, yy, x0+13, yy+2], fill=LOUV)

# антикрыло: плоскость + тёмные стойки-торцы (свободная зона y142-152)
d.rectangle([11, 142, 17, 152], fill=NAVY)   # левый торец
d.rectangle([88, 142, 94, 152], fill=NAVY)   # правый торец
d.rounded_rectangle([16, 144, 89, 150], radius=3, fill=WING, outline=NAVY)

img.save(DST)
print("saved", DST)
