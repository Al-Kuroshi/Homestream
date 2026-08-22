import { setupServer } from "msw/node";

// No default handlers: every test adds exactly the handlers it needs via
// server.use(...), so a test can never accidentally pass because of another
// test's leftover mock.
export const server = setupServer();
