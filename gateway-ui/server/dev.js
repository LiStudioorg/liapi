/**
 * 一键开发脚本：同时启动后端 API(3002) 与前端 Vite(5173)。
 * 用法：npm run dev
 */
import { spawn } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = resolve(__dirname, "..");
const npx = process.platform === "win32" ? "npx.cmd" : "npx";

const procs = [];

function run(label, color, cmd, args) {
  const p = spawn(cmd, args, { cwd: root, stdio: ["ignore", "pipe", "pipe"], env: process.env });
  const tag = `\x1b[${color}m[${label}]\x1b[0m`;
  const pipe = (stream, isErr) => {
    stream.setEncoding("utf8");
    let buf = "";
    stream.on("data", (chunk) => {
      buf += chunk;
      const lines = buf.split("\n");
      buf = lines.pop() ?? "";
      for (const l of lines) if (l.trim()) console.log(isErr ? tag + " " + l : tag + " " + l);
    });
  };
  pipe(p.stdout, false);
  pipe(p.stderr, true);
  p.on("exit", (code) => console.log(`${tag} 退出，code=${code}`));
  procs.push(p);
  return p;
}

run("api", "36", process.execPath, [resolve(__dirname, "index.js")]);
run("web", "35", npx, ["vite", "--host", "0.0.0.0"]);

const shutdown = () => {
  for (const p of procs) p.kill("SIGTERM");
  process.exit(0);
};
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
