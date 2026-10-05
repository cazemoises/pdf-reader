"""PDF inspection -> native layout -> selective OCR -> tables -> structure -> quality."""
import hashlib
import logging
import time
from collections import Counter
import pymupdf
from layout import bbox, normalized, reading_order, mark_margins, rect
from quality import evaluate
from settings import Settings
from tables import has_ruled_grid
from ocr import decide

pymupdf.no_recommend_layout()
logger = logging.getLogger('pdf_reader.extraction')


class ExtractionError(ValueError):
    pass


def block_from_lines(lines, source='native'):
    rectangles = [rect(line) for line in lines]
    bounds = (min(r[0] for r in rectangles), min(r[1] for r in rectangles),
              max(r[2] for r in rectangles), max(r[3] for r in rectangles))
    return {'text': normalized('\n'.join(line['text'] for line in lines)), 'bbox': bbox(bounds),
            'type': 'paragraph', 'source': source, 'confidence': None, 'lines': lines}


def native_blocks(page, textpage=None, source='native'):
    data = page.get_text('dict', flags=pymupdf.TEXTFLAGS_DICT & ~pymupdf.TEXT_PRESERVE_IMAGES & ~pymupdf.TEXT_USE_CID_FOR_UNKNOWN_UNICODE, textpage=textpage)
    blocks = []
    for raw in data['blocks']:
        if raw['type'] != 0:
            continue
        groups = []
        for line in raw['lines']:
            text = normalized(''.join(span['text'] for span in line['spans']), preserve_soft_hyphens=True)
            if not text:
                continue
            origin = 'ocr' if textpage is not None and any(span['font'] == 'GlyphLessFont' for span in line['spans']) else 'native'
            item = {'text': text, 'bbox': bbox(line['bbox']), 'direction': list(line['dir']), 'source': origin}
            lr = rect(item)
            group = next((g for g in groups if max(g['left'], lr[0]) < min(g['right'], lr[2])), None)
            if group is None:
                group = {'left': lr[0], 'right': lr[2], 'lines': []}
                groups.append(group)
            group['left'] = max(group['left'], lr[0])
            group['right'] = min(group['right'], lr[2])
            group['lines'].append(item)
        for group in groups:
            lines = group['lines']
            if all(abs(line['direction'][0]) > 0.9 for line in lines):
                lines = sorted(lines, key=lambda line: rect(line)[1])
            blocks.append(block_from_lines(lines, source))
    return blocks


def merge_ocr(native, candidate, full, settings):
    # Always retain original usable native text; the OCR TextPage is not a replacement for it.
    ocr_lines = [line for block in candidate for line in block['lines'] if line['source'] == 'ocr']
    retained = []
    replaced = 0
    rejected = 0
    for block in native:
        damaged = full and evaluate([block])['invalid_ratio'] > settings.invalid_ratio_trigger
        area = pymupdf.Rect(rect(block))
        matching = []
        for line in ocr_lines:
            r = pymupdf.Rect(rect(line))
            if area.contains(pymupdf.Point((r.x0+r.x1)/2, (r.y0+r.y1)/2)):
                matching.append(line)
        candidate_text = '\n'.join(line['text'] for line in matching)
        usable_native_words = Counter(word.casefold() for word in block['text'].split() if '\ufffd' not in word)
        candidate_words = Counter(word.casefold() for word in candidate_text.split())
        improves = matching and evaluate([{'text': candidate_text}])['invalid_ratio'] < evaluate([block])['invalid_ratio']
        if damaged and improves and not (usable_native_words-candidate_words):
            replaced += 1
        else:
            if damaged and improves:
                rejected += 1
            retained.append(block)
    new_lines = []
    duplicates = 0
    for line in ocr_lines:
        lr = pymupdf.Rect(rect(line))
        overlaps_native = any((lr & pymupdf.Rect(rect(original))).get_area() >= 0.5*min(lr.get_area(), pymupdf.Rect(rect(original)).get_area())
                              for block in retained for original in block['lines'])
        if overlaps_native:
            duplicates += 1
        elif evaluate([line])['non_whitespace_characters'] and not any(line['text'] == b['text'] and (lr & pymupdf.Rect(rect(b))).get_area() >= 0.5*min(lr.get_area(), pymupdf.Rect(rect(b)).get_area()) for b in new_lines):
            new_lines.append(line)
    selected = {id(line) for line in new_lines}
    new = [block_from_lines(kept, 'ocr') for block in candidate if (kept := [line for line in block['lines'] if id(line) in selected])]
    return retained+new, {'added_ocr_lines': len(new_lines), 'replaced_corrupt_blocks': replaced,
                          'suppressed_overlapping_ocr_lines': duplicates, 'rejected_corrupt_replacements': rejected}


