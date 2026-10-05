from dataclasses import replace
from collections import Counter
import pymupdf
import pytest
from adversarial import columns, image_pdf, margins, ruled_table, corrupted_font, pdf_page
from pipeline import extract_document
from settings import Settings


def text_of(page):
    return '\n'.join(b['text'] for b in page['blocks'])


@pytest.mark.parametrize('count,title', [(2, False), (3, False), (2, True), (3, True)])
def test_columns_exact(count, title):
    data, expected = columns(count, title)
    page = extract_document(data)['pages'][0]
    assert text_of(page).splitlines() == expected


def test_large_decorative_image_not_ocr(monkeypatch):
    def unexpected(*args, **kwargs):
        raise AssertionError('decorative image must not invoke Tesseract')
    monkeypatch.setattr(pymupdf.Page, 'get_textpage_ocr', unexpected)
    page = extract_document(image_pdf(native='Legitimate short text.', decorative=True))['pages'][0]
    assert not page['fallback_requested']
    assert text_of(page) == 'Legitimate short text.'


def test_small_scanned_region_in_hybrid():
    page = extract_document(image_pdf(text='SCANNED REGION', native='Native paragraph.', small=True))['pages'][0]
    assert page['fallback_requested']
    assert text_of(page).count('Native paragraph.') == 1
    assert text_of(page).count('SCANNED REGION') == 1


@pytest.mark.parametrize('rotation', [0, 90, 180, 270])
def test_portuguese_scan_rotations(rotation):
    page = extract_document(image_pdf(rotation=rotation))['pages'][0]
    assert text_of(page).strip() == 'AÇÃO, CAFÉ, CORAÇÃO'
    assert page['rotation'] == rotation
    for block in page['blocks']:
        b = block['bbox']
        assert 0 <= b['x'] <= page['coordinate_width']
        assert 0 <= b['y'] <= page['coordinate_height']
        assert b['x']+b['width'] <= page['coordinate_width']+1
        assert b['y']+b['height'] <= page['coordinate_height']+1


def test_searchable_scan_no_duplicates():
    page = extract_document(image_pdf(text='SCANNED DOCUMENT', searchable=True))['pages'][0]
    assert text_of(page).count('SCANNED DOCUMENT') == 1


def test_corrupt_font_without_image_can_recover():
    data = corrupted_font()
    with pymupdf.open(stream=data, filetype='pdf') as doc:
        assert '\ufffd' in doc[0].get_text(flags=pymupdf.TEXTFLAGS_TEXT & ~pymupdf.TEXT_USE_CID_FOR_UNKNOWN_UNICODE)
    page = extract_document(data)['pages'][0]
    assert 'ACAO CORRETA' in text_of(page)
    assert page['fallback_requested']


def test_nul_not_left_in_persisted_json():
    page = extract_document(corrupted_font(nul=True))['pages'][0]
    assert '\x00' not in text_of(page)


def test_margins_do_not_erase_repeated_body():
    pages = extract_document(margins())['pages']
    for page in pages[1:4]:
        assert any(b['text'] == 'Repeated legitimate body' and b['type'] == 'paragraph' for b in page['blocks'])
    assert 'Cover or appendix' in text_of(pages[0])
    assert 'Cover or appendix' in text_of(pages[-1])


def test_repeated_legitimate_top_paragraph_is_not_deleted():
    with pymupdf.open() as doc:
        for _ in range(3):
            page = doc.new_page(width=600, height=800)
            page.insert_text((50, 25), 'Legitimate repeated opening.')
            page.insert_text((50, 60), 'Continuation of the body.')
        result = extract_document(doc.tobytes())
    for page in result['pages']:
        assert all(not b.get('exclude_from_text', b['type'] in ('header', 'footer')) for b in page['blocks'])


@pytest.mark.parametrize('multiline,empty', [(False, False), (True, False), (False, True)])
def test_table_cells_exact(multiline, empty):
    page = extract_document(ruled_table(multiline=multiline, empty=empty))['pages'][0]
    table = next(b for b in page['blocks'] if b['type'] == 'table')
    assert table['cells'] == [['Name', 'Value'], ['Alpha\nBeta' if multiline else 'Alpha', '' if empty else '42']]
    assert text_of(page).count('Alpha') == 1


def test_table_crossing_block_does_not_duplicate():
    page = extract_document(ruled_table(near_prose=True))['pages'][0]
    assert text_of(page).count('Cell ending') == 1
    assert text_of(page).count('Outside prose') == 1


@pytest.mark.parametrize('ruled', [False, True])
def test_aligned_prose_not_table(ruled):
    def draw(page):
        page.insert_text((50, 100), 'Left prose')
        page.insert_text((330, 100), 'Sidebar prose')
        if ruled:
            page.draw_rect((40, 70, 220, 130))
            page.draw_rect((300, 70, 500, 130))
    page = extract_document(pdf_page(draw))['pages'][0]
    assert not any(b['type'] == 'table' for b in page['blocks'])
    assert Counter(text_of(page).splitlines()) == Counter(['Left prose', 'Sidebar prose'])


