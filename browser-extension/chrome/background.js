import { createBrowserActivityHandler } from "./activity.js";
import { buildBlockedPageURL } from "./blocked_page.js";
import { checkURL, endVisit } from "./kidcontrol_api.js";
import {
  createNavigationHandler,
  createURLDecisionHandler,
} from "./navigation.js";

const blockNavigation = (tabId, attemptedURL, decision) => {
  const blockedURL = buildBlockedPageURL(
    chrome.runtime.getURL("blocked.html"),
    attemptedURL,
    decision,
  );
  return chrome.tabs.update(tabId, { url: blockedURL });
};

const handleURLDecision = createURLDecisionHandler({
  checkURL,
  blockNavigation,
  logger: console,
});

const getActiveTabInWindow = async (windowId) => {
  const [tab] = await chrome.tabs.query({
    active: true,
    windowId,
  });
  return tab;
};

const getActiveTab = async () => {
  const [tab] = await chrome.tabs.query({
    active: true,
    lastFocusedWindow: true,
  });
  return tab;
};

const handleNavigation = createNavigationHandler({
  getActiveTab,
  handleURLDecision,
  logger: console,
});

const activity = createBrowserActivityHandler({
  endVisit,
  getActiveTabInWindow,
  getFrameURL: async (tabId) => {
    const frame = await chrome.webNavigation.getFrame({
      tabId,
      frameId: 0,
    });
    return frame?.url;
  },
  getLastFocusedWindow: () => chrome.windows.getLastFocused(),
  isWindowFocused: async (windowId) => {
    const window = await chrome.windows.get(windowId);
    return window.focused;
  },
  handleURLDecision,
  windowIDNone: chrome.windows.WINDOW_ID_NONE,
  logger: console,
});

chrome.webNavigation.onBeforeNavigate.addListener((details) => {
  void handleNavigation(details);
});

chrome.tabs.onActivated.addListener((activeInfo) => {
  void activity.handleTabActivated(activeInfo);
});

chrome.tabs.onRemoved.addListener((tabId) => {
  void activity.handleTabRemoved(tabId);
});

chrome.windows.onFocusChanged.addListener((windowId) => {
  void activity.handleWindowFocusChanged(windowId);
});

void activity.initialize();
