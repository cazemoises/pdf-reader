export type BookStatus = "processing" | "ready" | "failed";

export interface Book {
  id: string;
  title: string;
  sourcePath: string;
  status: BookStatus;
  createdAt: string;
  updatedAt: string;
}
