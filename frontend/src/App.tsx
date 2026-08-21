import { Route, Routes } from "react-router-dom";
import BookListPage from "./pages/BookListPage";

function App() {
  return (
    <Routes>
      <Route path="/" element={<BookListPage />} />
    </Routes>
  );
}

export default App;
