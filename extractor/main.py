from fastapi import FastAPI

app = FastAPI(title="pdf-reader extractor")


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}
