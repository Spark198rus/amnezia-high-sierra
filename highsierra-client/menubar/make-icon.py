#!/usr/bin/env python3
"""Draws menubar/AppIcon.png, the app icon: a white shield with a check mark
on a blue tile. The shield has the same outline as the menu bar icon
(Icons.m). Needs Pillow; run it from anywhere:

    python3 menubar/make-icon.py

"make app" turns the PNG into AppIcon.icns with sips and iconutil.
"""

import os

from PIL import Image, ImageDraw, ImageFilter

SIZE = 1024
SCALE = 4  # draw larger, then shrink, for smooth edges
TOP = (64, 128, 255)
BOTTOM = (22, 56, 158)
CHECK = (34, 86, 214)


def bezier(p0, p1, p2, p3, steps=48):
    points = []
    for i in range(1, steps + 1):
        t = i / steps
        u = 1 - t
        points.append((
            u**3 * p0[0] + 3 * u * u * t * p1[0] + 3 * u * t * t * p2[0] + t**3 * p3[0],
            u**3 * p0[1] + 3 * u * u * t * p1[1] + 3 * u * t * t * p2[1] + t**3 * p3[1],
        ))
    return points


def shield(cx, cy, k):
    """The Icons.m shield (an 18-point box, y up), centered at cx, cy, k px per point."""
    def at(x, y):
        return (cx + (x - 9) * k, cy - (y - 9) * k)

    pts = [at(9, 16.5)]
    pts += bezier(at(9, 16.5), at(7.2, 15.2), at(5.3, 14.4), at(3.5, 14.2))
    pts.append(at(3.5, 9))
    pts += bezier(at(3.5, 9), at(3.5, 5.2), at(6, 2.8), at(9, 1.5))
    pts += bezier(at(9, 1.5), at(12, 2.8), at(14.5, 5.2), at(14.5, 9))
    pts.append(at(14.5, 14.2))
    pts += bezier(at(14.5, 14.2), at(12.7, 14.4), at(10.8, 15.2), at(9, 16.5))
    return pts


def main():
    s = SIZE * SCALE
    icon = Image.new("RGBA", (s, s), (0, 0, 0, 0))

    # The tile, with a soft shadow below it.
    inset, radius = 100 * SCALE, 185 * SCALE
    tile = (inset, inset, s - inset, s - inset)
    shadow = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    ImageDraw.Draw(shadow).rounded_rectangle(
        (tile[0], tile[1] + 14 * SCALE, tile[2], tile[3] + 14 * SCALE), radius, fill=(0, 0, 0, 90))
    icon.alpha_composite(shadow.filter(ImageFilter.GaussianBlur(22 * SCALE)))

    gradient = Image.new("RGBA", (s, s))
    top, bottom = tile[1], tile[3]
    for y in range(s):
        t = min(max((y - top) / (bottom - top), 0), 1)
        color = tuple(round(a + (b - a) * t) for a, b in zip(TOP, BOTTOM)) + (255,)
        ImageDraw.Draw(gradient).line((0, y, s, y), fill=color)
    mask = Image.new("L", (s, s), 0)
    ImageDraw.Draw(mask).rounded_rectangle(tile, radius, fill=255)
    icon.paste(gradient, (0, 0), mask)

    # The shield and its check mark.
    draw = ImageDraw.Draw(icon)
    draw.polygon(shield(s / 2, s / 2 + 8 * SCALE, 37 * SCALE), fill=(250, 251, 255, 255))
    width = 70 * SCALE
    check = [(s / 2 + x * SCALE, s / 2 + y * SCALE) for x, y in ((-118, 22), (-32, 106), (128, -78))]
    draw.line(check, fill=CHECK + (255,), width=width, joint="curve")
    for x, y in (check[0], check[-1]):
        r = width / 2
        draw.ellipse((x - r, y - r, x + r, y + r), fill=CHECK + (255,))

    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "AppIcon.png")
    icon.resize((SIZE, SIZE), Image.LANCZOS).save(out, optimize=True)
    print("wrote", out)


if __name__ == "__main__":
    main()