def test_spanning_heading_near_columns_not_row_interleaving():
    expected = ['A heading spanning the document columns', 'A1', 'A2', 'B1', 'B2']
    def draw(page):
        page.insert_text((40, 80), expected[0], fontsize=24)
        for i in reversed(range(4)):
            page.insert_text((40+(i//2)*300, 110+(i%2)*200), expected[i+1])
    assert text_of(extract_document(pdf_page(draw))['pages'][0]).splitlines() == expected


def test_sidebox_and_unusual_positions_preserve_exact_content():
    def draw(page):
        page.insert_text((430, 230), 'Side note')
        page.insert_text((10, 350), 'Body continuation')
        page.insert_text((10, 100), 'Body opening')
    assert text_of(extract_document(pdf_page(draw))['pages'][0]).splitlines() == ['Body opening', 'Body continuation', 'Side note']


def test_overlapping_native_elements_are_not_silently_deduplicated():
    def draw(page):
        page.insert_text((50, 100), 'FIRST')
        page.insert_text((50, 100), 'SECOND')
    result = extract_document(pdf_page(draw))['pages'][0]
    assert Counter(text_of(result).splitlines()) == Counter(['FIRST', 'SECOND'])


def test_low_resolution_image_decision_and_result_are_observable():
    page = extract_document(image_pdf(text='DOCUMENTO LEGIVEL', low_resolution=True))['pages'][0]
    assert page['ocr']['executed']
    assert page['ocr']['reasons'] == ['raster_glyph_rows_outside_native_text']
    assert text_of(page) == 'DOCUMENTO LEGIVEL'


def test_partial_ocr_cannot_replace_usable_native_words():
    from pipeline import merge_ocr, block_from_lines
    def line(text, y, source):
        return {'text': text, 'bbox': {'x': 50, 'y': y, 'width': 200, 'height': 20}, 'source': source, 'direction': [1, 0]}
    native = [block_from_lines([line('Valuable native words.', 50, 'native')])]
    # Deliberately wrong native content in an OCR adapter's TextPage; add a separate OCR region.
    candidate = [block_from_lines([line('Wrong longer native replacement!', 50, 'native'), line('Scanned words.', 200, 'ocr')])]
    result, signals = merge_ocr(native, candidate, False, Settings())
    assert [b['text'] for b in result] == ['Valuable native words.', 'Scanned words.']
    assert signals['added_ocr_lines'] == 1


def test_failed_corrupt_replacement_preserves_readable_native_tokens():
    from pipeline import merge_ocr, block_from_lines
    def line(text, source):
        return {'text': text, 'bbox': {'x': 50, 'y': 50, 'width': 200, 'height': 20}, 'source': source, 'direction': [1, 0]}
    native = [block_from_lines([line('KEEP THESE WORDS ��������', 'native')])]
    candidate = [block_from_lines([line('Other words entirely', 'ocr')])]
    result, signals = merge_ocr(native, candidate, True, Settings())
    assert result[0]['text'] == native[0]['text']
    assert signals['rejected_corrupt_replacements'] == 1


def test_table_candidate_that_loses_words_is_rejected(monkeypatch):
    class BrokenTable:
        bbox = (50, 100, 450, 200)
        def extract(self):
            return [['Name', 'Value'], ['', '42']]
    class Finder:
        tables = [BrokenTable()]
    monkeypatch.setattr(pymupdf.Page, 'find_tables', lambda *args, **kwargs: Finder())
    page = extract_document(ruled_table())['pages'][0]
    assert 'Alpha' in text_of(page)
    assert not any(b['type'] == 'table' for b in page['blocks'])
    assert 'table_candidate_rejected_text_loss' in page['warnings']


def test_ocr_paragraph_lines_are_not_turned_into_separate_paragraphs():
    from pipeline import merge_ocr, block_from_lines
    lines = [{'text': text, 'bbox': {'x': 50, 'y': 100+i*20, 'width': 200, 'height': 15}, 'source': 'ocr', 'direction': [1, 0]}
             for i, text in enumerate(['First wrapped line', 'continuation of the paragraph.'])]
    result, _ = merge_ocr([], [block_from_lines(lines, 'ocr')], False, Settings())
    assert len(result) == 1
    assert result[0]['text'] == 'First wrapped line\ncontinuation of the paragraph.'


def test_soft_hyphen_evidence_survives_line_extraction():
    from pipeline import native_blocks
    class Page:
        def get_text(self, *args, **kwargs):
            return {'blocks': [{'type': 0, 'lines': [
                {'bbox': (50, 100, 150, 115), 'dir': (1, 0), 'spans': [{'text': 'inter\u00ad', 'font': 'Helvetica'}]},
                {'bbox': (50, 120, 200, 135), 'dir': (1, 0), 'spans': [{'text': 'nacional', 'font': 'Helvetica'}]}]}]}
    blocks = native_blocks(Page())
    assert blocks[0]['text'] == 'internacional'
    assert blocks[0]['lines'][0]['text'].endswith('\u00ad')


@pytest.mark.parametrize('gray,expected', [(0.6, 'SCANNED GRAY TEXT'), (0.7, '')])
def test_light_gray_scanned_text_not_missed(gray, expected):
    from adversarial import raster
    image = raster('SCANNED GRAY TEXT', color=(gray, gray, gray))
    data = pdf_page(lambda page: page.insert_image(pymupdf.Rect(0, 100, 600, 700), stream=image))
    page = extract_document(data)['pages'][0]
    assert page['ocr']['executed']
    assert text_of(page) == expected
    if not expected:
        assert 'ocr_no_new_usable_text' in page['warnings']
