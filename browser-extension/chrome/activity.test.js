import assert from "node:assert/strict";
import { test } from "node:test";

import { createBrowserActivityHandler } from "./activity.js";

function createActivity(overrides = {}) {
  const events = [];

  const activity = createBrowserActivityHandler({
    endVisit: async () => {
      events.push(["end"]);
    },
    getActiveTabInWindow: async (windowId) => ({
      id: windowId * 10,
      windowId,
    }),
    getFrameURL: async (tabId) => `https://example.com/tab-${tabId}`,
    getLastFocusedWindow: async () => ({ id: 1, focused: true }),
    isWindowFocused: async () => true,
    handleURLDecision: async (tabId, url) => {
      events.push(["check", tabId, url]);
      return true;
    },
    windowIDNone: -1,
    logger: { warn() {} },
    ...overrides,
  });

  return { activity, events };
}

test("initialize checks the active tab in the focused window", async () => {
  const { activity, events } = createActivity();

  await activity.initialize();

  assert.deepEqual(events, [
    ["check", 10, "https://example.com/tab-10"],
  ]);
  assert.deepEqual(activity.getState(), {
    activeTabId: 10,
    activeWindowId: 1,
  });
});

test("active tab change ends previous visit before checking new tab", async () => {
  const { activity, events } = createActivity();

  await activity.initialize();
  events.length = 0;

  await activity.handleTabActivated({
    tabId: 22,
    windowId: 2,
  });

  assert.deepEqual(events, [
    ["end"],
    ["check", 22, "https://example.com/tab-22"],
  ]);
});

test("tab activation in an unfocused window does not affect current visit", async () => {
  const { activity, events } = createActivity({
    isWindowFocused: async () => false,
  });

  await activity.initialize();
  events.length = 0;

  await activity.handleTabActivated({
    tabId: 22,
    windowId: 2,
  });

  assert.deepEqual(events, []);
});

test("window focus loss ends current visit", async () => {
  const { activity, events } = createActivity();

  await activity.initialize();
  events.length = 0;

  await activity.handleWindowFocusChanged(-1);

  assert.deepEqual(events, [["end"]]);
  assert.deepEqual(activity.getState(), {
    activeTabId: null,
    activeWindowId: null,
  });
});

test("focus moving to another Chrome window ends then checks its active tab", async () => {
  const { activity, events } = createActivity();

  await activity.initialize();
  events.length = 0;

  await activity.handleWindowFocusChanged(3);

  assert.deepEqual(events, [
    ["end"],
    ["check", 30, "https://example.com/tab-30"],
  ]);
});

test("closing active tab ends visit while closing background tab does not", async () => {
  const { activity, events } = createActivity();

  await activity.initialize();
  events.length = 0;

  await activity.handleTabRemoved(99);
  assert.deepEqual(events, []);

  await activity.handleTabRemoved(10);
  assert.deepEqual(events, [["end"]]);
});

test("non-web active tab ends previous visit but does not start a new one", async () => {
  const { activity, events } = createActivity({
    getFrameURL: async (tabId) => (
      tabId === 22 ? "chrome://settings" : "https://example.com"
    ),
  });

  await activity.initialize();
  events.length = 0;

  await activity.handleTabActivated({
    tabId: 22,
    windowId: 2,
  });

  assert.deepEqual(events, [["end"]]);
});

test("duplicate boundary events do not send duplicate end calls", async () => {
  const { activity, events } = createActivity();

  await activity.initialize();
  events.length = 0;

  await activity.handleWindowFocusChanged(-1);
  await activity.handleWindowFocusChanged(-1);

  assert.deepEqual(events, [["end"]]);
});

test("end-visit failure is logged and does not stop the next tab check", async () => {
  const warnings = [];
  const events = [];
  const { activity } = createActivity({
    endVisit: async () => {
      events.push(["end"]);
      throw new Error("daemon unavailable");
    },
    handleURLDecision: async (tabId, url) => {
      events.push(["check", tabId, url]);
      return true;
    },
    logger: {
      warn(...args) {
        warnings.push(args);
      },
    },
  });

  await activity.initialize();
  events.length = 0;

  await activity.handleTabActivated({
    tabId: 22,
    windowId: 2,
  });

  assert.deepEqual(events, [
    ["end"],
    ["check", 22, "https://example.com/tab-22"],
  ]);
  assert.equal(warnings.length, 1);
});
