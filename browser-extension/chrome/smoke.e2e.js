import assert from "node:assert/strict";
import { createServer } from "node:http";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

const KIDCONTROL_API_PORT = 17653;
const APPROVAL_API_PORT = 17654;
const HOST = "127.0.0.1";
const extensionPath = fileURLToPath(new URL(".", import.meta.url));

function listen(server, port = 0) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, HOST, () => {
      server.off("error", reject);
      resolve(server.address());
    });
  });
}

function close(server) {
  if (!server.listening) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    server.close((error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
}

async function readJSON(request) {
  const chunks = [];
  for await (const chunk of request) {
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

function writeJSON(response, status, body) {
  response.writeHead(status, { "Content-Type": "application/json" });
  response.end(JSON.stringify(body));
}

async function waitFor(predicate, message, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await predicate()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  assert.fail(message);
}

function blockedPageMatches(page, blockedPageURL, attemptedURL) {
  if (!page.url().startsWith(blockedPageURL)) {
    return false;
  }
  return new URL(page.url()).searchParams.get("url") === attemptedURL;
}

async function openBlockedPage({
  context,
  serviceWorker,
  checks,
  blockedPageURL,
  attemptedURL,
}) {
  const tabID = await serviceWorker.evaluate(async () => {
    const tab = await chrome.tabs.create({ active: true, url: "about:blank" });
    return tab.id;
  });
  assert.notEqual(tabID, undefined);

  await serviceWorker.evaluate(
    ({ tabId, url }) => chrome.tabs.update(tabId, { url }),
    { tabId: tabID, url: attemptedURL },
  );

  await waitFor(
    () => checks.includes(attemptedURL),
    `extension did not send ${attemptedURL} to KidControl`,
  );
  await waitFor(
    () => context.pages().some(
      (page) => blockedPageMatches(page, blockedPageURL, attemptedURL),
    ),
    `blocked tab did not navigate to blocked.html for ${attemptedURL}`,
  );

  const page = context.pages().find(
    (candidate) => blockedPageMatches(candidate, blockedPageURL, attemptedURL),
  );
  assert.ok(page);
  await page.waitForFunction(
    (expectedURL) => document.getElementById("blocked-url")?.textContent === expectedURL,
    attemptedURL,
    { timeout: 10000 },
  );
  return page;
}

async function waitForAllowedPage(page, expectedURL) {
  await waitFor(
    () => page.url() === expectedURL,
    `approved page did not retry ${expectedURL}`,
  );
  assert.equal(await page.locator("#page").textContent(), "Allowed target");
}

test("Chromium extension handles blocked-page approval flows", { timeout: 120000 }, async () => {
  const checks = [];
  const approvedURLs = new Set();
  const accessRequests = [];
  const directApprovals = [];
  let endVisits = 0;
  let nextRequestID = 1;
  let pageBaseURL = "";

  const kidControlServer = createServer(async (request, response) => {
    if (request.method === "POST" && request.url === "/api/browser/check") {
      const body = await readJSON(request);
      checks.push(body.url);

      const parsedURL = new URL(body.url);
      const requiresApproval = parsedURL.pathname.startsWith("/blocked");
      const allowed = !requiresApproval || approvedURLs.has(body.url);
      writeJSON(response, 200, {
        profile: requiresApproval ? "study" : "default",
        allowed,
        reason: allowed ? "allowed_by_block_list" : "not_in_allow_list",
      });
      return;
    }

    if (request.method === "POST" && request.url === "/api/browser/end-visit") {
      endVisits += 1;
      response.writeHead(204);
      response.end();
      return;
    }

    response.writeHead(404);
    response.end();
  });

  const approvalServer = createServer(async (request, response) => {
    if (request.method === "POST" && request.url === "/api/browser/access-requests") {
      const body = await readJSON(request);
      const accessRequest = {
        id: `request-${nextRequestID}`,
        url: body.url,
        browserProfile: body.browser_profile,
        state: "pending",
      };
      nextRequestID += 1;
      accessRequests.push(accessRequest);
      writeJSON(response, 201, {
        request_id: accessRequest.id,
        state: accessRequest.state,
        expires_at: "2026-09-23T06:00:00Z",
      });
      return;
    }

    const requestMatch = request.url?.match(/^\/api\/browser\/access-requests\/(.+)$/);
    if (request.method === "GET" && requestMatch) {
      const requestID = decodeURIComponent(requestMatch[1]);
      const accessRequest = accessRequests.find((candidate) => candidate.id === requestID);
      if (!accessRequest) {
        writeJSON(response, 404, { error: "access request not found" });
        return;
      }
      writeJSON(response, 200, {
        request_id: accessRequest.id,
        state: accessRequest.state,
        expires_at: "2026-09-23T06:00:00Z",
      });
      return;
    }

    if (request.method === "POST" && request.url === "/api/browser/approval") {
      const body = await readJSON(request);
      directApprovals.push(body);
      if (body.approval_code === "bad-code") {
        writeJSON(response, 401, { error: "invalid parent code" });
        return;
      }
      approvedURLs.add(body.url);
      writeJSON(response, 200, { approved: true });
      return;
    }

    response.writeHead(404);
    response.end();
  });

  const pageServer = createServer((request, response) => {
    const requestedURL = `${pageBaseURL}${request.url}`;
    const blocked = request.url?.startsWith("/blocked");
    if (blocked && !approvedURLs.has(requestedURL)) {
      // Keep the first blocked navigation pending. The extension must replace
      // it with blocked.html; once approved, the retry receives normal HTML.
      return;
    }

    response.writeHead(200, { "Content-Type": "text/html" });
    response.end(`<!doctype html>
<html>
<body>
  <h1 id="page">Allowed target</h1>
</body>
</html>`);
  });

  let context;
  try {
    await listen(kidControlServer, KIDCONTROL_API_PORT);
    await listen(approvalServer, APPROVAL_API_PORT);
    const pageAddress = await listen(pageServer);
    pageBaseURL = `http://${HOST}:${pageAddress.port}`;

    context = await chromium.launchPersistentContext("", {
      channel: "chromium",
      headless: true,
      args: [
        `--disable-extensions-except=${extensionPath}`,
        `--load-extension=${extensionPath}`,
      ],
    });

    let [serviceWorker] = context.serviceWorkers();
    if (!serviceWorker) {
      serviceWorker = await context.waitForEvent("serviceworker");
    }

    const extensionID = new URL(serviceWorker.url()).host;
    assert.ok(extensionID, "extension service worker should expose an extension ID");
    const blockedPageURL = `chrome-extension://${extensionID}/blocked.html`;

    const firstTabID = await serviceWorker.evaluate(async () => {
      const tab = await chrome.tabs.create({ active: true, url: "about:blank" });
      return tab.id;
    });
    assert.notEqual(firstTabID, undefined);

    const allowedURL = `${pageBaseURL}/allowed`;
    await serviceWorker.evaluate(
      ({ tabId, url }) => chrome.tabs.update(tabId, { url }),
      { tabId: firstTabID, url: allowedURL },
    );

    await waitFor(
      () => checks.includes(allowedURL),
      "extension did not send the allowed URL to KidControl",
    );
    await waitFor(
      () => context.pages().some((page) => page.url() === allowedURL),
      "allowed tab did not remain on the requested URL",
    );
    const allowedPage = context.pages().find((page) => page.url() === allowedURL);
    assert.ok(allowedPage);
    assert.equal(await allowedPage.locator("#page").textContent(), "Allowed target");

    const endsBeforeTabSwitch = endVisits;
    const askParentURL = `${pageBaseURL}/blocked-ask-parent`;
    const askParentPage = await openBlockedPage({
      context,
      serviceWorker,
      checks,
      blockedPageURL,
      attemptedURL: askParentURL,
    });
    await waitFor(
      () => endVisits > endsBeforeTabSwitch,
      "activating the blocked tab did not end the previous visit",
    );

    assert.equal(await askParentPage.locator("#blocked-profile").textContent(), "study");
    assert.equal(
      await askParentPage.locator("#blocked-reason").textContent(),
      "not_in_allow_list",
    );
    assert.equal(await askParentPage.locator("#parent-panel").isHidden(), true);
    assert.equal(await askParentPage.locator("[id*='temporary-code']").count(), 0);

    await askParentPage.locator("#ask-parent-button").click();
    await waitFor(
      () => accessRequests.some((request) => request.url === askParentURL),
      "Ask parent did not create an access request",
    );
    await askParentPage.locator("#access-status").waitFor({
      state: "visible",
      timeout: 10000,
    });
    await waitFor(
      async () => (await askParentPage.locator("#access-status").textContent())
        ?.includes("Waiting for parent approval"),
      "Ask parent did not remain in the pending state",
    );

    const askParentRequest = accessRequests.find((request) => request.url === askParentURL);
    assert.ok(askParentRequest);
    assert.equal(askParentRequest.browserProfile, "study");
    askParentRequest.state = "approved";
    approvedURLs.add(askParentURL);
    await waitForAllowedPage(askParentPage, askParentURL);

    const temporaryURL = `${pageBaseURL}/blocked-temporary`;
    const temporaryPage = await openBlockedPage({
      context,
      serviceWorker,
      checks,
      blockedPageURL,
      attemptedURL: temporaryURL,
    });
    await temporaryPage.locator("#parent-toggle-button").click();
    assert.equal(await temporaryPage.locator("#duration-field").isVisible(), true);
    await temporaryPage.locator("#parent-code").fill("parent-temp-secret");
    assert.doesNotMatch(temporaryPage.url(), /parent-temp-secret/);
    await temporaryPage.locator("#approval-action").selectOption("temporary");
    await temporaryPage.locator("#approval-duration").selectOption("1800");
    await temporaryPage.locator("#parent-submit-button").click();
    await waitFor(
      () => directApprovals.some((approval) => approval.url === temporaryURL),
      "temporary parent approval was not sent",
    );
    const temporaryApproval = directApprovals.find(
      (approval) => approval.url === temporaryURL,
    );
    assert.equal(temporaryApproval.action, "temporary");
    assert.equal(temporaryApproval.duration_seconds, 1800);
    await waitForAllowedPage(temporaryPage, temporaryURL);

    const permanentURL = `${pageBaseURL}/blocked-permanent`;
    const permanentPage = await openBlockedPage({
      context,
      serviceWorker,
      checks,
      blockedPageURL,
      attemptedURL: permanentURL,
    });
    await permanentPage.locator("#parent-toggle-button").click();
    await permanentPage.locator("#approval-action").selectOption("permanent");
    assert.equal(await permanentPage.locator("#duration-field").isHidden(), true);
    await permanentPage.locator("#parent-code").fill("parent-permanent-secret");
    assert.doesNotMatch(permanentPage.url(), /parent-permanent-secret/);
    await permanentPage.locator("#parent-submit-button").click();
    await waitFor(
      () => directApprovals.some((approval) => approval.url === permanentURL),
      "permanent parent approval was not sent",
    );
    const permanentApproval = directApprovals.find(
      (approval) => approval.url === permanentURL,
    );
    assert.equal(permanentApproval.action, "permanent");
    assert.equal(Object.hasOwn(permanentApproval, "duration_seconds"), false);
    await waitForAllowedPage(permanentPage, permanentURL);

    const rejectedURL = `${pageBaseURL}/blocked-rejected`;
    const rejectedPage = await openBlockedPage({
      context,
      serviceWorker,
      checks,
      blockedPageURL,
      attemptedURL: rejectedURL,
    });
    await rejectedPage.locator("#ask-parent-button").click();
    await waitFor(
      () => accessRequests.some((request) => request.url === rejectedURL),
      "rejected scenario did not create an access request",
    );
    const rejectedRequest = accessRequests.find((request) => request.url === rejectedURL);
    assert.ok(rejectedRequest);
    rejectedRequest.state = "rejected";
    await waitFor(
      async () => (await rejectedPage.locator("#access-status").textContent())
        ?.includes("did not approve"),
      "rejected request did not show the rejected state",
    );
    assert.equal(blockedPageMatches(rejectedPage, blockedPageURL, rejectedURL), true);

    await rejectedPage.locator("#parent-toggle-button").click();
    await rejectedPage.locator("#approval-action").selectOption("permanent");
    await rejectedPage.locator("#parent-code").fill("bad-code");
    const blockedURLBeforeFailedApproval = rejectedPage.url();
    assert.doesNotMatch(blockedURLBeforeFailedApproval, /bad-code/);
    await rejectedPage.locator("#parent-submit-button").click();
    await waitFor(
      async () => (await rejectedPage.locator("#parent-status").textContent())
        ?.includes("invalid parent code"),
      "invalid parent code did not show an error",
    );
    assert.equal(blockedPageMatches(rejectedPage, blockedPageURL, rejectedURL), true);
    assert.doesNotMatch(rejectedPage.url(), /bad-code/);
  } finally {
    if (context) {
      await context.close();
    }
    pageServer.closeAllConnections();
    approvalServer.closeAllConnections();
    kidControlServer.closeAllConnections();
    await Promise.all([
      close(pageServer),
      close(approvalServer),
      close(kidControlServer),
    ]);
  }
});
