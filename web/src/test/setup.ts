/// <reference types="node" />
// Pin the test-runner timezone to UTC, before any other module runs. Several
// tests (e.g. web/src/scheduling/time.test.ts's formatTimeRange test) assert
// on formatted local-time strings; pinning TZ=UTC here keeps that
// deterministic across machines without hardcoding UTC into the production
// formatting itself (see web/src/scheduling/time.ts).
process.env.TZ = "UTC";

import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll } from "vitest";
import { server } from "./server";

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterEach(() => cleanup());
afterAll(() => server.close());
