"""Observable coverage evidence, separate from the recovery decision."""
import pymupdf
from ocr import glyph_components
from layout import rect
from quality import evaluate


def vector_evidence(page, paths, native, minimum=4):
    usable = [pymupdf.Rect(rect(line)) for b in native if evaluate([b])['invalid_ratio'] == 0
              for line in b.get('lines', [])]
    glyphs = []
    for path in paths:
        area = path['rect'] & page.rect
        if (path['type'] not in ('f', 'fs') or not path.get('fill') or max(path['fill']) >= .9
                or area.is_empty or area.height > page.rect.height / 10
                or area.width > 3 * area.height
                or not any(item[0] == 'c' for item in path['items'])):
            continue
        if any(r.contains(area) for r in usable):
            continue
        glyphs.append(area)
    rows = []
    for area in sorted(glyphs, key=lambda r: (r.y1, r.x0)):
        row = next((row for row in rows if abs(row[0].y1-area.y1) <= min(row[0].height, area.height)*.3), None)
        if row is None:
            rows.append([area])
        else:
            row.append(area)
    evidence = []
    for row in rows:
        row.sort(key=lambda r: r.x0)
        run = [row[0]]
        for area in row[1:]:
            if area.x0-run[-1].x1 > 3*max(area.height, run[-1].height):
                if len(run) >= minimum:
                    evidence.append(run)
                run = []
            run.append(area)
        if len(run) >= minimum:
            evidence.append(run)
    rendered = False
    # Some producers combine all outline contours into one path. A cheap path
    # complexity gate precedes a bounded render; a vector border never qualifies.
    if not evidence and not native and any(len(path['items']) >= minimum*4 for path in paths):
        scale = min(1, 600/max(page.rect.width, page.rect.height))
        pix = page.get_pixmap(matrix=pymupdf.Matrix(scale, scale), colorspace=pymupdf.csGRAY)
        components = glyph_components(pix, [])
        candidates = [b for b in components if 3 <= b[3]-b[1] <= 60 and 1 <= b[2]-b[0] <= 2*(b[3]-b[1])]
        groups = []
        for box in sorted(candidates, key=lambda b: (b[3], b[0])):
            group = next((g for g in groups if abs(g[0][3]-box[3]) <= min(g[0][3]-g[0][1],box[3]-box[1])*.3), None)
            if group is None:
                groups.append([box])
            else:
                group.append(box)
        evidence = [g for g in groups if len(g) >= minimum]
        rendered = True
    return {'glyph_paths': len(glyphs), 'rows': len(evidence),
            'aligned_glyphs': sum(len(row) for row in evidence), 'inspection_rendered': rendered}


def ocr_line_supported(page, line, settings):
    """Reject geometrically unsupported OCR, never by book or vocabulary.

    This is not a semantic confidence estimate. A drawing resembling a glyph
    can still pass, and connected lettering must not be rejected by word length.
    """
    area = pymupdf.Rect(rect(line))
    if area.is_empty or area.is_infinite or not page.rect.contains(area):
        return False
    scale = settings.ocr_dpi/72
    pix = page.get_pixmap(matrix=pymupdf.Matrix(scale, scale), clip=area,
                          colorspace=pymupdf.csGRAY, alpha=False)
    components = glyph_components(pix, [], settings.max_inspection_components,
                                  settings.inspection_gray_threshold)
    height = area.height*scale
    return any(.3*height <= b[3]-b[1] <= 1.2*height
               and .04*height <= b[2]-b[0] <= 2*(b[3]-b[1]) for b in components)
