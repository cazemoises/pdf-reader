import type { Book } from "./types";

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
