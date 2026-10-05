"""Geometry ordering and loss-conscious normalization, independent of the PDF loader."""
import unicodedata


def normalized(text):
    # Keep ordinary hyphens: distinguishing line-wrap hyphens from compounds needs language context.
    return unicodedata.normalize('NFC', text).replace('\u00ad\n', '').replace('\u00ad', '').strip()


def bbox(rect):
    x0, y0, x1, y1 = rect
    return {'x': x0, 'y': y0, 'width': x1-x0, 'height': y1-y0}


def rect(block):
    b = block['bbox']
    return b['x'], b['y'], b['x']+b['width'], b['y']+b['height']


def reading_order(blocks, depth=0):
    """Recursive whitespace cuts: vertical gutters before horizontal splits (LTR)."""
    if len(blocks) < 2 or depth >= 32:
        return sorted(blocks, key=lambda b: (rect(b)[1], rect(b)[0]))
    for axis in (0, 1):
        ordered = sorted(blocks, key=lambda b: rect(b)[axis])
        end = rect(ordered[0])[axis+2]
        gaps = []
        for i in range(1, len(ordered)):
            start = rect(ordered[i])[axis]
            if start > end:
                gaps.append((start-end, i))
            end = max(end, rect(ordered[i])[axis+2])
        if gaps:
            _, split = max(gaps)
            return reading_order(ordered[:split], depth+1) + reading_order(ordered[split:], depth+1)
    return sorted(blocks, key=lambda b: (rect(b)[1], rect(b)[0]))


def mark_margins(pages, fraction):
    """Retain repeated marginal text as semantic blocks; omit only in prose serialization."""
    if len(pages) < 3:
        return
    occurrences = {}
    for page in pages:
        for block in page['blocks']:
            x0, y0, x1, y1 = rect(block)
            role = 'header' if y1 < page['height']*fraction else 'footer' if y0 > page['height']*(1-fraction) else None
            if role:
                key = (role, '#' if block['text'].isdigit() else block['text'])
                occurrences.setdefault(key, []).append((page['page_number'], block))
    for (role, _), entries in occurrences.items():
        if len({n for n, _ in entries}) >= max(3, (len(pages)+1)//2):
            for _, block in entries:
                block['type'] = role
