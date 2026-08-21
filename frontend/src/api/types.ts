export type BookStatus = "processing" | "ready" | "failed";

export interface Book {
  id: string;
  title: string;
  sourcePath: string;
  status: BookStatus;
  createdAt: string;
  updatedAt: string;
}

export interface ReadingProgress {
  bookId: string;
  lastPage: number;
  percentage: number;
  updatedAt: string;
}

export interface CharRange {
  start: number;
  end: number;
}

export interface Highlight {
  id: string;
  bookId: string;
  pageNumber: number;
  range: CharRange;
  color: string;
  createdAt: string;
}

export interface Page {
  bookId: string;
  number: number;
  text: string;
  width: number;
  height: number;
}

export interface Note {
  id: string;
  bookId: string;
  highlightId: string | null;
  content: string;
  createdAt: string;
}
