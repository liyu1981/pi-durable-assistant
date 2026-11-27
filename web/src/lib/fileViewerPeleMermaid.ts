// Pele-backed Mermaid shim for @file-viewer/renderer-text.
//
// The text renderer expects mermaid.initialize(...) and render(id, source);
// Pele exposes render(source) -> { svg }. The theme is injected into the SVG
// so diagram colours follow the viewer toolbar's light/dark toggle.
import { render as peleRender } from "pele";

type Theme = "light" | "dark";

const DARK_VARS =
  "--pele-bg:#1e1e1e;--pele-fg:#e6e6e6;--pele-muted:#a0a0a0;" +
  "--pele-line:#e6e6e6;--pele-surface:#2a2a2a;--pele-surface-alt:#333333;" +
  "--pele-border:#555555;--pele-accent:#7aa2f7;";

let theme: Theme = "light";

function injectVars(svg: string, vars: string): string {
  if (/<svg\b[^>]*\bstyle="/.test(svg)) {
    return svg.replace(/(<svg\b[^>]*\bstyle=")([^"]*)(")/, `$1$2;${vars}$3`);
  }
  return svg.replace(/<svg\b/, `<svg style="${vars}"`);
}

const mermaid = {
  initialize: (options: { theme?: string }) => {
    theme = options?.theme === "dark" ? "dark" : "light";
  },
  render: async (_id: string, source: string) => {
    const svg = peleRender(source).svg;
    return { svg: theme === "dark" ? injectVars(svg, DARK_VARS) : svg };
  },
};

export default mermaid;
