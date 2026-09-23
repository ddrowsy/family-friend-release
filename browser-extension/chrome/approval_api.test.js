import assert from "node:assert/strict";
import { test } from "node:test";

import { createApprovalAPI } from "./approval_api.js";

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

test("create request sends only child-safe request fields", async () => {
  let captured;
  const api = createApprovalAPI(async (url, options) => {
    captured = { url, options };
    return jsonResponse({
      request_id: "request-1",
      state: "pending",
      expires_at: "2026-09-23T05:00:00Z",
    }, 201);
  });

  const result = await api.createRequest({
    url: "https://example.com/private",
    browserProfile: "study",
  });

  assert.equal(captured.url, "http://127.0.0.1:17654/api/browser/access-requests");
  assert.equal(captured.options.method, "POST");
  assert.deepEqual(JSON.parse(captured.options.body), {
    url: "https://example.com/private",
    browser_profile: "study",
  });
  assert.equal(result.request_id, "request-1");
});

test("request status uses opaque request ID", async () => {
  let capturedURL;
  const api = createApprovalAPI(async (url) => {
    capturedURL = url;
    return jsonResponse({ request_id: "request-1", state: "approved" });
  });

  const result = await api.requestStatus("request-1");

  assert.equal(
    capturedURL,
    "http://127.0.0.1:17654/api/browser/access-requests/request-1",
  );
  assert.equal(result.state, "approved");
});

test("temporary direct approval sends duration", async () => {
  let capturedBody;
  const api = createApprovalAPI(async (_url, options) => {
    capturedBody = JSON.parse(options.body);
    return jsonResponse({ approved: true });
  });

  await api.approveDirect({
    url: "https://example.com",
    browserProfile: "study",
    approvalCode: "parent-secret",
    action: "temporary",
    durationSeconds: 900,
  });

  assert.deepEqual(capturedBody, {
    url: "https://example.com",
    browser_profile: "study",
    approval_code: "parent-secret",
    action: "temporary",
    duration_seconds: 900,
  });
});

test("permanent direct approval omits duration", async () => {
  let capturedBody;
  const api = createApprovalAPI(async (_url, options) => {
    capturedBody = JSON.parse(options.body);
    return jsonResponse({ approved: true });
  });

  await api.approveDirect({
    url: "https://example.com",
    browserProfile: "study",
    approvalCode: "parent-secret",
    action: "permanent",
    durationSeconds: 900,
  });

  assert.deepEqual(capturedBody, {
    url: "https://example.com",
    browser_profile: "study",
    approval_code: "parent-secret",
    action: "permanent",
  });
});

test("agent error surfaces stable error without request payload", async () => {
  const api = createApprovalAPI(async () => jsonResponse({ error: "invalid parent code" }, 401));

  await assert.rejects(
    () => api.approveDirect({
      url: "https://example.com",
      browserProfile: "study",
      approvalCode: "parent-secret",
      action: "permanent",
    }),
    (error) => {
      assert.match(error.message, /invalid parent code/);
      assert.doesNotMatch(error.message, /parent-secret/);
      return true;
    },
  );
});
