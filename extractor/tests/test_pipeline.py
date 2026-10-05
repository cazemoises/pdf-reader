from dataclasses import replace
import pymupdf
import pytest
from corpus import corpus, legacy
from pipeline import extract_document, ExtractionError
from settings import Settings
from main import app, slot
from fastapi.testclient import TestClient


@pytest.mark.parametrize('kind', ['simple', 'multipage', 'columns', 'table', 'rotated', 'unicode', 'margins', 'complex'])
def test_ground_truth_in_visual_order(kind):
    data, expected = corpus()[kind]
    result = extract_document(data, replace(Settings(), ocr_enabled=False))
    for page, truth in zip(result['pages'], expected):
        text = '\n'.join(b['text'] for b in page['blocks'] if b['type'] not in ('header', 'footer'))
        expected_lines = ['Name | Value', 'Alpha | 42'] if kind == 'table' else truth
        assert text.splitlines() == expected_lines
        assert len(page['blocks']) == (1 if kind == 'table' else len(truth))
        assert page['quality']['invalid_characters'] == 0
        assert not page['fallback_requested']


def test_baseline_demonstrates_paint_order_bug():
    data, _ = corpus()['columns']
    assert legacy(data)[0]['blocks'][0]['text'].startswith('Right first.')
    assert extract_document(data)['pages'][0]['blocks'][0]['text'] == 'Left first.'


def test_table_cells_preserved_without_duplicate_prose():
    page = extract_document(corpus()['table'][0])['pages'][0]
    tables = [b for b in page['blocks'] if b['type'] == 'table']
    assert tables[0]['cells'] == [['Name', 'Value'], ['Alpha', '42']]
    assert sum(b['text'].count('Alpha') for b in page['blocks']) == 1


def test_repeated_margins_retained_with_roles():
    pages = extract_document(corpus()['margins'][0])['pages']
    for page in pages:
        assert any(b.get('margin_candidate') == 'header' for b in page['blocks'])
        assert any(b.get('margin_candidate') == 'footer' for b in page['blocks'])


@pytest.mark.parametrize('kind', ['scan', 'hybrid'])
def test_ocr_only_when_required(kind):
    data, expected = corpus()[kind]
    result = extract_document(data)
    page = result['pages'][0]
    assert page['fallback_requested']
    if 'ocr_unavailable_or_failed' in page['warnings']:
        pytest.fail('OCR dependency missing: run tests in Docker test target')
    text = '\n'.join(b['text'] for b in page['blocks'])
    assert text.splitlines() == expected[0]
    assert all(text.count(phrase) == 1 for phrase in expected[0])
    assert page['strategy'] == 'ocr-partial'


def test_disabled_ocr_reports_incomplete_extraction():
    page = extract_document(corpus()['scan'][0], replace(Settings(), ocr_enabled=False))['pages'][0]
    assert 'ocr_disabled' in page['warnings']
    assert page['quality']['score'] is None


def test_limits_and_invalid_pdf():
    with pytest.raises(ExtractionError):
        extract_document(b'not a PDF')
    with pytest.raises(ExtractionError):
        extract_document(corpus()['simple'][0], replace(Settings(), max_bytes=10))
    with pytest.raises(ExtractionError):
        extract_document(corpus()['multipage'][0], replace(Settings(), max_pages=1))
    with pymupdf.open() as doc:
        doc.new_page()
        encrypted = doc.tobytes(encryption=pymupdf.PDF_ENCRYPT_AES_256, user_pw='secret')
    with pytest.raises(ExtractionError, match='encrypted'):
        extract_document(encrypted)


def test_invalid_input_is_controlled_http_error():
    with TestClient(app) as client:
        response = client.post('/extract', files={'file': ('bad.pdf', b'bad', 'application/pdf')})
        assert response.status_code == 422
        assert 'bad.pdf' not in response.text
        assert client.get('/health').status_code == 200


def test_busy_service_rejects_without_queue():
    slot.acquire()
    try:
        with TestClient(app) as client:
            assert client.post('/extract', files={'file': ('a.pdf', b'a')}).status_code == 503
    finally:
        slot.release()


def test_http_table_output_is_valid_json():
    with TestClient(app) as client:
        response = client.post('/extract', files={'file': ('table.pdf', corpus()['table'][0])})
        assert response.status_code == 200
        assert response.json()['pages'][0]['blocks'][0]['type'] == 'table'


def test_upload_limit_before_multipart_parsing():
    with TestClient(app) as client:
        response = client.post('/extract', content=b'', headers={'content-length': str(34*1024*1024)})
        assert response.status_code == 413


def test_blank_page_does_not_request_ocr():
    with pymupdf.open() as doc:
        doc.new_page()
        page = extract_document(doc.tobytes())['pages'][0]
    assert not page['fallback_requested']
    assert page['warnings'] == ['empty_page']


def test_page_border_does_not_invoke_table_parser(monkeypatch):
    def unexpected(*args, **kwargs):
        raise AssertionError('page decoration must not invoke table extraction')
    monkeypatch.setattr(pymupdf.Page, 'find_tables', unexpected)
    with pymupdf.open() as doc:
        page = doc.new_page()
        page.draw_rect(page.rect)
        page.insert_text((50, 100), 'Ordinary prose')
        result = extract_document(doc.tobytes())
    assert result['pages'][0]['blocks'][0]['text'] == 'Ordinary prose'


def test_normalization_does_not_guess_compound_hyphens():
    from layout import normalized
    assert normalized('ação\u00ad\ncorreta') == 'açãocorreta'
    assert normalized('well-\nknown') == 'well-\nknown'


def test_chunked_upload_budget():
    import asyncio
    from limits import UploadLimit
    sent = []
    async def downstream(scope, receive, send):
        assert (await receive())['type'] == 'http.disconnect'
        await send({'type': 'http.response.start', 'status': 400, 'headers': []})
        await send({'type': 'http.response.body', 'body': b'ignored'})
    async def receive():
        return {'type': 'http.request', 'body': b'too much', 'more_body': False}
    async def send(message):
        sent.append(message)
    asyncio.run(UploadLimit(downstream, 2)({'type': 'http', 'path': '/extract', 'headers': []}, receive, send))
    assert sent[0]['status'] == 413
