"""Explainable decisions for original synthetic fixtures only; never logs real PDF text."""
import json
import sys
from pathlib import Path
sys.path[:0] = [str(Path(__file__).resolve().parents[1]), str(Path(__file__).resolve().parents[1]/'tests')]
import pymupdf
from corpus import corpus
from adversarial import image_pdf, corrupted_font, columns, margins, ruled_table, raster, pdf_page
from pipeline import extract_document, native_blocks

cases = {name: data for name, (data, _) in corpus().items()}
cases.update({'decorative': image_pdf(native='Native text.', decorative=True),
              'small_hybrid': image_pdf(native='Native text.', text='SCANNED REGION', small=True),
              'searchable_scan': image_pdf(text='SCANNED DOCUMENT', searchable=True),
              'low_resolution': image_pdf(text='DOCUMENTO LEGIVEL', low_resolution=True),
              'corrupt_font': corrupted_font(), 'nul_font': corrupted_font(nul=True),
              'three_columns': columns()[0], 'different_margins': margins(),
              'multiline_cells': ruled_table(multiline=True, empty=True)})
for gray in (0.6, 0.7):
    image = raster('SCANNED GRAY TEXT', color=(gray, gray, gray))
    cases[f'gray_scan_{gray}'] = pdf_page(lambda page: page.insert_image(pymupdf.Rect(0, 100, 600, 700), stream=image))
for rotation in (0, 90, 180, 270): cases[f'portuguese_scan_{rotation}'] = image_pdf(rotation=rotation)
rows = {}
for name, data in cases.items():
    result = extract_document(data)
    native = []
    with pymupdf.open(stream=data, filetype='pdf') as doc:
        native = ['\n'.join(b['text'] for b in native_blocks(page)) for page in doc]
    rows[name] = [{'page': page['page_number'], 'native_text': text, 'decision': page['ocr'],
                   'result_text': '\n'.join(b['text'] for b in page['blocks']), 'warnings': page['warnings']}
                  for text, page in zip(native, result['pages'])]
print(json.dumps(rows, indent=2, ensure_ascii=False))
