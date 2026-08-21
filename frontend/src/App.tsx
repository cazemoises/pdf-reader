import { Route, Routes } from "react-router-dom";
import BookListPage from "./pages/BookListPage";
import UploadPage from "./pages/UploadPage";
import ReaderPage from "./pages/ReaderPage";

function App() {
  return (
    <Routes>
      <Route path="/" element={<BookListPage />} />
      <Route path="/upload" element={<UploadPage />} />
      <Route path="/read/:id" element={<ReaderPage />} />
    </Routes>
  );
}

export default App;
