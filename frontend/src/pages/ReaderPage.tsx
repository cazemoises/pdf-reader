import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { GlobalWorkerOptions, TextLayer, getDocument } from "pdfjs-dist";
import type { PDFDocumentProxy, RenderTask } from "pdfjs-dist";
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
import ThemeToggle from "../components/ThemeToggle";
import "./ReaderPage.css";

GlobalWorkerOptions.workerSrc = pdfWorkerSrc;

const PAGE_SCALE = 1.4;

const HIGHLIGHT_COLORS = ["#d9a441", "#6fa87c", "#d98a6f", "#7fa3c2"];

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
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [chromeVisible, setChromeVisible] = useState(false);
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
    let renderTask: RenderTask | null = null;

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
        // pdf.js's TextLayer positions/sizes every span via CSS calc()/round()
        // expressions that read these custom properties from the container;
        // without them the text layer overlay is unscaled and misaligned
        // with the canvas, breaking mouse-based text selection.
        textLayerDiv.style.setProperty("--scale-factor", `${viewport.scale}`);
        textLayerDiv.style.setProperty("--user-unit", "1");
        textLayerDiv.style.setProperty(
          "--total-scale-factor",
          "calc(var(--scale-factor) * var(--user-unit))",
        );
        textLayerDiv.style.setProperty("--scale-round-x", "1px");
        textLayerDiv.style.setProperty("--scale-round-y", "1px");
        textLayerDiv.replaceChildren();

        setCanvasSize({ width: cssWidth, height: cssHeight });

        const transform =
          outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : undefined;

        renderTask = page.render({ canvas, transform, viewport });
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
      renderTask?.cancel();
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
    if (!selection || selection.isCollapsed || selection.rangeCount === 0 || !selection.toString().trim()) {
      // No text was dragged into a selection: treat this as a tap that
      // shows/hides the reading chrome instead of starting a highlight.
      setChromeVisible((visible) => !visible);
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
    return <p className="p-6 text-danger">Missing book id.</p>;
  }

  const numPages = pdf?.numPages ?? 0;
  const readingProgressPct = numPages > 0 ? (pageNumber / numPages) * 100 : 0;

  return (
    <div className="min-h-screen bg-reading font-sans text-ink">
      {/* Thin progress line — always visible, independent of the hideable chrome. */}
      <div className="fixed left-0 right-0 top-0 z-30 h-0.5 bg-border">
        <div className="h-full bg-accent" style={{ width: `${readingProgressPct}%` }} />
      </div>

      {/* Top chrome: hidden by default, revealed by tapping the reading area. */}
      <div
        className={
          "fixed left-0 right-0 top-0 z-20 flex items-center justify-between border-b border-border bg-elevated/95 px-3 py-3 transition-opacity duration-150 " +
          (chromeVisible ? "opacity-100" : "pointer-events-none opacity-0")
        }
      >
        <Link
          to="/"
          className="flex h-11 w-11 items-center justify-center rounded-full text-lg text-ink"
          aria-label="Voltar para a biblioteca"
        >
          ‹
        </Link>
        <span className="truncate px-2 text-sm font-semibold">{book?.title ?? "Carregando…"}</span>
        <div className="flex items-center gap-1">
          <ThemeToggle />
          <button
            type="button"
            onClick={() => setSidebarOpen((open) => !open)}
            aria-label={`Notas (${highlights.length})`}
            className="flex h-11 w-11 items-center justify-center rounded-full text-base text-ink"
          >
            ≡
          </button>
        </div>
      </div>

      {error && <p className="fixed left-0 right-0 top-12 z-20 bg-danger-soft px-4 py-2 text-sm text-danger">{error}</p>}

      <div className="flex justify-center overflow-auto pb-6 pt-6">
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
              className="absolute z-30 flex min-w-[200px] flex-col gap-2 rounded-lg border border-border bg-elevated p-2.5 shadow-[0_8px_20px_rgba(0,0,0,0.3)]"
              style={{ left: pendingSelection.anchorX, top: pendingSelection.anchorY }}
            >
              <div className="flex gap-1.5">
                {HIGHLIGHT_COLORS.map((color) => (
                  <button
                    key={color}
                    type="button"
                    className={
                      "h-5 w-5 rounded-full border-2 " +
                      (color === selectedColor ? "border-ink" : "border-transparent")
                    }
                    style={{ backgroundColor: color }}
                    onClick={() => setSelectedColor(color)}
                    aria-label={`Use color ${color}`}
                  />
                ))}
              </div>
              <textarea
                className="resize-none rounded border border-border bg-background p-1.5 text-sm text-ink outline-none"
                placeholder="Adicionar uma nota (opcional)"
                value={noteDraft}
                onChange={(event) => setNoteDraft(event.target.value)}
                rows={2}
              />
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={handleCancelHighlight}
                  className="rounded border border-border px-2.5 py-1 text-sm text-ink"
                >
                  Cancelar
                </button>
                <button
                  type="button"
                  onClick={() => void handleConfirmHighlight()}
                  className="rounded bg-accent px-2.5 py-1 text-sm font-semibold text-accent-text"
                >
                  Salvar
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Bottom chrome: page navigation, hidden with the same tap gesture as the top bar. */}
      {numPages > 0 && (
        <div
          className={
            "fixed bottom-0 left-0 right-0 z-20 flex flex-col items-center gap-2 border-t border-border bg-elevated/95 px-5 py-3.5 transition-opacity duration-150 " +
            (chromeVisible ? "opacity-100" : "pointer-events-none opacity-0")
          }
        >
          <div className="h-0.5 w-full overflow-hidden rounded-full bg-border">
            <div className="h-full bg-accent" style={{ width: `${readingProgressPct}%` }} />
          </div>
          <div className="flex w-full items-center justify-between">
            <button
              type="button"
              onClick={() => setPageNumber((n) => Math.max(1, n - 1))}
              disabled={pageNumber <= 1}
              className="flex h-9 w-9 items-center justify-center rounded-full text-ink disabled:cursor-not-allowed disabled:opacity-30"
              aria-label="Página anterior"
            >
              ‹
            </button>
            <span className="text-xs tabular-nums text-ink-faint">
              {pageNumber} de {numPages}
            </span>
            <button
              type="button"
              onClick={() => setPageNumber((n) => Math.min(numPages, n + 1))}
              disabled={pageNumber >= numPages}
              className="flex h-9 w-9 items-center justify-center rounded-full text-ink disabled:cursor-not-allowed disabled:opacity-30"
              aria-label="Próxima página"
            >
              ›
            </button>
          </div>
        </div>
      )}

      {/* Notes bottom sheet */}
      {sidebarOpen && (
        <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/30" onClick={() => setSidebarOpen(false)}>
          <aside
            className="flex max-h-[70vh] w-full max-w-lg flex-col gap-4 rounded-t-[20px] bg-elevated px-5 pb-6 pt-2.5"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="mx-auto h-1 w-9 rounded-full bg-border" />
            <div className="flex items-center justify-between">
              <div>
                <h2 className="font-serif text-lg font-semibold">Notas — {book?.title ?? ""}</h2>
                <p className="mt-0.5 text-xs text-ink-muted">{highlights.length} highlights</p>
              </div>
              <button
                type="button"
                onClick={() => setSidebarOpen(false)}
                aria-label="Fechar"
                className="flex h-10 w-10 items-center justify-center rounded-full border border-border text-sm text-ink-muted"
              >
                ✕
              </button>
            </div>

            {highlights.length === 0 && (
              <p className="text-sm text-ink-faint">No highlights yet.</p>
            )}

            <ul className="flex flex-col gap-4 overflow-auto pb-2">
              {highlights
                .slice()
                .sort((a, b) => a.pageNumber - b.pageNumber)
                .map((highlight) => {
                  const highlightNotes = notes.filter(
                    (note) => note.highlightId === highlight.id,
                  );
                  return (
                    <li key={highlight.id} className="flex flex-col gap-1.5">
                      <div className="flex items-center gap-2">
                        <span
                          className="h-[9px] w-[9px] shrink-0 rounded-full"
                          style={{ backgroundColor: highlight.color }}
                        />
                        <span className="text-[11px] text-ink-faint">Página {highlight.pageNumber}</span>
                      </div>
                      {highlightNotes.map((note) => (
                        <p key={note.id} className="whitespace-pre-wrap text-sm text-ink-muted">
                          {note.content}
                        </p>
                      ))}
                    </li>
                  );
                })}
            </ul>
          </aside>
        </div>
      )}
    </div>
  );
}

export default ReaderPage;
