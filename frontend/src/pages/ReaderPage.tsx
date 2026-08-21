import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { GlobalWorkerOptions, TextLayer, getDocument } from "pdfjs-dist";
import type { PDFDocumentProxy } from "pdfjs-dist";
import pdfWorkerSrc from "pdfjs-dist/build/pdf.worker.mjs?url";
import { bookFileUrl, getBook, getProgress, saveProgress } from "../api/client";
import type { Book } from "../api/types";
import "./ReaderPage.css";

GlobalWorkerOptions.workerSrc = pdfWorkerSrc;

const PAGE_SCALE = 1.4;

function ReaderPage() {
  const { id } = useParams<{ id: string }>();
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const textLayerRef = useRef<HTMLDivElement | null>(null);

  const [book, setBook] = useState<Book | null>(null);
  const [pdf, setPdf] = useState<PDFDocumentProxy | null>(null);
  const [pageNumber, setPageNumber] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [progressReady, setProgressReady] = useState(false);
  const skipNextSaveRef = useRef(false);

  useEffect(() => {
    if (!id) {
      return;
    }

    let cancelled = false;

    getBook(id)
      .then((result) => {
        if (!cancelled) {
          setBook(result);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load book");
        }
      });

    const loadingTask = getDocument({ url: bookFileUrl(id) });
    loadingTask.promise
      .then((doc) => {
        if (cancelled) {
          return;
        }
        setPdf(doc);
        setPageNumber(1);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load PDF");
        }
      });

    return () => {
      cancelled = true;
      void loadingTask.destroy();
    };
  }, [id]);

  useEffect(() => {
    if (!pdf || !id) {
      return;
    }

    let cancelled = false;
    setProgressReady(false);

    getProgress(id)
      .then((progress) => {
        if (cancelled || !progress) {
          return;
        }
        if (progress.lastPage >= 1 && progress.lastPage <= pdf.numPages) {
          skipNextSaveRef.current = true;
          setPageNumber(progress.lastPage);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load reading progress");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setProgressReady(true);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [pdf, id]);

  useEffect(() => {
    if (!pdf) {
      return;
    }

    const canvas = canvasRef.current;
    const textLayerDiv = textLayerRef.current;
    if (!canvas || !textLayerDiv) {
      return;
    }

    let cancelled = false;

    pdf
      .getPage(pageNumber)
      .then(async (page) => {
        if (cancelled) {
          return;
        }

        const viewport = page.getViewport({ scale: PAGE_SCALE });
        const outputScale = window.devicePixelRatio || 1;

        canvas.width = Math.floor(viewport.width * outputScale);
        canvas.height = Math.floor(viewport.height * outputScale);
        canvas.style.width = `${Math.floor(viewport.width)}px`;
        canvas.style.height = `${Math.floor(viewport.height)}px`;

        textLayerDiv.style.width = `${Math.floor(viewport.width)}px`;
        textLayerDiv.style.height = `${Math.floor(viewport.height)}px`;
        textLayerDiv.replaceChildren();

        const transform =
          outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : undefined;

        const renderTask = page.render({ canvas, transform, viewport });
        await renderTask.promise;
        if (cancelled) {
          return;
        }

        const textLayer = new TextLayer({
          textContentSource: page.streamTextContent({
            includeMarkedContent: true,
            disableNormalization: true,
          }),
          container: textLayerDiv,
          viewport,
        });
        await textLayer.render();
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to render page");
        }
      });

    return () => {
      cancelled = true;
    };
  }, [pdf, pageNumber]);

  useEffect(() => {
    if (!pdf || !id || !progressReady) {
      return;
    }

    if (skipNextSaveRef.current) {
      skipNextSaveRef.current = false;
      return;
    }

    const percentage = (pageNumber / pdf.numPages) * 100;
    saveProgress(id, pageNumber, percentage).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : "failed to save reading progress");
    });
  }, [pdf, id, pageNumber, progressReady]);

  if (!id) {
    return <p className="p-6 text-red-400">Missing book id.</p>;
  }

  const numPages = pdf?.numPages ?? 0;

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      <div className="flex items-center justify-between border-b border-slate-800 px-6 py-3">
        <div className="flex items-center gap-4">
          <Link to="/" className="text-sm text-slate-400 hover:text-slate-100">
            Back to books
          </Link>
          <h1 className="text-lg font-medium">{book?.title ?? "Loading…"}</h1>
        </div>

        {numPages > 0 && (
          <div className="flex items-center gap-3 text-sm">
            <button
              type="button"
              onClick={() => setPageNumber((n) => Math.max(1, n - 1))}
              disabled={pageNumber <= 1}
              className="rounded-md border border-slate-700 px-3 py-1 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Previous
            </button>
            <span>
              Page {pageNumber} of {numPages}
            </span>
            <button
              type="button"
              onClick={() => setPageNumber((n) => Math.min(numPages, n + 1))}
              disabled={pageNumber >= numPages}
              className="rounded-md border border-slate-700 px-3 py-1 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Next
            </button>
          </div>
        )}
      </div>

      {error && <p className="px-6 py-4 text-red-400">{error}</p>}

      <div className="flex justify-center overflow-auto p-6">
        <div className="pageContainer">
          <canvas ref={canvasRef} />
          <div ref={textLayerRef} className="textLayer" />
        </div>
      </div>
    </div>
  );
}

export default ReaderPage;
