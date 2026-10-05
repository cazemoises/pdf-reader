"""Explainable OCR policy: bounded raster evidence, never image area alone."""
import re
import pymupdf
from layout import rect
from quality import evaluate


def glyph_components(pixmap, masks, max_components=50_000, gray_threshold=223):
    """Run-length connected components of dark pixels; no OCR engine or ML dependency."""
    pixels = bytearray(pixmap.samples)
    width, height = pixmap.width, pixmap.height
    for x0, y0, x1, y1 in masks:
        for y in range(max(0, y0), min(height, y1)):
            left, right = max(0, x0), min(width, x1)
            if right > left:
                pixels[y*width+left:y*width+right] = b'\xff'*(right-left)
    parent, boxes = [], []
    previous = []
    def root(i):
        while parent[i] != i:
            parent[i] = parent[parent[i]]
            i = parent[i]
        return i
    for y in range(height):
        current = []
        index = 0
        for match in re.finditer(b'[\x00-'+bytes([gray_threshold])+b']+', pixels[y*width:(y+1)*width]):
            left, right = match.span()
            while index < len(previous) and previous[index][1] < left:
                index += 1
            connections = []
            j = index
            while j < len(previous) and previous[j][0] <= right:
                connections.append(root(previous[j][2]))
                j += 1
            if not connections:
                component = len(parent)
                parent.append(component)
                boxes.append([left, y, right, y+1])
            else:
                component = connections[0]
                for other in connections[1:]:
                    if other != component:
                        parent[other] = component
                        a, b = boxes[component], boxes[other]
                        boxes[component] = [min(a[0], b[0]), min(a[1], b[1]), max(a[2], b[2]), max(a[3], b[3])]
                b = boxes[component]
                boxes[component] = [min(b[0], left), b[1], max(b[2], right), y+1]
            current.append((left, right, component))
        previous = current
        if len(parent) > max_components:
            return []  # Bounded inspection: very noisy images remain uncertain.
    return [b for i, b in enumerate(boxes) if parent[i] == i]


def image_text_evidence(page, images, native, settings):
    """Detect aligned glyph-sized components outside usable native text; LTR heuristic."""
    detected = []
    bounds = page.rect
    for image in images[:settings.max_inspected_images]:
        area = pymupdf.Rect(rect(image)) & bounds
        if area.is_empty or area.is_infinite:
            continue
        scale = min(1, settings.inspection_max_dimension/max(area.width, area.height))
        pix = page.get_pixmap(matrix=pymupdf.Matrix(scale, scale), clip=area, colorspace=pymupdf.csGRAY, alpha=False)
        masks = []
        for block in native:
            if evaluate([block])['invalid_ratio'] > settings.invalid_ratio_trigger:
                continue
            for line in block['lines']:
                r = pymupdf.Rect(rect(line)) & area
                if not r.is_empty:
                    masks.append((int(r.x0*scale)-pix.x-1, int(r.y0*scale)-pix.y-1,
                                  int(r.x1*scale)-pix.x+2, int(r.y1*scale)-pix.y+2))
        components = glyph_components(pix, masks, settings.max_inspection_components, settings.inspection_gray_threshold)
        glyphs = [b for b in components if 3 <= b[3]-b[1] <= 60 and 1 <= b[2]-b[0] <= 2*(b[3]-b[1])]
        rows = []
        for box in sorted(glyphs, key=lambda b: (b[1], b[0])):
            row = next((r for r in rows if abs(r[0][1]-box[1]) <= max(r[0][3]-r[0][1], box[3]-box[1])/2), None)
            if row is None:
                rows.append([box])
            else:
                row.append(box)
        # At least four neighboring glyphs on a baseline: avoids isolated icons and large shapes.
        for row in rows:
            row.sort(key=lambda b: b[0])
            run = 1
            for a, b in zip(row, row[1:]):
                run = run+1 if b[0]-a[2] <= 3*max(a[3]-a[1], b[3]-b[1]) else 1
                if run >= settings.min_text_components:
                    detected.append({'bbox': image['bbox'], 'aligned_components': run})
                    break
            if detected and detected[-1]['bbox'] == image['bbox']:
                break
    return detected


def decide(page, images, native, settings):
    quality = evaluate(native)
    reasons = []
    evidence = []
    if quality['invalid_ratio'] > settings.invalid_ratio_trigger:
        reasons.append('native_character_corruption')
    if images:
        evidence = image_text_evidence(page, images, native, settings)
        if evidence:
            reasons.append('raster_glyph_rows_outside_native_text')
    return {'requested': bool(reasons), 'executed': False, 'accepted': False,
            'reasons': reasons, 'skip_reason': None if reasons else 'no_uncovered_raster_text_evidence',
            'native_characters': quality['characters'], 'native_invalid_ratio': quality['invalid_ratio'],
            'image_count': len(images), 'inspected_images': min(len(images), settings.max_inspected_images),
            'raster_text_regions': evidence, 'mode': 'full' if 'native_character_corruption' in reasons else 'partial'}
