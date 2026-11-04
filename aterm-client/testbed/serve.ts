// Serve the built client from the mock daemon, for looking at a journey's screen by hand.
import { startMock } from "./mock-daemon.ts";

const port = Number(process.argv[2] ?? 7420);
const mock = await startMock({ port });
console.log(`aterm client test bed: ${mock.url}  (Ctrl+C to stop)`);
process.on("SIGINT", () => void mock.stop().then(() => process.exit(0)));
