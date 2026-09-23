const AGENT_BASE_URL = "http://127.0.0.1:17654";

async function readError(response) {
  const text = await response.text();
  if (!text.trim()) {
    return `HTTP ${response.status}`;
  }

  try {
    const body = JSON.parse(text);
    return body.error || text.trim();
  } catch {
    return text.trim();
  }
}

async function readJSON(response, operation) {
  if (!response.ok) {
    throw new Error(`${operation}: ${await readError(response)}`);
  }
  return response.json();
}

export function createApprovalAPI(fetchImpl = globalThis.fetch) {
  return {
    async createRequest({ url, browserProfile }, signal) {
      const response = await fetchImpl(`${AGENT_BASE_URL}/api/browser/access-requests`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          url,
          browser_profile: browserProfile,
        }),
        signal,
      });
      return readJSON(response, "Request access failed");
    },

    async requestStatus(requestID, signal) {
      const encodedID = encodeURIComponent(requestID);
      const response = await fetchImpl(
        `${AGENT_BASE_URL}/api/browser/access-requests/${encodedID}`,
        { signal },
      );
      return readJSON(response, "Checking request failed");
    },

    async approveDirect(
      { url, browserProfile, approvalCode, action, durationSeconds },
      signal,
    ) {
      const body = {
        url,
        browser_profile: browserProfile,
        approval_code: approvalCode,
        action,
      };
      if (action === "temporary") {
        body.duration_seconds = durationSeconds;
      }

      const response = await fetchImpl(`${AGENT_BASE_URL}/api/browser/approval`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
        signal,
      });
      return readJSON(response, "Parent approval failed");
    },
  };
}
