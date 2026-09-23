export function isWebURL(rawURL) {
  try {
    const url = new URL(rawURL);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

export function createURLDecisionHandler({
  checkURL,
  blockNavigation,
  logger = console,
}) {
  return async function handleURLDecision(tabId, rawURL) {
    let decision;
    try {
      decision = await checkURL(rawURL);
    } catch (error) {
      logger.warn("KidControl browser check failed; allowing navigation", error);
      return false;
    }

    if (decision?.allowed === false) {
      try {
        await blockNavigation(tabId, rawURL, decision);
      } catch (error) {
        logger.warn("KidControl could not show blocked page", error);
      }
      return false;
    }

    if (decision?.allowed !== true) {
      logger.warn("KidControl returned an invalid browser decision; allowing navigation");
      return false;
    }

    return true;
  };
}

export function createNavigationHandler({
  getActiveTab,
  handleURLDecision,
  logger = console,
}) {
  const lastCheckedURLByTab = new Map();

  return async function handleNavigation(details) {
    if (details.frameId !== 0 || !isWebURL(details.url)) {
      return;
    }

    let activeTab;
    try {
      activeTab = await getActiveTab();
    } catch (error) {
      logger.warn("KidControl could not inspect active browser tab", error);
      return;
    }

    if (activeTab?.id !== details.tabId) {
      return;
    }

    if (lastCheckedURLByTab.get(details.tabId) === details.url) {
      return;
    }

    const allowed = await handleURLDecision(details.tabId, details.url);
    if (allowed) {
      lastCheckedURLByTab.set(details.tabId, details.url);
    }
  };
}