def extract_tables(page, blocks):
    if not has_ruled_grid(page.get_drawings()):
        return blocks, []
    warnings = []
    for table in page.find_tables().tables:
        cells = [[normalized(cell or '') for cell in row] for row in table.extract()]
        area = pymupdf.Rect(table.bbox)
        inside = [line for block in blocks for line in block.get('lines', []) if area.contains(pymupdf.Rect(rect(line)))]
        native_words = Counter(' '.join(line['text'] for line in inside).split())
        cell_words = Counter(' '.join(cell for row in cells for cell in row).split())
        if not cell_words or native_words-cell_words:
            warnings.append('table_candidate_rejected_text_loss')
            continue
        remaining = []
        for block in blocks:
            lines = block.get('lines')
            if lines is None:
                remaining.append(block)
                continue
            kept = [line for line in lines if not area.contains(pymupdf.Rect(rect(line)))]
            if len(kept) == len(lines):
                remaining.append(block)
            elif kept:
                remaining.append(block_from_lines(kept, block['source']))
        remaining.append({'type': 'table', 'source': 'native-table', 'confidence': None,
                          'bbox': bbox(table.bbox), 'cells': cells,
                          'text': '\n'.join(' | '.join(' '.join(cell.splitlines()) for cell in row) for row in cells)})
        blocks = remaining
    return blocks, warnings


def extract_page(page, number, settings):
    started = time.perf_counter()
    rotation = page.rotation
    # OCR rendering honors rotation but native coordinates do not. Normalize in this private document.
    page.set_rotation(0)
    try:
        bounds = page.rect
        blocks = native_blocks(page)
        native_quality = evaluate(blocks)
        images = [{'bbox': bbox(i['bbox']), 'width': i['width'], 'height': i['height']} for i in page.get_image_info()]
        if any(image['width']*image['height'] > settings.max_source_image_pixels for image in images):
            raise ExtractionError('source image pixel limit exceeded')
        decision = decide(page, images, blocks, settings)
        strategy = 'native-layout'
        warnings = []
        if len(images) > settings.max_inspected_images:
            warnings.append('image_inspection_budget_exceeded')
        if decision['requested']:
            if not settings.ocr_enabled:
                decision['skip_reason'] = 'ocr_disabled'
            elif bounds.get_area()*(settings.ocr_dpi/72)**2 > settings.max_page_pixels:
                decision['skip_reason'] = 'ocr_pixel_budget_exceeded'
            else:
                try:
                    decision['executed'] = True
                    full = decision['mode'] == 'full'
                    tp = page.get_textpage_ocr(language=settings.ocr_language, dpi=settings.ocr_dpi, full=full)
                    candidate = native_blocks(page, tp, 'ocr' if full else 'native+ocr')
                    merged, merge_signals = merge_ocr(blocks, candidate, full, settings)
                    decision.update(merge_signals)
                    decision['accepted'] = bool(merge_signals['added_ocr_lines'] or merge_signals['replaced_corrupt_blocks'])
                    if decision['accepted']:
                        blocks = merged
                        strategy = 'ocr-full' if full else 'ocr-partial'
                    else:
                        decision['skip_reason'] = 'ocr_no_new_usable_text'
                except RuntimeError:
                    decision['skip_reason'] = 'ocr_unavailable_or_failed'
            if decision['skip_reason']:
                warnings.append(decision['skip_reason'])
        if len(blocks) > settings.max_blocks:
            raise ExtractionError('page block budget exceeded')
        if strategy == 'native-layout':
            try:
                blocks, table_warnings = extract_tables(page, blocks)
                warnings.extend(table_warnings)
            except (RuntimeError, ValueError):
                warnings.append('table_extraction_failed')
        blocks = reading_order(blocks)
        if any(abs(line['direction'][0]) < 0.9 for block in blocks for line in block.get('lines', [])):
            warnings.append('non_horizontal_text_order_uncertain')
        quality = evaluate(blocks)
        if not blocks:
            warnings.append('image_without_text' if images else 'empty_page')
        result = {'page_number': number, 'width': bounds.height if rotation in (90, 270) else bounds.width,
                  'height': bounds.width if rotation in (90, 270) else bounds.height,
                  'coordinate_space': 'unrotated', 'coordinate_width': bounds.width, 'coordinate_height': bounds.height,
                  'rotation': rotation, 'blocks': blocks, 'images': images, 'strategy': strategy,
                  'fallback_requested': decision['requested'], 'ocr': decision, 'quality': quality,
                  'native_quality': native_quality, 'warnings': warnings,
                  'duration_ms': round((time.perf_counter()-started)*1000, 3)}
        logger.info('page=%s strategy=%s ocr_reasons=%s ocr_executed=%s ocr_accepted=%s skip_reason=%s duration_ms=%s characters=%s warnings=%s',
                    number, strategy, decision['reasons'], decision['executed'], decision['accepted'],
                    decision['skip_reason'], result['duration_ms'], quality['characters'], warnings)
        return result
    finally:
        page.set_rotation(rotation)


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
            pages = [extract_page(page, number, settings) for number, page in enumerate(doc, 1)]
            mark_margins([dict(page, height=page['coordinate_height']) for page in pages], settings.margin_fraction)
            document_id = hashlib.sha256(data).hexdigest()[:16]
            logger.info('document=%s pages=%s stage=complete', document_id, len(pages))
            return {'schema_version': 2, 'document_id': document_id, 'pages': pages}
    except (pymupdf.FileDataError, pymupdf.EmptyFileError) as exc:
        raise ExtractionError('invalid PDF') from exc
