import { useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router-dom";
import { uploadBook } from "../api/client";
import type { Book } from "../api/types";

function UploadPage() {
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [uploadedBook, setUploadedBook] = useState<Book | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file) {
      setError("Choose a PDF file first");
      return;
    }

    setSubmitting(true);
    setError(null);
    setUploadedBook(null);

    try {
      const book = await uploadBook(file, title);
      setUploadedBook(book);
    } catch (err) {
      setError(err instanceof Error ? err.message : "upload failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen bg-slate-950 px-6 py-10 text-slate-100">
      <div className="mx-auto max-w-lg">
        <div className="mb-8 flex items-center justify-between">
          <h1 className="text-2xl font-semibold">Upload a PDF</h1>
          <Link to="/" className="text-sm text-slate-400 hover:text-slate-100">
            Back to books
          </Link>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="title" className="mb-1 block text-sm text-slate-400">
              Title (optional)
            </label>
            <input
              id="title"
              type="text"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              className="w-full rounded-md border border-slate-800 bg-slate-900 px-3 py-2 text-sm outline-none focus:border-slate-500"
              placeholder="Defaults to the file name"
            />
          </div>

          <div>
            <label htmlFor="file" className="mb-1 block text-sm text-slate-400">
              PDF file
            </label>
            <input
              id="file"
              type="file"
              accept="application/pdf"
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              className="w-full text-sm text-slate-300"
            />
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="rounded-md bg-slate-100 px-4 py-2 text-sm font-medium text-slate-900 hover:bg-white disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting ? "Uploading…" : "Upload"}
          </button>
        </form>

        {error && <p className="mt-4 text-red-400">{error}</p>}

        {uploadedBook && (
          <div className="mt-6 rounded-md border border-slate-800 p-4">
            <p className="text-sm text-slate-400">Uploaded</p>
            <p className="font-medium">{uploadedBook.title}</p>
            <p className="mt-1 text-sm">
              Status: <span className="font-medium">{uploadedBook.status}</span>
            </p>
            <Link
              to={`/books/${uploadedBook.id}`}
              className="mt-3 inline-block text-sm text-slate-100 underline"
            >
              Open book
            </Link>
          </div>
        )}
      </div>
    </div>
  );
}

export default UploadPage;
