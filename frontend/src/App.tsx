import { Route, Routes } from "react-router-dom";
import BookListPage from "./pages/BookListPage";
import UploadPage from "./pages/UploadPage";

function App() {
  return (
    <Routes>
      <Route path="/" element={<BookListPage />} />
      <Route path="/upload" element={<UploadPage />} />
    </Routes>
  );
}

export default App;
