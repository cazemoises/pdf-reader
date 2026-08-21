import type { Book, ReadingProgress } from "./types";

async function parseJSONOrThrow<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`request failed (${res.status}): ${body}`);
  }
  return res.json() as Promise<T>;
}

export async function listBooks(): Promise<Book[]> {
  const res = await fetch("/books");
  const books = await parseJSONOrThrow<Book[] | null>(res);
  return books ?? [];
}

export async function getBook(id: string): Promise<Book> {
  const res = await fetch(`/books/${id}`);
  return parseJSONOrThrow<Book>(res);
}

export async function uploadBook(file: File, title: string): Promise<Book> {
  const form = new FormData();
  form.append("file", file);
  if (title) {
    form.append("title", title);
  }

  const res = await fetch("/books", {
    method: "POST",
    body: form,
  });
  return parseJSONOrThrow<Book>(res);
}

export function bookFileUrl(id: string): string {
  return `/books/${id}/file`;
}

// Returns null when the book has no saved progress yet (backend 404s in
// that case) instead of treating it as an error.
export async function getProgress(bookId: string): Promise<ReadingProgress | null> {
  const res = await fetch(`/books/${bookId}/progress`);
  if (res.status === 404) {
    return null;
  }
  return parseJSONOrThrow<ReadingProgress>(res);
}

// percentage is 0-100, matching the backend's domain.ReadingProgress validation.
export async function saveProgress(
  bookId: string,
  lastPage: number,
  percentage: number,
): Promise<ReadingProgress> {
  const res = await fetch(`/books/${bookId}/progress`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ lastPage, percentage }),
  });
  return parseJSONOrThrow<ReadingProgress>(res);
}
