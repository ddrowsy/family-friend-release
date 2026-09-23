import assert from "node:assert/strict";
import { test } from "node:test";

import {
  createNavigationHandler,
  createURLDecisionHandler,
  isWebURL,
} from "./navigation.js";

function createDecisionHandler(overrides = {}) {
  return createURLDecisionHandler({
    checkURL: async () => ({ allowed: true }),
    blockNavigation: async () => {},
    logger: { warn() {} },
    ...overrides,
  });
}

function createHandler(overrides = {}) {
  return createNavigationHandler({
    getActiveTab: async () => ({ id: 1 }),
    handleURLDecision: createDecisionHandler(),
    logger: { warn() {} },
    ...overrides,
  });
}

test("isWebURL accepts only HTTP and HTTPS URLs", () => {
  assert.equal(isWebURL("https://example.com"), true);
  assert.equal(isWebURL("http://example.com/path"), true);
  assert.equal(isWebURL("chrome://settings"), false);
  assert.equal(isWebURL("chrome-extension://abc/page.html"), false);
  assert.equal(isWebURL("file:///tmp/test"), false);
  assert.equal(isWebURL("not a url"), false);
});

test("URL decision handler leaves allowed navigation alone", async () => {
  let blocks = 0;
  const handler = createDecisionHandler({
    checkURL: async () => ({
      profile: "default",
      allowed: true,
      reason: "allowed_by_block_list",
    }),
    blockNavigation: async () => {
      blocks += 1;
    },
  });

  assert.equal(await handler(1, "https://example.com"), true);
  assert.equal(blocks, 0);
});

test("URL decision handler redirects blocked navigation", async () => {
  const blocks = [];
  const decision = {
    profile: "study",
    allowed: false,
    reason: "not_in_allow_list",
  };
  const handler = createDecisionHandler({
    checkURL: async () => decision,
    blockNavigation: async (...args) => {
      blocks.push(args);
    },
  });

  assert.equal(await handler(5, "https://tiktok.com"), false);
  assert.deepEqual(blocks, [[5, "https://tiktok.com", decision]]);
});

test("URL decision handler fails open on daemon error", async () => {
  const warnings = [];
  const handler = createDecisionHandler({
    checkURL: async () => {
      throw new Error("daemon unavailable");
    },
    logger: {
      warn(...args) {
        warnings.push(args);
      },
    },
  });

  assert.equal(await handler(1, "https://example.com"), false);
  assert.equal(warnings.length, 1);
});

test("URL decision handler logs blocked-page update failure", async () => {
  const warnings = [];
  const handler = createDecisionHandler({
    checkURL: async () => ({
      profile: "study",
      allowed: false,
      reason: "not_in_allow_list",
    }),
    blockNavigation: async () => {
      throw new Error("tab update failed");
    },
    logger: {
      warn(...args) {
        warnings.push(args);
      },
    },
  });

  assert.equal(await handler(1, "https://example.com"), false);
  assert.equal(warnings.length, 1);
});

test("URL decision handler fails open on invalid daemon decision", async () => {
  const warnings = [];
  const handler = createDecisionHandler({
    checkURL: async () => ({ profile: "study" }),
    logger: {
      warn(...args) {
        warnings.push(args);
      },
    },
  });

  assert.equal(await handler(1, "https://example.com"), false);
  assert.equal(warnings.length, 1);
});

test("navigation handler checks active top-frame web navigation", async () => {
  const checked = [];
  const handler = createHandler({
    getActiveTab: async () => ({ id: 7 }),
    handleURLDecision: async (tabId, url) => {
      checked.push([tabId, url]);
      return true;
    },
  });

  await handler({
    tabId: 7,
    frameId: 0,
    url: "https://example.com/page",
  });

  assert.deepEqual(checked, [[7, "https://example.com/page"]]);
});

test("navigation handler ignores subframes, inactive tabs, and non-web URLs", async () => {
  const checked = [];
  const handler = createHandler({
    handleURLDecision: async (...args) => {
      checked.push(args);
      return true;
    },
  });

  await handler({ tabId: 1, frameId: 1, url: "https://example.com/frame" });
  await handler({ tabId: 2, frameId: 0, url: "https://example.com/inactive" });
  await handler({ tabId: 1, frameId: 0, url: "chrome://settings" });
  await handler({
    tabId: 1,
    frameId: 0,
    url: "chrome-extension://abc/blocked.html",
  });

  assert.deepEqual(checked, []);
});

test("navigation handler deduplicates repeated allowed checks", async () => {
  let checks = 0;
  const handler = createHandler({
    getActiveTab: async () => ({ id: 4 }),
    handleURLDecision: async () => {
      checks += 1;
      return true;
    },
  });

  const details = {
    tabId: 4,
    frameId: 0,
    url: "https://example.com/repeated",
  };

  await handler(details);
  await handler(details);

  assert.equal(checks, 1);
});

test("navigation handler retries blocked or failed checks", async () => {
  let checks = 0;
  const handler = createHandler({
    getActiveTab: async () => ({ id: 6 }),
    handleURLDecision: async () => {
      checks += 1;
      return false;
    },
  });

  const details = {
    tabId: 6,
    frameId: 0,
    url: "https://example.com/retry",
  };

  await handler(details);
  await handler(details);

  assert.equal(checks, 2);
});

test("navigation handler logs active-tab lookup failures", async () => {
  let checks = 0;
  const warnings = [];
  const handler = createHandler({
    getActiveTab: async () => {
      throw new Error("tab unavailable");
    },
    handleURLDecision: async () => {
      checks += 1;
      return true;
    },
    logger: {
      warn(...args) {
        warnings.push(args);
      },
    },
  });

  await handler({
    tabId: 10,
    frameId: 0,
    url: "https://example.com",
  });

  assert.equal(checks, 0);
  assert.equal(warnings.length, 1);
});
