import pymupdf
from fastapi import FastAPI, UploadFile

app = FastAPI(title="pdf-reader extractor")


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/extract")
async def extract(file: UploadFile) -> dict:
    pdf_bytes = await file.read()
    doc = pymupdf.open(stream=pdf_bytes, filetype="pdf")

    pages = []
    for page_number, page in enumerate(doc, start=1):
        blocks = []
        for x0, y0, x1, y1, text, *_ in page.get_text("blocks"):
            blocks.append(
                {
                    "text": text.strip(),
                    "bbox": {
                        "x": x0,
                        "y": y0,
                        "width": x1 - x0,
                        "height": y1 - y0,
                    },
                }
            )
        pages.append(
            {
                "page_number": page_number,
                "width": page.rect.width,
                "height": page.rect.height,
                "blocks": blocks,
            }
        )

    doc.close()
    return {"pages": pages}
