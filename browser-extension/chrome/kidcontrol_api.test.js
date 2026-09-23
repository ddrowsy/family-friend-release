import assert from "node:assert/strict";
import { afterEach, test } from "node:test";

import { checkURL, endVisit } from "./kidcontrol_api.js";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

test("checkURL posts URL and returns daemon decision", async () => {
  const expected = {
    profile: "study",
    allowed: true,
    reason: "allowed_by_allow_list",
  };

  let requestURL;
  let requestOptions;
  globalThis.fetch = async (url, options) => {
    requestURL = url;
    requestOptions = options;
    return {
      ok: true,
      status: 200,
      json: async () => expected,
    };
  };

  const result = await checkURL("https://wikipedia.org/wiki/Go");

  assert.deepEqual(result, expected);
  assert.equal(requestURL, "http://127.0.0.1:17653/api/browser/check");
  assert.equal(requestOptions.method, "POST");
  assert.equal(requestOptions.headers["Content-Type"], "application/json");
  assert.deepEqual(
    JSON.parse(requestOptions.body),
    { url: "https://wikipedia.org/wiki/Go" },
  );
});

test("checkURL throws on daemon error response", async () => {
  globalThis.fetch = async () => ({
    ok: false,
    status: 500,
    text: async () => "browser monitor is not started",
  });

  await assert.rejects(
    checkURL("https://example.com"),
    /KidControl check failed: browser monitor is not started/,
  );
});

test("checkURL surfaces network failure", async () => {
  globalThis.fetch = async () => {
    throw new Error("connection refused");
  };

  await assert.rejects(checkURL("https://example.com"), /connection refused/);
});

test("endVisit posts to daemon and accepts 204", async () => {
  let requestURL;
  let requestOptions;
  globalThis.fetch = async (url, options) => {
    requestURL = url;
    requestOptions = options;
    return {
      status: 204,
    };
  };

  await endVisit();

  assert.equal(requestURL, "http://127.0.0.1:17653/api/browser/end-visit");
  assert.equal(requestOptions.method, "POST");
});

test("endVisit throws on non-204 response", async () => {
  globalThis.fetch = async () => ({
    status: 500,
    text: async () => "stop failed",
  });

  await assert.rejects(
    endVisit(),
    /KidControl end-visit failed: stop failed/,
  );
});

test("endVisit surfaces network failure", async () => {
  globalThis.fetch = async () => {
    throw new Error("connection refused");
  };

  await assert.rejects(endVisit(), /connection refused/);
});
