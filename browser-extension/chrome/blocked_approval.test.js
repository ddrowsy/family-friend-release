import assert from "node:assert/strict";
import { test } from "node:test";

import { createBlockedApprovalController } from "./blocked_approval.js";

const context = {
  attemptedURL: "https://example.com/private",
  profile: "study",
  reason: "not_in_allow_list",
};

function createHarness(apiOverrides = {}) {
  const renders = [];
  const navigations = [];
  const timers = [];
  const clearedTimers = [];
  let nextTimerID = 1;

  const api = {
    async createRequest() {
      return { request_id: "request-1", state: "pending" };
    },
    async requestStatus() {
      return { request_id: "request-1", state: "pending" };
    },
    async approveDirect() {
      return { approved: true };
    },
    ...apiOverrides,
  };

  const controller = createBlockedApprovalController({
    context,
    api,
    render(state) {
      renders.push(state);
    },
    navigate(url) {
      navigations.push(url);
    },
    setTimeoutImpl(callback) {
      const timer = { id: nextTimerID, callback };
      nextTimerID += 1;
      timers.push(timer);
      return timer.id;
    },
    clearTimeoutImpl(timerID) {
      clearedTimers.push(timerID);
    },
    pollIntervalMs: 1,
  });

  return {
    api,
    controller,
    renders,
    navigations,
    timers,
    clearedTimers,
  };
}

async function runLatestTimer(harness) {
  const timer = harness.timers.at(-1);
  assert.ok(timer, "expected scheduled poll");
  await timer.callback();
}

test("initial blocked page state is idle", () => {
  const harness = createHarness();

  assert.equal(harness.renders.length, 1);
  assert.equal(harness.renders[0].status, "idle");
  assert.equal(harness.renders[0].pendingRequestID, "");
});

test("ask parent creates one child-safe request and polls pending status", async () => {
  const requests = [];
  const harness = createHarness({
    async createRequest(request) {
      requests.push(request);
      return { request_id: "request-1", state: "pending" };
    },
  });

  await harness.controller.askParent();
  await harness.controller.askParent();

  assert.deepEqual(requests, [{
    url: context.attemptedURL,
    browserProfile: context.profile,
  }]);
  assert.equal(harness.renders.at(-1).status, "waiting");
  assert.equal(harness.renders.at(-1).pendingRequestID, "request-1");
  assert.ok(harness.timers.length >= 1);
});

test("ask parent service failure shows error without bypass", async () => {
  const harness = createHarness({
    async createRequest() {
      throw new Error("Request access failed: control service unavailable");
    },
  });

  await harness.controller.askParent();

  assert.equal(harness.renders.at(-1).status, "error");
  assert.match(harness.renders.at(-1).message, /control service unavailable/);
  assert.deepEqual(harness.navigations, []);
});

test("approved remote request retries original URL", async () => {
  let statusCalls = 0;
  const harness = createHarness({
    async requestStatus() {
      statusCalls += 1;
      return { request_id: "request-1", state: "approved" };
    },
  });

  await harness.controller.askParent();
  await runLatestTimer(harness);

  assert.equal(statusCalls, 1);
  assert.equal(harness.renders.at(-1).status, "approved");
  assert.deepEqual(harness.navigations, [context.attemptedURL]);
});

test("rejected remote request stops without navigation", async () => {
  const harness = createHarness({
    async requestStatus() {
      return { request_id: "request-1", state: "rejected" };
    },
  });

  await harness.controller.askParent();
  await runLatestTimer(harness);

  assert.equal(harness.renders.at(-1).status, "rejected");
  assert.deepEqual(harness.navigations, []);
});

test("expired remote request allows a new request", async () => {
  let createCalls = 0;
  const harness = createHarness({
    async createRequest() {
      createCalls += 1;
      return { request_id: `request-${createCalls}`, state: "pending" };
    },
    async requestStatus() {
      return { request_id: "request-1", state: "expired" };
    },
  });

  await harness.controller.askParent();
  await runLatestTimer(harness);
  await harness.controller.askParent();

  assert.equal(createCalls, 2);
  assert.equal(harness.renders.at(-1).status, "waiting");
});

test("service failure shows error and pending status can resume", async () => {
  let statusCalls = 0;
  const harness = createHarness({
    async requestStatus() {
      statusCalls += 1;
      if (statusCalls === 1) {
        throw new Error("Checking request failed: control service unavailable");
      }
      return { request_id: "request-1", state: "pending" };
    },
  });

  await harness.controller.askParent();
  await runLatestTimer(harness);

  assert.equal(harness.renders.at(-1).status, "error");
  assert.equal(harness.renders.at(-1).canResume, true);

  harness.controller.resumePolling();
  await runLatestTimer(harness);

  assert.equal(statusCalls, 2);
  assert.equal(harness.renders.at(-1).status, "waiting");
});

test("temporary parent approval forwards chosen duration and retries URL", async () => {
  let approval;
  const harness = createHarness({
    async approveDirect(request) {
      approval = request;
      return { approved: true };
    },
  });

  const approved = await harness.controller.approveParent({
    approvalCode: "parent-secret",
    action: "temporary",
    durationSeconds: 1800,
  });

  assert.equal(approved, true);
  assert.deepEqual(approval, {
    url: context.attemptedURL,
    browserProfile: context.profile,
    approvalCode: "parent-secret",
    action: "temporary",
    durationSeconds: 1800,
  });
  assert.deepEqual(harness.navigations, [context.attemptedURL]);
});

test("permanent parent approval does not require duration", async () => {
  let approval;
  const harness = createHarness({
    async approveDirect(request) {
      approval = request;
      return { approved: true };
    },
  });

  const approved = await harness.controller.approveParent({
    approvalCode: "parent-secret",
    action: "permanent",
  });

  assert.equal(approved, true);
  assert.equal(approval.action, "permanent");
  assert.equal(approval.durationSeconds, undefined);
});

test("temporary parent approval rejects invalid duration locally", async () => {
  let approvalCalls = 0;
  const harness = createHarness({
    async approveDirect() {
      approvalCalls += 1;
      return { approved: true };
    },
  });

  const approved = await harness.controller.approveParent({
    approvalCode: "parent-secret",
    action: "temporary",
    durationSeconds: 0,
  });

  assert.equal(approved, false);
  assert.equal(approvalCalls, 0);
  assert.equal(harness.renders.at(-1).status, "parent-error");
});

test("invalid parent code error does not navigate", async () => {
  const harness = createHarness({
    async approveDirect() {
      throw new Error("Parent approval failed: invalid parent code");
    },
  });

  const approved = await harness.controller.approveParent({
    approvalCode: "wrong-code",
    action: "permanent",
  });

  assert.equal(approved, false);
  assert.equal(harness.renders.at(-1).status, "parent-error");
  assert.match(harness.renders.at(-1).message, /invalid parent code/);
  assert.deepEqual(harness.navigations, []);
});

test("stop cancels polling and prevents later navigation", async () => {
  const harness = createHarness({
    async requestStatus() {
      return { request_id: "request-1", state: "approved" };
    },
  });

  await harness.controller.askParent();
  const scheduledTimer = harness.timers.at(-1);
  harness.controller.stop();
  await scheduledTimer.callback();

  assert.ok(harness.clearedTimers.includes(scheduledTimer.id));
  assert.deepEqual(harness.navigations, []);
});
