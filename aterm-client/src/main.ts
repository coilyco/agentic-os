import { mount } from "svelte";
import App from "./App.svelte";
import "./app.css";
import { initCrashReporting } from "./lib/crash";

// Baked in at build time; unset leaves Sentry off.
initCrashReporting(import.meta.env.VITE_SENTRY_DSN);

const target = document.getElementById("app");
if (!target) throw new Error("index.html is missing #app");

export const app = mount(App, { target });
