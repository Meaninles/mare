import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HashRouter } from "react-router-dom";
import { App } from "./App";
import { ThemeProvider } from "./components/ThemeProvider";
import { LibraryProvider } from "./context/LibraryContext";
import "./styles.css";

const queryClient = new QueryClient();

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <LibraryProvider>
          <HashRouter>
            <App />
          </HashRouter>
        </LibraryProvider>
      </QueryClientProvider>
    </ThemeProvider>
  </React.StrictMode>
);
