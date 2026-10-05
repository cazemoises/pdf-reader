"""Actual HTTP ingest -> Go adapter -> Python worker -> PostgreSQL -> read, with synthetic data."""
import json
import sys
import time
import urllib.request
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]/'tests'))
from corpus import corpus

base = sys.argv[1]
for attempt in range(30):
    try:
        urllib.request.urlopen(base+'/health', timeout=2).close()
        break
    except OSError:
        time.sleep(1)
else:
    raise RuntimeError('backend did not start')
for name in ('columns', 'table', 'scan', 'hybrid'):
    data, expected = corpus()[name]
    boundary = 'pdf-reader-synthetic-fixture'
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="fixture.pdf"\r\nContent-Type: application/pdf\r\n\r\n'.encode()
            + data + f'\r\n--{boundary}--\r\n'.encode())
    request = urllib.request.Request(base+'/books', body, {'Content-Type': 'multipart/form-data; boundary='+boundary})
    with urllib.request.urlopen(request, timeout=100) as response:
        book = json.load(response)
    assert book['status'] == 'ready', book
    with urllib.request.urlopen(base+'/books/'+book['id']+'/pages/1') as response:
        page = json.load(response)
    positions = [page['text'].index(phrase) for phrase in expected[0]]
    assert positions == sorted(positions), name
    assert page['extraction']['strategy'] == ('ocr-partial' if name in ('scan', 'hybrid') else 'native-layout')
    assert page['extraction']['blocks'], name
    print(name+': ingest, persistence and reading OK')
