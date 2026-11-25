import { Provider } from "jotai/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles/tokens.css";

const container = document.getElementById("root");
if (!container) throw new Error("Missing #root mount point");

createRoot(container).render(
  <StrictMode>
    <Provider>
      <App />
    </Provider>
  </StrictMode>,
);
