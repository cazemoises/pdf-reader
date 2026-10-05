import asyncio
import io
import os
import subprocess
import sys
import threading
import time
from dataclasses import replace
from pathlib import Path
import pytest
import pymupdf
from fastapi import HTTPException
from fastapi.testclient import TestClient
from starlette.requests import Request
from starlette.datastructures import UploadFile
import main
from limits import UploadLimit
from pipeline import extract_document, ExtractionError
from settings import Settings


def test_concurrent_upload_rejected_before_receiving_body():
    async def scenario():
        entered = asyncio.Event()
        release = asyncio.Event()
        middleware = None
        async def app(scope, receive, send):
            entered.set()
            await release.wait()
            await send({'type': 'http.response.start', 'status': 200, 'headers': []})
            await send({'type': 'http.response.body', 'body': b'ok'})
        middleware = UploadLimit(app, 100)
        async def forbidden_receive():
            raise AssertionError('busy requests must not read/spool their body')
        first_sent, second_sent = [], []
        async def first_send(message): first_sent.append(message)
        async def second_send(message): second_sent.append(message)
        scope = lambda: {'type': 'http', 'path': '/extract', 'headers': []}
        first = asyncio.create_task(middleware(scope(), forbidden_receive, first_send))
        await entered.wait()
        await middleware(scope(), forbidden_receive, second_send)
        assert second_sent[0]['status'] == 503
        release.set()
        await first
        assert middleware.slot.acquire(blocking=False)
        middleware.slot.release()
    asyncio.run(scenario())


def test_stalled_upload_has_deadline_and_releases_slot():
    async def scenario():
        sent = []
        async def app(scope, receive, send):
            assert (await receive())['type'] == 'http.disconnect'
        async def receive():
            await asyncio.Event().wait()
        async def send(message): sent.append(message)
        middleware = UploadLimit(app, 100, timeout_seconds=0.02)
        await middleware({'type': 'http', 'path': '/extract', 'headers': []}, receive, send)
        assert sent[0]['status'] == 408
        assert middleware.slot.acquire(blocking=False)
        middleware.slot.release()
    asyncio.run(scenario())


def test_cancelled_request_does_not_start_another_worker(monkeypatch):
    entered = threading.Event()
    release = threading.Event()
    def worker(data):
        entered.set()
        assert release.wait(5)
        return {'pages': []}
    monkeypatch.setattr(main, 'isolated_extract', worker)
    async def scenario():
        upload = UploadFile(io.BytesIO(b'PDF'), filename='sensitive.pdf')
        task = asyncio.create_task(main.extract(upload, Request({'type': 'http'})))
        try:
            assert await asyncio.to_thread(entered.wait, 2)
            task.cancel()
            with pytest.raises(asyncio.CancelledError): await task
            assert not main.slot.acquire(blocking=False)
            assert upload.file.closed
        finally:
            release.set()
        for _ in range(100):
            await asyncio.sleep(0.01)
            if main.slot.acquire(blocking=False):
                main.slot.release()
                break
        else:
            pytest.fail('slot leaked after worker completion')
    asyncio.run(scenario())


def test_real_subprocess_timeout_kills_and_reaps_child(monkeypatch, tmp_path):
    real_run = subprocess.run
    pidfile = tmp_path/'pid'
    def hung_worker(args, **kwargs):
        program = f'import os,time; open({str(pidfile)!r},"w").write(str(os.getpid())); time.sleep(30)'
        return real_run([sys.executable, '-c', program], **kwargs)
    monkeypatch.setattr(main.subprocess, 'run', hung_worker)
    monkeypatch.setattr(main, 'settings', replace(Settings(), timeout_seconds=1))
    with pytest.raises(HTTPException) as error:
        main.isolated_extract(b'')
    assert error.value.status_code == 504
    with pytest.raises(ProcessLookupError): os.kill(int(pidfile.read_text()), 0)


def test_tempfiles_closed_on_parser_error(monkeypatch):
    created = []
    real_temp = main.tempfile.TemporaryFile
    def temporary_file():
        f = real_temp()
        created.append(f)
        return f
    monkeypatch.setattr(main.tempfile, 'TemporaryFile', temporary_file)
    with pytest.raises(HTTPException): main.isolated_extract(b'invalid PDF')
    assert len(created) == 2 and all(f.closed for f in created)


def test_tempfile_exhaustion_is_controlled(monkeypatch):
    def unavailable(): raise OSError('disk exhausted')
    monkeypatch.setattr(main.tempfile, 'TemporaryFile', unavailable)
    with pytest.raises(HTTPException) as error: main.isolated_extract(b'')
    assert error.value.status_code == 503


def test_invalid_multipart_releases_capacity():
    with TestClient(main.app) as client:
        assert client.post('/extract', content=b'broken', headers={'content-type': 'multipart/form-data'}).status_code == 400
        assert client.get('/health').status_code == 200
    assert main.slot.acquire(blocking=False)
    main.slot.release()


def test_actual_http_file_and_page_limits():
    with TestClient(main.app) as client:
        with pymupdf.open() as doc:
            for _ in range(501): doc.new_page()
            response = client.post('/extract', files={'file': ('pages.pdf', doc.tobytes())})
        assert response.status_code == 422
        response = client.post('/extract', files={'file': ('large.pdf', b'x'*(32*1024*1024+1))})
        assert response.status_code == 413


def test_source_image_exhaustion_rejected_before_decoding():
    with pymupdf.open() as doc:
        page = doc.new_page()
        pixel = pymupdf.Pixmap(pymupdf.csGRAY, pymupdf.IRect(0, 0, 1, 1), False)
        pixel.clear_with(255)
        xref = page.insert_image(page.rect, pixmap=pixel)
        # Hostile metadata declares a 2.5-billion-pixel image backed by a tiny stream.
        doc.xref_set_key(xref, 'Width', '50000')
        doc.xref_set_key(xref, 'Height', '50000')
        data = doc.tobytes()
    with pytest.raises(ExtractionError, match='source image pixel'):
        extract_document(data)


def test_native_stderr_cannot_inject_content_into_logs(monkeypatch, caplog):
    import json
    from types import SimpleNamespace
    def noisy_worker(args, **kwargs):
        kwargs['stderr'].write(b'INFO:pdf_reader.extraction: CONFIDENTIAL PDF TEXT\n')
        kwargs['stdout'].write(json.dumps({'document_id': 'audit', 'pages': []}).encode())
        return SimpleNamespace(returncode=0)
    monkeypatch.setattr(main.subprocess, 'run', noisy_worker)
    with caplog.at_level('INFO'):
        assert main.isolated_extract(b'') == {'document_id': 'audit', 'pages': []}
    assert 'CONFIDENTIAL' not in caplog.text
