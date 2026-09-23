import assert from "node:assert/strict";
import { test } from "node:test";

import {
  buildBlockedPageURL,
  parseBlockedPageURL,
} from "./blocked_page.js";

test("blocked page URL round-trips attempted URL and decision context", () => {
  const blockedURL = buildBlockedPageURL(
    "chrome-extension://extension-id/blocked.html",
    "https://example.com/path?q=hello world",
    {
      profile: "study",
      allowed: false,
      reason: "not_in_allow_list",
    },
  );

  assert.deepEqual(parseBlockedPageURL(blockedURL), {
    attemptedURL: "https://example.com/path?q=hello world",
    profile: "study",
    reason: "not_in_allow_list",
  });
});
