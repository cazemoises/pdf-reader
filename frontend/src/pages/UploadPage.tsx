import { useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router-dom";
import { uploadBook } from "../api/client";
import type { Book } from "../api/types";
import ThemeToggle from "../components/ThemeToggle";

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
    <div className="min-h-screen bg-background font-sans text-ink">
      <header className="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-elevated px-4 py-3.5 sm:px-8">
        <Link to="/" className="text-sm font-medium text-ink-muted hover:text-ink">
          ‹ Biblioteca
        </Link>
        <ThemeToggle />
      </header>

      <main className="mx-auto max-w-lg px-4 pb-16 pt-6 sm:px-8">
        <h1 className="mb-4 font-serif text-2xl font-semibold">Adicionar PDF</h1>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col items-center gap-3.5 rounded-xl border border-border bg-elevated px-5 py-7">
            <div className="h-[38px] w-[30px] rounded-sm border-2 border-ink-faint" />
            <p className="text-sm text-ink-muted">
              {file ? file.name : "Nenhum arquivo selecionado"}
            </p>
            <label className="flex h-12 w-full cursor-pointer items-center justify-center rounded-[10px] bg-accent px-4 text-sm font-semibold text-accent-text">
              Escolher arquivo do dispositivo
              <input
                id="file"
                type="file"
                accept="application/pdf"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                className="sr-only"
              />
            </label>
          </div>

          <div>
            <label htmlFor="title" className="mb-1 block text-sm text-ink-muted">
              Título (opcional)
            </label>
            <input
              id="title"
              type="text"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              className="w-full rounded-md border border-border bg-elevated px-3 py-2 text-sm text-ink outline-none focus:border-accent"
              placeholder="Usa o nome do arquivo por padrão"
            />
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="h-12 rounded-[10px] bg-accent text-sm font-semibold text-accent-text disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting ? "Enviando…" : "Enviar"}
          </button>
        </form>

        {submitting && (
          <div className="mt-4 flex items-center gap-3 rounded-[10px] border border-border bg-elevated p-4">
            <div className="h-[18px] w-[18px] shrink-0 animate-spin rounded-full border-2 border-border border-t-accent" />
            <div>
              <p className="font-mono text-xs">{file?.name}</p>
              <p className="mt-0.5 text-[11.5px] text-ink-muted">
                Extraindo texto e gerando capa…
              </p>
            </div>
          </div>
        )}

        {error && (
          <div className="mt-4 flex flex-col gap-2.5 rounded-[10px] border border-border bg-danger-soft p-4">
            {file && <p className="font-mono text-xs">{file.name}</p>}
            <p className="text-xs font-semibold leading-relaxed text-danger">{error}</p>
          </div>
        )}

        {uploadedBook && (
          <div className="mt-4 flex items-center justify-between gap-3 rounded-[10px] border border-border bg-elevated p-4">
            <div className="flex items-center gap-2.5">
              <span className="h-2 w-2 shrink-0 rounded-full bg-accent" />
              <div>
                <p className="font-mono text-xs">{uploadedBook.title}</p>
                <p className="mt-0.5 text-[11.5px] font-semibold text-ink">
                  {uploadedBook.status === "ready" ? "Pronto" : uploadedBook.status}
                </p>
              </div>
            </div>
            <Link
              to={`/read/${uploadedBook.id}`}
              className="shrink-0 text-xs font-semibold text-accent"
            >
              Ver →
            </Link>
          </div>
        )}
      </main>
    </div>
  );
}

export default UploadPage;
