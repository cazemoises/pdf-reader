import pymupdf
from fastapi.testclient import TestClient

from main import app

client = TestClient(app)


def _make_pdf_bytes(text: str) -> bytes:
    doc = pymupdf.open()
    page = doc.new_page(width=612, height=792)
    page.insert_text((72, 100), text)
    pdf_bytes = doc.tobytes()
    doc.close()
    return pdf_bytes


def test_extract_returns_text_and_bbox_per_page():
    pdf_bytes = _make_pdf_bytes("Hello World")

    response = client.post(
        "/extract",
        files={"file": ("test.pdf", pdf_bytes, "application/pdf")},
    )

    assert response.status_code == 200
    data = response.json()

    assert len(data["pages"]) == 1
    page = data["pages"][0]
    assert page["page_number"] == 1
    assert page["width"] == 612
    assert page["height"] == 792

    assert len(page["blocks"]) >= 1
    block = page["blocks"][0]
    assert "Hello World" in block["text"]

    bbox = block["bbox"]
    assert bbox["x"] > 0
    assert bbox["y"] > 0
    assert bbox["width"] > 0
    assert bbox["height"] > 0
