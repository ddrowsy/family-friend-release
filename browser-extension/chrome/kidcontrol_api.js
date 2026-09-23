const KIDCONTROL_BASE_URL = "http://127.0.0.1:17653";

async function readError(response) {
  const text = await response.text();
  return text.trim() || `HTTP ${response.status}`;
}

export async function checkURL(url) {
  const response = await fetch(`${KIDCONTROL_BASE_URL}/api/browser/check`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ url }),
  });

  if (!response.ok) {
    throw new Error(`KidControl check failed: ${await readError(response)}`);
  }

  return response.json();
}

export async function endVisit() {
  const response = await fetch(`${KIDCONTROL_BASE_URL}/api/browser/end-visit`, {
    method: "POST",
  });

  if (response.status !== 204) {
    throw new Error(`KidControl end-visit failed: ${await readError(response)}`);
  }
}
