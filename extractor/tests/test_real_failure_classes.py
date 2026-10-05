"""Minimal synthetic reproductions, without private PDF content.

Strict expected failures record unresolved defects, not successful regressions.
See docs/real-validation.md for visual evidence and scope.
"""
import pytest
from layout import reading_order
from pipeline import native_blocks


def block(text, x, y, width, height=10):
    return {'text': text, 'bbox': {'x': x, 'y': y, 'width': width, 'height': height}}


@pytest.mark.xfail(strict=True, reason='RV-ORDER: vertical gutter places centered heading after body')
def test_centered_heading_before_short_left_aligned_body():
    items = [block('Heading', 260, 100, 50), block('First paragraph.', 50, 160, 100)]
    assert [b['text'] for b in reading_order(items)] == ['Heading', 'First paragraph.']


def test_same_baseline_fragments_before_paragraph_continuation():
    def line(text, box):
        return {'bbox': box, 'dir': (1, 0), 'spans': [{'text': text, 'font': 'Helvetica'}]}

    class Page:
        def get_text(self, *args, **kwargs):
            return {'blocks': [{'type': 0, 'lines': [
                line('Label', (120, 100, 165, 110)),
                line('positive:', (201, 100, 249, 110)),
                line('justice,', (284, 100, 322, 110)),
                line('continued sentence.', (85, 114, 565, 124)),
            ]}]}

    text = ' '.join(b['text'].replace('\n', ' ') for b in reading_order(native_blocks(Page())))
    assert text == 'Label positive: justice, continued sentence.'


def test_visible_vector_glyphs_without_native_text_request_recovery():
    import re
    import pymupdf
    from pipeline import extract_document

    # Convert our own generated glyph pixels into vector runs: no copyrighted
    # material, embedded image, or text operator in the document under test.
    with pymupdf.open() as source, pymupdf.open() as document:
        page = source.new_page(width=220, height=80)
        page.insert_text((15, 45), 'VECTOR WORDS', fontsize=18)
        pixels = page.get_pixmap(colorspace=pymupdf.csGRAY)
        vector = document.new_page(width=220, height=80)
        shape = vector.new_shape()
        for y in range(pixels.height):
            row = pixels.samples[y * pixels.width:(y + 1) * pixels.width]
            for run in re.finditer(b'[\x00-\x7f]+', row):
                shape.draw_rect(pymupdf.Rect(run.start(), y, run.end(), y + 1))
        shape.finish(color=None, fill=(0, 0, 0))
        shape.commit()
        assert not vector.get_text().strip()
        assert not vector.get_images()
        data = document.tobytes()
    result = extract_document(data)['pages'][0]
    assert result['fallback_requested']
    assert ' '.join(b['text'] for b in result['blocks']) == 'VECTOR WORDS'


@pytest.mark.parametrize('scale', [0.5, 1, 3])
def test_baseline_reconstruction_scales_with_font(scale):
    from pipeline import reconstruct_horizontal_lines
    def fragment(text, x):
        return {'bbox': (x*scale,100*scale,(x+20)*scale,110*scale),
                'dir': (1,0), 'wmode': 0,
                'spans': [{'text': text, 'font': 'Helvetica', 'size': 10*scale,
                           'origin': (x*scale,108*scale)}]}
    result = reconstruct_horizontal_lines([fragment('third',110),fragment('first',50),fragment('second',80)])
    assert len(result) == 1
    assert ' '.join(s['text'].strip() for s in result[0]['spans']) == 'first second third'


def test_vector_border_does_not_require_recovery():
    import pymupdf
    from pipeline import extract_document
    with pymupdf.open() as doc:
        page = doc.new_page()
        page.draw_rect((30,30,400,500), fill=(0,0,0))
        result = extract_document(doc.tobytes())['pages'][0]
    assert not result['fallback_requested']
    assert result['detection']['state'] == 'HEALTHY'


def test_recovery_budget_preserves_detection(monkeypatch):
    import pymupdf
    from pipeline import extract_page
    from settings import Settings
    from adversarial import image_pdf
    with pymupdf.open(stream=image_pdf(),filetype='pdf') as doc:
        result = extract_page(doc[0],1,Settings(),{'seconds':0})
    assert result['fallback_requested']
    assert not result['ocr']['executed']
    assert result['detection']['state'] == 'RECOVERY_REQUIRED'
    assert 'ocr_document_time_budget_exceeded' in result['warnings']


def test_ocr_with_no_visible_ink_is_rejected():
    import pymupdf
    from inspection import ocr_line_supported
    from settings import Settings
    with pymupdf.open() as doc:
        page = doc.new_page()
        item = block('Invented words',50,50,100)
        assert not ocr_line_supported(page,item,Settings())


def test_raster_uncertainty_does_not_automatically_request_ocr():
    import pymupdf
    from pipeline import extract_document
    from adversarial import image_pdf
    page = extract_document(image_pdf(native='Short text.',decorative=True))['pages'][0]
    assert not page['ocr']['executed']


@pytest.mark.parametrize('phrase', ['A', '42', '1 + 2 = 3', 'ação e café'])
def test_ocr_support_preserves_short_words_and_symbols(phrase):
    import pymupdf
    from inspection import ocr_line_supported
    from settings import Settings
    from layout import bbox
    # Test the acceptance gate independently of whether Tesseract recognizes
    # or the four-component decision policy requests these very short scans.
    with pymupdf.open() as doc:
        page = doc.new_page()
        page.insert_text((50,100),phrase,fontsize=26)
        line = page.get_text('dict')['blocks'][0]['lines'][0]
        assert ocr_line_supported(page,{'text':phrase,'bbox':bbox(line['bbox'])},Settings())


def test_same_baseline_columns_remain_separate():
    from pipeline import reconstruct_horizontal_lines
    def line(text,x):
        return {'bbox':(x,100,x+30,110),'dir':(1,0),
                'spans':[{'text':text,'font':'Helvetica','size':10,'origin':(x,108)}]}
    assert len(reconstruct_horizontal_lines([line('Left',50),line('Right',330)])) == 2


def test_rotated_line_paint_order_is_preserved():
    from pipeline import reconstruct_horizontal_lines
    lines=[{'bbox':(20,50,30,100),'dir':(0,-1),'spans':[{'text':'One'}]},
           {'bbox':(40,10,50,40),'dir':(0,-1),'spans':[{'text':'Two'}]}]
    assert reconstruct_horizontal_lines(lines) == lines
