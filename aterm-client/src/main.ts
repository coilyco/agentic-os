import { mount } from "svelte";
import App from "./App.svelte";
import "./app.css";
import { initCrashReporting } from "./lib/crash";
import TapTraceBar from "./components/TapTraceBar.svelte";
import { registerShellWorker } from "./lib/shell-worker";
import { startTrace } from "./lib/tap-trace";

// Baked in at build time; unset leaves Sentry off.
initCrashReporting(import.meta.env.VITE_SENTRY_DSN);

registerShellWorker();

const target = document.getElementById("app");
if (!target) throw new Error("index.html is missing #app");

export const app = mount(App, { target });

// Off unless the page is opened with ?taptrace (COI-3334).
if (startTrace(window)) mount(TapTraceBar, { target: document.body });
