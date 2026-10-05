"""PDF inspection -> native layout -> selective OCR -> tables -> structure -> quality."""
import hashlib
import logging
import time
import pymupdf
from layout import bbox, normalized, reading_order, mark_margins, rect
from quality import evaluate
from settings import Settings
from tables import has_ruled_grid

pymupdf.no_recommend_layout()

logger = logging.getLogger('pdf_reader.extraction')


class ExtractionError(ValueError):
    pass


def native_blocks(page, textpage=None, source='native'):
    data = page.get_text('dict', flags=pymupdf.TEXTFLAGS_DICT & ~pymupdf.TEXT_PRESERVE_IMAGES, textpage=textpage)
    blocks = []
    for raw in data['blocks']:
        if raw['type'] != 0:
            continue
        lines = []
        for line in raw['lines']:
            text = normalized(''.join(s['text'] for s in line['spans']))
            if text:
                lines.append({'text': text, 'bbox': bbox(line['bbox']), 'direction': list(line['dir'])})
        # PDF text blocks can span two columns; split disjoint line x-intervals first.
        groups = []
        for line in lines:
            lr = rect(line)
            group = next((g for g in groups if max(g['left'], lr[0]) < min(g['right'], lr[2])), None)
            if group is None:
                group = {'left': lr[0], 'right': lr[2], 'lines': []}
                groups.append(group)
            group['left'] = max(group['left'], lr[0])
            group['right'] = min(group['right'], lr[2])
            group['lines'].append(line)
        for group in groups:
            group_lines = sorted(group['lines'], key=lambda l: rect(l)[1])
            rectangles = [rect(l) for l in group_lines]
            bounds = (min(r[0] for r in rectangles), min(r[1] for r in rectangles),
                      max(r[2] for r in rectangles), max(r[3] for r in rectangles))
            blocks.append({'text': normalized('\n'.join(l['text'] for l in group_lines)),
                           'bbox': bbox(bounds), 'type': 'paragraph', 'source': source,
                           'confidence': None, 'lines': group_lines})
    return blocks


def extract_page(page, number, settings):
    started = time.perf_counter()
    # PyMuPDF text coordinates are unrotated; use that same coordinate space throughout.
    bounds = page.rect * page.derotation_matrix
    blocks = native_blocks(page)
    native_quality = evaluate(blocks)
    images = [{'bbox': bbox(i['bbox']), 'width': i['width'], 'height': i['height']} for i in page.get_image_info()]
    image_coverage = max((pymupdf.Rect(i['bbox']['x'], i['bbox']['y'], i['bbox']['x']+i['bbox']['width'], i['bbox']['y']+i['bbox']['height']) & bounds).get_area()/max(1, bounds.get_area()) for i in images) if images else 0
    bad_text = native_quality['invalid_ratio'] > settings.invalid_ratio_trigger
    needs_ocr = bad_text or (bool(images) and not blocks) or image_coverage >= settings.image_area_trigger
    strategy = 'native-layout'
    warnings = []
    if needs_ocr:
        if not settings.ocr_enabled:
            warnings.append('ocr_disabled')
        elif bounds.get_area()*(settings.ocr_dpi/72)**2 > settings.max_page_pixels:
            warnings.append('ocr_pixel_budget_exceeded')
        else:
            try:
                tp = page.get_textpage_ocr(language=settings.ocr_language, dpi=settings.ocr_dpi, full=bad_text)
                candidate = native_blocks(page, tp, 'ocr' if bad_text or not blocks else 'native+ocr')
                cq = evaluate(candidate)
                # Accept only nonempty, no worse character integrity, preserving native character coverage.
                if cq['characters'] >= native_quality['characters'] and cq['invalid_ratio'] <= native_quality['invalid_ratio']:
                    blocks = candidate
                    strategy = 'ocr-full' if bad_text else 'ocr-partial'
                else:
                    warnings.append('ocr_candidate_rejected')
            except RuntimeError:
                warnings.append('ocr_unavailable_or_failed')
    if len(blocks) > settings.max_blocks:
        raise ExtractionError('page block budget exceeded')
    # Ruled native tables only; OCR/unstyled tables remain explicitly unsupported.
    if strategy == 'native-layout':
        try:
            if has_ruled_grid(page.get_drawings()):
                for table in page.find_tables().tables:
                    cells = table.extract()
                    tr = pymupdf.Rect(table.bbox)
                    blocks = [b for b in blocks if not tr.contains(pymupdf.Rect(rect(b))) ]
                    blocks.append({'type': 'table', 'source': 'native-table', 'confidence': None,
                                   'bbox': bbox(table.bbox), 'cells': cells,
                                   'text': '\n'.join(' | '.join(normalized(c or '') for c in row) for row in cells)})
        except (RuntimeError, ValueError):
            warnings.append('table_extraction_failed')
    blocks = reading_order(blocks)
    quality = evaluate(blocks)
    if not blocks:
        warnings.append('image_without_text' if images else 'empty_page')
    result = {'page_number': number, 'width': page.rect.width, 'height': page.rect.height,
              'coordinate_space': 'unrotated', 'coordinate_width': bounds.width, 'coordinate_height': bounds.height,
              'rotation': page.rotation, 'blocks': blocks, 'images': images, 'strategy': strategy,
              'fallback_requested': needs_ocr, 'quality': quality, 'native_quality': native_quality,
              'warnings': warnings, 'duration_ms': round((time.perf_counter()-started)*1000, 3)}
    logger.info('page=%s strategy=%s fallback=%s duration_ms=%s characters=%s score=%s warnings=%s',
                number, strategy, needs_ocr, result['duration_ms'], quality['characters'], quality['score'], warnings)
    return result


def extract_document(data, settings=None):
    settings = settings or Settings.from_env()
    if len(data) > settings.max_bytes:
        raise ExtractionError('file size limit exceeded')
    try:
        with pymupdf.open(stream=data, filetype='pdf') as doc:
            if doc.needs_pass:
                raise ExtractionError('encrypted PDF requires a password')
            if not 0 < len(doc) <= settings.max_pages:
                raise ExtractionError('page count limit exceeded')
            pages = [extract_page(p, n, settings) for n, p in enumerate(doc, 1)]
            # Marginal classification uses unrotated geometry.
            unrotated = [dict(p, height=p['coordinate_height']) for p in pages]
            mark_margins(unrotated, settings.margin_fraction)
            logger.info('document=%s pages=%s stage=complete', hashlib.sha256(data).hexdigest()[:16], len(pages))
            return {'schema_version': 2, 'document_id': hashlib.sha256(data).hexdigest()[:16], 'pages': pages}
    except (pymupdf.FileDataError, pymupdf.EmptyFileError) as exc:
        raise ExtractionError('invalid PDF') from exc
