import { rm } from "node:fs/promises";

await rm(new URL("../out", import.meta.url), { recursive: true, force: true });
