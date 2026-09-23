import { isWebURL } from "./navigation.js";

export function createBrowserActivityHandler({
  endVisit,
  getActiveTabInWindow,
  getFrameURL,
  getLastFocusedWindow,
  isWindowFocused,
  handleURLDecision,
  windowIDNone,
  logger = console,
}) {
  let activeTabId = null;
  let activeWindowId = null;
  let queue = Promise.resolve();

  function enqueue(operation) {
    queue = queue.then(operation, operation);
    return queue;
  }

  async function finishCurrentVisit() {
    if (activeTabId === null) {
      return;
    }

    activeTabId = null;
    activeWindowId = null;

    try {
      await endVisit();
    } catch (error) {
      logger.warn("KidControl could not end browser visit", error);
    }
  }

  async function startTab(tabId, windowId) {
    activeTabId = tabId;
    activeWindowId = windowId;

    let rawURL;
    try {
      rawURL = await getFrameURL(tabId);
    } catch (error) {
      logger.warn("KidControl could not inspect active tab URL", error);
      return;
    }

    if (!isWebURL(rawURL)) {
      return;
    }

    await handleURLDecision(tabId, rawURL);
  }

  return {
    initialize() {
      return enqueue(async () => {
        let window;
        try {
          window = await getLastFocusedWindow();
        } catch (error) {
          logger.warn("KidControl could not inspect focused browser window", error);
          return;
        }

        if (!window?.focused || window.id === windowIDNone) {
          return;
        }

        let tab;
        try {
          tab = await getActiveTabInWindow(window.id);
        } catch (error) {
          logger.warn("KidControl could not inspect active browser tab", error);
          return;
        }

        if (tab?.id !== undefined) {
          await startTab(tab.id, window.id);
        }
      });
    },

    handleTabActivated({ tabId, windowId }) {
      return enqueue(async () => {
        let focused;
        try {
          focused = await isWindowFocused(windowId);
        } catch (error) {
          logger.warn("KidControl could not inspect browser window focus", error);
          return;
        }

        if (!focused) {
          return;
        }

        await finishCurrentVisit();
        await startTab(tabId, windowId);
      });
    },

    handleWindowFocusChanged(windowId) {
      return enqueue(async () => {
        await finishCurrentVisit();

        if (windowId === windowIDNone) {
          return;
        }

        let tab;
        try {
          tab = await getActiveTabInWindow(windowId);
        } catch (error) {
          logger.warn("KidControl could not inspect active browser tab", error);
          return;
        }

        if (tab?.id !== undefined) {
          await startTab(tab.id, windowId);
        }
      });
    },

    handleTabRemoved(tabId) {
      return enqueue(async () => {
        if (tabId === activeTabId) {
          await finishCurrentVisit();
        }
      });
    },

    getState() {
      return {
        activeTabId,
        activeWindowId,
      };
    },
  };
}
