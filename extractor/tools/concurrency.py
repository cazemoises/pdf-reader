"""Parallel real OCR requests; busy responses must remain bounded and health must survive."""
import concurrent.futures
import json
import sys
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]/'tests'))
from adversarial import image_pdf

base = sys.argv[1]
barrier = threading.Barrier(8)
data = image_pdf()
boundary = 'concurrent-fixture'
body = f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="test.pdf"\r\nContent-Type: application/pdf\r\n\r\n'.encode()+data+f'\r\n--{boundary}--\r\n'.encode()
def request(_):
    barrier.wait()
    started = time.perf_counter()
    req = urllib.request.Request(base+'/extract', body, {'Content-Type': 'multipart/form-data; boundary='+boundary})
    try:
        with urllib.request.urlopen(req, timeout=100) as response:
            status = response.status
            result = json.load(response)
            assert result['pages'][0]['ocr']['executed']
    except urllib.error.HTTPError as error:
        status = error.code
        error.close()
    return {'status': status, 'seconds': time.perf_counter()-started}
with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
    results = list(pool.map(request, range(8)))
assert all(r['status'] in (200, 503) for r in results), results
assert any(r['status'] == 200 for r in results) and any(r['status'] == 503 for r in results), results
with urllib.request.urlopen(base+'/health', timeout=2) as response: assert response.status == 200
print(json.dumps({'requests': 8, 'accepted': sum(r['status'] == 200 for r in results),
                  'busy': sum(r['status'] == 503 for r in results), 'results': results}))
