"""Original synthetic fixtures, generated in memory; CC0, no external PDFs/fonts."""
import pymupdf
from functools import lru_cache


@lru_cache(maxsize=1)
def corpus():
    cases = {}
    for kind in ('simple', 'multipage', 'columns', 'table', 'scan', 'hybrid', 'rotated', 'unicode', 'margins', 'complex'):
        with pymupdf.open() as doc:
            count = 3 if kind in ('multipage', 'margins') else 1
            expected = []
            for n in range(count):
                page = doc.new_page(width=600, height=800)
                texts = ['First paragraph.', 'Second paragraph.']
                if kind == 'columns':
                    texts = ['Left first.', 'Left second.', 'Right first.', 'Right second.']
                    for i in (2, 0, 3, 1):
                        page.insert_text((330 if i >= 2 else 50, 100 + (i % 2)*70), texts[i])
                elif kind == 'table':
                    texts = ['Name', 'Value', 'Alpha', '42']
                    for x in (50, 250, 450):
                        page.draw_line((x, 100), (x, 200))
                    for y in (100, 150, 200):
                        page.draw_line((50, y), (450, y))
                    for i, text in enumerate(texts):
                        page.insert_text((60 + (i % 2)*200, 130 + (i // 2)*50), text)
                elif kind in ('scan', 'hybrid'):
                    texts = ['SCANNED DOCUMENT']
                    with pymupdf.open() as image_doc:
                        image_page = image_doc.new_page(width=600, height=800)
                        image_page.insert_text((60, 200), texts[0], fontsize=28)
                        image = image_page.get_pixmap(matrix=pymupdf.Matrix(2, 2)).tobytes('png')
                    page.insert_image(page.rect, stream=image)
                    if kind == 'hybrid':
                        page.insert_text((50, 50), 'Native caption.')
                        texts.insert(0, 'Native caption.')
                else:
                    if kind == 'unicode':
                        texts = ['Ação, café, coração.']
                    if kind == 'complex':
                        texts = ['Document title', 'First paragraph.', '1. List item', 'Figure caption.']
                    # Deliberately write lower content first, simulating PDF paint order.
                    for i in reversed(range(len(texts))):
                        page.insert_text((50, 100 + i*70), texts[i])
                    if kind == 'margins':
                        page.insert_text((50, 25), 'Repeated header')
                        page.insert_text((50, 780), str(n + 1))
                        texts = ['Repeated header']+texts+[str(n+1)]
                    if kind == 'rotated':
                        page.set_rotation(90)
                expected.append(texts)
            cases[kind] = (doc.tobytes(), expected)
    return cases


def legacy(data):
    with pymupdf.open(stream=data, filetype='pdf') as doc:
        return [{'blocks': [{'text': b[4].strip()} for b in p.get_text('blocks')]} for p in doc]
