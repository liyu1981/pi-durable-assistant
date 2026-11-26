import { spawn } from "node:child_process";

// `pnpm dev:demo` (or DEMO=1) runs the deterministic faux provider next to the
// dev server; plain `pnpm dev` starts the server without it and expects a real
// provider configured through the environment or the pida.env file.
const demo = process.argv.includes("--demo") || process.env.DEMO === "1";

const env = {
  ...process.env,
  DEV: "1",
  // Explicitly force HTTPS so a local pida.env cannot silently switch dev to
  // plain HTTP (the env var wins over the config file).
  HTTP_MODE: "https",
  TLS_CERT_FILE: "certs/localhost.pem",
  TLS_KEY_FILE: "certs/localhost-key.pem",
};

if (demo) {
  Object.assign(env, {
    LLM_API: "openai-completions",
    LLM_BASE_URL: "http://127.0.0.1:18789/v1",
    LLM_API_KEY: "faux",
    LLM_MODEL: "faux-1",
    FAUX_PORT: "18789",
  });
}

const children = [];
let stopping = false;

function start(name, command, args, extraEnv = {}) {
  const child = spawn(command, args, {
    stdio: "inherit",
    env: { ...env, ...extraEnv },
  });
  child.on("error", (error) => {
    console.error(`${name} failed to start:`, error.message);
    if (name === "air") console.error("Install it with: pnpm dev:install");
  });
  children.push(child);
  return child;
}

async function waitForFaux() {
  for (let attempt = 0; attempt < 80; attempt += 1) {
    try {
      const response = await fetch("http://127.0.0.1:18789/v1/models");
      if (response.ok) return;
    } catch {
      // Provider is still compiling/starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error("faux provider did not become ready on 127.0.0.1:18789");
}

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

if (demo) {
  // Start the development-only OpenAI-compatible provider first.
  const faux = start("faux", "go", ["run", "./cmd/faux-llm"]);
  faux.on("exit", (code) => {
    if (!stopping) {
      console.error(`faux provider exited with code ${code}`);
      stop();
    }
  });

  try {
    await waitForFaux();
    console.log("faux provider ready; starting air");
  } catch (error) {
    console.error(error.message);
    stop();
    process.exit(1);
  }
}

// Air watches Go + web sources, debounces changes, rebuilds web then Go, and restarts the binary.
const air = start("air", "air", ["-c", ".air.toml"]);
air.on("exit", (code) => {
  stop();
  process.exit(code ?? 0);
});
