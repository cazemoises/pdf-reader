import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getProgress, listBooks } from "../api/client";
import type { Book } from "../api/types";
import ThemeToggle from "../components/ThemeToggle";

function BookListPage() {
  const [books, setBooks] = useState<Book[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [progressByBook, setProgressByBook] = useState<Record<string, number>>({});

  useEffect(() => {
    let cancelled = false;

    listBooks()
      .then((result) => {
        if (!cancelled) {
          setBooks(result);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load books");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    books
      .filter((book) => book.status === "ready")
      .forEach((book) => {
        getProgress(book.id)
          .then((progress) => {
            if (!cancelled && progress) {
              setProgressByBook((prev) => ({ ...prev, [book.id]: progress.percentage }));
            }
          })
          .catch(() => {
            // Decorative only: a missing/failed progress fetch just leaves the bar off.
          });
      });

    return () => {
      cancelled = true;
    };
  }, [books]);

  return (
    <div className="min-h-screen bg-background font-sans text-ink">
      <header className="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-elevated px-4 py-3.5 sm:px-8">
        <span className="font-serif text-lg font-semibold tracking-tight">leitor.</span>
        <ThemeToggle />
      </header>

      <main className="mx-auto max-w-5xl px-4 pb-28 pt-6 sm:px-8">
        <div className="mb-5 flex items-end justify-between">
          <h1 className="font-serif text-2xl font-semibold sm:text-[26px]">Biblioteca</h1>
          <span className="text-sm text-ink-muted">
            {books.length} {books.length === 1 ? "livro" : "livros"}
          </span>
        </div>

        {loading && <p className="text-ink-muted">Carregando…</p>}
        {error && <p className="text-danger">{error}</p>}

        {!loading && !error && books.length === 0 && (
          <div className="flex flex-col items-center gap-4 px-6 py-24 text-center">
            <div className="h-11 w-8 rounded-sm border-2 border-ink-faint" />
            <p className="font-serif text-lg font-semibold">Nenhum livro ainda</p>
            <p className="max-w-[230px] text-sm text-ink-muted">
              Envie seu primeiro PDF para começar a ler.
            </p>
            <Link
              to="/upload"
              className="mt-1 inline-flex h-12 min-w-[200px] items-center justify-center rounded-[10px] bg-accent px-6 text-sm font-semibold text-accent-text"
            >
              Adicionar PDF
            </Link>
          </div>
        )}

        {!loading && !error && books.length > 0 && (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
            {books.map((book) => {
              const percentage = progressByBook[book.id];
              return (
                <Link
                  key={book.id}
                  to={`/read/${book.id}`}
                  className="flex flex-col gap-2"
                >
                  <div
                    className="flex aspect-[3/4] items-center justify-center rounded-md border border-border bg-surface px-2.5"
                    style={{
                      backgroundImage:
                        "repeating-linear-gradient(135deg, var(--color-border) 0 2px, transparent 2px 10px)",
                    }}
                  >
                    {book.status === "processing" && (
                      <div className="flex flex-col items-center gap-2">
                        <div className="h-[18px] w-[18px] animate-spin rounded-full border-2 border-border border-t-accent" />
                        <span className="text-center text-[9.5px] text-ink-faint">
                          processando…
                        </span>
                      </div>
                    )}
                    {book.status !== "processing" && (
                      <span className="text-center font-mono text-[10px] text-ink-faint">
                        capa: {book.title}
                      </span>
                    )}
                  </div>

                  <div className="font-serif text-[13.5px] font-semibold leading-tight">
                    {book.title}
                  </div>

                  {book.status === "failed" && (
                    <span className="text-[11px] font-medium text-danger">Falhou</span>
                  )}

                  {book.status === "ready" && percentage !== undefined && (
                    <div className="flex items-center gap-1.5">
                      <div className="h-[3px] flex-1 overflow-hidden rounded-full bg-border">
                        <div
                          className="h-full bg-accent"
                          style={{ width: `${Math.min(100, Math.max(0, percentage))}%` }}
                        />
                      </div>
                      <span className="text-[10px] text-ink-faint">
                        {percentage >= 100 ? "Concluído" : `${Math.round(percentage)}%`}
                      </span>
                    </div>
                  )}

                  {book.status === "ready" && percentage === undefined && (
                    <span className="text-[11px] text-ink-faint">Pronto</span>
                  )}
                </Link>
              );
            })}
          </div>
        )}
      </main>

      <Link
        to="/upload"
        aria-label="Adicionar PDF"
        className="fixed bottom-6 right-5 flex h-14 w-14 items-center justify-center rounded-full bg-accent text-2xl leading-none text-accent-text shadow-[0_8px_20px_rgba(0,0,0,0.25)]"
      >
        +
      </Link>
    </div>
  );
}

export default BookListPage;
