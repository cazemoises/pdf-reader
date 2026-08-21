import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { GlobalWorkerOptions, TextLayer, getDocument } from "pdfjs-dist";
import type { PDFDocumentProxy } from "pdfjs-dist";
import pdfWorkerSrc from "pdfjs-dist/build/pdf.worker.mjs?url";
import {
  bookFileUrl,
  createHighlight,
  createNote,
  getBook,
  getProgress,
  listHighlights,
  listNotes,
  saveProgress,
} from "../api/client";
import type { Book, BoundingBox, Highlight, Note } from "../api/types";
import "./ReaderPage.css";

GlobalWorkerOptions.workerSrc = pdfWorkerSrc;

const PAGE_SCALE = 1.4;

const HIGHLIGHT_COLORS = ["#ffeb3b", "#4ade80", "#60a5fa", "#f472b6"];

interface PendingSelection {
  pageNumber: number;
  box: BoundingBox;
  anchorX: number;
  anchorY: number;
}

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

  const [highlights, setHighlights] = useState<Highlight[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [canvasSize, setCanvasSize] = useState({ width: 0, height: 0 });
  const [pendingSelection, setPendingSelection] = useState<PendingSelection | null>(null);
  const [selectedColor, setSelectedColor] = useState(HIGHLIGHT_COLORS[0]);
  const [noteDraft, setNoteDraft] = useState("");

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

        const cssWidth = Math.floor(viewport.width);
        const cssHeight = Math.floor(viewport.height);

        canvas.width = Math.floor(viewport.width * outputScale);
        canvas.height = Math.floor(viewport.height * outputScale);
        canvas.style.width = `${cssWidth}px`;
        canvas.style.height = `${cssHeight}px`;

        textLayerDiv.style.width = `${cssWidth}px`;
        textLayerDiv.style.height = `${cssHeight}px`;
        textLayerDiv.replaceChildren();

        setCanvasSize({ width: cssWidth, height: cssHeight });

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
    if (!id) {
      return;
    }

    let cancelled = false;

    listHighlights(id)
      .then((result) => {
        if (!cancelled) {
          setHighlights(result);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load highlights");
        }
      });

    listNotes(id)
      .then((result) => {
        if (!cancelled) {
          setNotes(result);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load notes");
        }
      });

    return () => {
      cancelled = true;
    };
  }, [id]);

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

  function handleTextLayerMouseUp() {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
      return;
    }
    if (!selection.toString().trim()) {
      return;
    }

    const canvas = canvasRef.current;
    if (!canvas) {
      return;
    }

    const canvasRect = canvas.getBoundingClientRect();
    const selectionRect = selection.getRangeAt(0).getBoundingClientRect();
    if (canvasRect.width === 0 || canvasRect.height === 0) {
      return;
    }

    const box: BoundingBox = {
      x: Math.max(0, (selectionRect.left - canvasRect.left) / canvasRect.width),
      y: Math.max(0, (selectionRect.top - canvasRect.top) / canvasRect.height),
      width: selectionRect.width / canvasRect.width,
      height: selectionRect.height / canvasRect.height,
    };

    if (box.width <= 0 || box.height <= 0) {
      return;
    }

    setPendingSelection({
      pageNumber,
      box,
      anchorX: selectionRect.right - canvasRect.left,
      anchorY: selectionRect.bottom - canvasRect.top,
    });
    setSelectedColor(HIGHLIGHT_COLORS[0]);
    setNoteDraft("");
  }

  function handleCancelHighlight() {
    setPendingSelection(null);
    setNoteDraft("");
  }

  async function handleConfirmHighlight() {
    if (!id || !pendingSelection) {
      return;
    }

    try {
      const highlight = await createHighlight(
        id,
        pendingSelection.pageNumber,
        pendingSelection.box,
        selectedColor,
      );
      setHighlights((prev) => [...prev, highlight]);

      const content = noteDraft.trim();
      if (content) {
        const note = await createNote(id, highlight.id, content);
        setNotes((prev) => [...prev, note]);
      }

      window.getSelection()?.removeAllRanges();
      setPendingSelection(null);
      setNoteDraft("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to create highlight");
    }
  }

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
          <div ref={textLayerRef} className="textLayer" onMouseUp={handleTextLayerMouseUp} />

          {canvasSize.width > 0 &&
            highlights
              .filter((highlight) => highlight.pageNumber === pageNumber)
              .map((highlight) => (
                <div
                  key={highlight.id}
                  className="highlightMark"
                  style={{
                    left: highlight.box.x * canvasSize.width,
                    top: highlight.box.y * canvasSize.height,
                    width: highlight.box.width * canvasSize.width,
                    height: highlight.box.height * canvasSize.height,
                    backgroundColor: highlight.color,
                  }}
                />
              ))}

          {pendingSelection && (
            <div
              className="highlightPopup"
              style={{ left: pendingSelection.anchorX, top: pendingSelection.anchorY }}
            >
              <div className="highlightPopupColors">
                {HIGHLIGHT_COLORS.map((color) => (
                  <button
                    key={color}
                    type="button"
                    className={
                      "colorSwatch" + (color === selectedColor ? " colorSwatch--selected" : "")
                    }
                    style={{ backgroundColor: color }}
                    onClick={() => setSelectedColor(color)}
                    aria-label={`Use color ${color}`}
                  />
                ))}
              </div>
              <textarea
                className="highlightPopupNote"
                placeholder="Add a note (optional)"
                value={noteDraft}
                onChange={(event) => setNoteDraft(event.target.value)}
                rows={2}
              />
              <div className="highlightPopupActions">
                <button type="button" onClick={handleCancelHighlight}>
                  Cancel
                </button>
                <button type="button" onClick={() => void handleConfirmHighlight()}>
                  Save highlight
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export default ReaderPage;
