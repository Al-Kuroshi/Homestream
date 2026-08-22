import { BrowserRouter } from "react-router-dom";
import { AppRoutes } from "./AppRoutes";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Sidebar } from "./components/Sidebar";
import "./App.css";

export function App() {
  return (
    <BrowserRouter>
      <div className="app-shell">
        <Sidebar />
        <main className="app-content">
          <ErrorBoundary>
            <AppRoutes />
          </ErrorBoundary>
        </main>
      </div>
    </BrowserRouter>
  );
}
