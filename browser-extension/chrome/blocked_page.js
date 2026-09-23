export function buildBlockedPageURL(pageURL, attemptedURL, decision) {
  const url = new URL(pageURL);
  url.searchParams.set("url", attemptedURL);
  url.searchParams.set("profile", decision.profile ?? "");
  url.searchParams.set("reason", decision.reason ?? "");
  return url.toString();
}

export function parseBlockedPageURL(pageURL) {
  const url = new URL(pageURL);
  return {
    attemptedURL: url.searchParams.get("url") ?? "",
    profile: url.searchParams.get("profile") ?? "",
    reason: url.searchParams.get("reason") ?? "",
  };
}
