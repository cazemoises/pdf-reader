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
