import { Provider } from "jotai/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { TooltipProvider } from "@/components/ui/tooltip";
import { App } from "./App";
import "./styles/tokens.css";

const container = document.getElementById("root");
if (!container) throw new Error("Missing #root mount point");

createRoot(container).render(
  <StrictMode>
    <Provider>
      <TooltipProvider>
        <App />
      </TooltipProvider>
    </Provider>
  </StrictMode>,
);

if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    void navigator.serviceWorker.register("./sw.js").catch((error) => {
      console.error("Service worker registration failed", error);
    });
  });
}
