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


@pytest.mark.xfail(strict=True, reason='RV-FRAGMENTS: justified line fragments separated from continuation')
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


@pytest.mark.xfail(strict=True, reason='RV-VECTOR: visible outline glyphs do not request recovery')
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
    assert extract_document(data)['pages'][0]['fallback_requested']
