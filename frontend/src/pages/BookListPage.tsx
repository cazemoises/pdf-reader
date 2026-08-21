import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { listBooks } from "../api/client";
import type { Book } from "../api/types";

const statusStyles: Record<Book["status"], string> = {
  processing: "bg-amber-500/20 text-amber-300",
  ready: "bg-emerald-500/20 text-emerald-300",
  failed: "bg-red-500/20 text-red-300",
};

function BookListPage() {
  const [books, setBooks] = useState<Book[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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

  return (
    <div className="min-h-screen bg-slate-950 px-6 py-10 text-slate-100">
      <div className="mx-auto max-w-3xl">
        <div className="mb-8 flex items-center justify-between">
          <h1 className="text-2xl font-semibold">Your books</h1>
          <Link
            to="/upload"
            className="rounded-md bg-slate-100 px-4 py-2 text-sm font-medium text-slate-900 hover:bg-white"
          >
            Upload a PDF
          </Link>
        </div>

        {loading && <p className="text-slate-400">Loading books…</p>}
        {error && <p className="text-red-400">{error}</p>}

        {!loading && !error && books.length === 0 && (
          <p className="text-slate-400">
            No books yet. Upload a PDF to get started.
          </p>
        )}

        {!loading && !error && books.length > 0 && (
          <ul className="divide-y divide-slate-800 rounded-lg border border-slate-800">
            {books.map((book) => (
              <li key={book.id}>
                <Link
                  to={`/read/${book.id}`}
                  className="flex items-center justify-between px-4 py-3 hover:bg-slate-900"
                >
                  <span className="font-medium">{book.title}</span>
                  <span
                    className={`rounded-full px-2 py-0.5 text-xs font-medium ${statusStyles[book.status]}`}
                  >
                    {book.status}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

export default BookListPage;
