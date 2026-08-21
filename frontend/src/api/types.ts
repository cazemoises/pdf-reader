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

export interface BoundingBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Highlight {
  id: string;
  bookId: string;
  pageNumber: number;
  box: BoundingBox;
  color: string;
  createdAt: string;
}

export interface Note {
  id: string;
  bookId: string;
  highlightId: string | null;
  content: string;
  createdAt: string;
}
