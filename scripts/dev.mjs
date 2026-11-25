import { spawn } from "node:child_process";

const env = {
  ...process.env,
  DEV: "1",
  LLM_API: "openai-completions",
  LLM_BASE_URL: "http://127.0.0.1:18789/v1",
  LLM_API_KEY: "faux",
  LLM_MODEL: "faux-1",
  FAUX_PORT: "18789",
};

const children = [];
let stopping = false;

function start(name, command, args, extraEnv = {}) {
  const child = spawn(command, args, {
    stdio: "inherit",
    env: { ...env, ...extraEnv },
  });
  child.on("error", (error) => {
    console.error(`${name} failed to start:`, error.message);
    if (name === "air") {
      console.error("Install it with: pnpm dev:install");
    }
  });
  children.push(child);
  return child;
}

// Development-only OpenAI-compatible provider, outside the main binary.
start("faux", "go", ["run", "./cmd/faux-llm"]);

// Air watches Go + web sources, debounces changes, rebuilds web then Go, and restarts the binary.
const air = start("air", "air", ["-c", ".air.toml"]);

function stop(signal = "SIGTERM") {
  if (stopping) return;
  stopping = true;
  for (const child of children) {
    if (!child.killed) child.kill(signal);
  }
  setTimeout(() => process.exit(0), 300).unref();
}

process.on("SIGINT", () => stop("SIGINT"));
process.on("SIGTERM", () => stop("SIGTERM"));
air.on("exit", (code) => {
  stop();
  process.exit(code ?? 0);
});
