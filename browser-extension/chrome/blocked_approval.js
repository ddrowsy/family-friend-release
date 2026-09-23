const DEFAULT_POLL_INTERVAL_MS = 2000;

export function createBlockedApprovalController({
  context,
  api,
  render,
  navigate,
  setTimeoutImpl = globalThis.setTimeout,
  clearTimeoutImpl = globalThis.clearTimeout,
  pollIntervalMs = DEFAULT_POLL_INTERVAL_MS,
}) {
  let stopped = false;
  let submitting = false;
  let pendingRequestID = "";
  let pollTimer;

  function show(status, details = {}) {
    render({
      status,
      submitting,
      pendingRequestID,
      ...details,
    });
  }

  function clearPoll() {
    if (pollTimer !== undefined) {
      clearTimeoutImpl(pollTimer);
      pollTimer = undefined;
    }
  }

  function retryOriginalURL() {
    if (!stopped) {
      navigate(context.attemptedURL);
    }
  }

  function handleRequestState(result) {
    switch (result.state) {
      case "pending":
        show("waiting");
        schedulePoll();
        return;
      case "approved":
        pendingRequestID = "";
        show("approved");
        retryOriginalURL();
        return;
      case "rejected":
      case "cancelled":
        pendingRequestID = "";
        show("rejected");
        return;
      case "expired":
        pendingRequestID = "";
        show("expired");
        return;
      default:
        show("error", { message: "Unknown approval status." });
    }
  }

  async function poll() {
    pollTimer = undefined;
    if (stopped || !pendingRequestID) {
      return;
    }

    try {
      const result = await api.requestStatus(pendingRequestID);
      if (!stopped) {
        handleRequestState(result);
      }
    } catch (error) {
      if (!stopped) {
        show("error", {
          message: error.message,
          canResume: true,
        });
      }
    }
  }

  function schedulePoll() {
    clearPoll();
    if (stopped || !pendingRequestID) {
      return;
    }
    pollTimer = setTimeoutImpl(poll, pollIntervalMs);
  }

  async function askParent() {
    if (stopped || submitting) {
      return;
    }
    if (pendingRequestID) {
      show("waiting");
      schedulePoll();
      return;
    }

    submitting = true;
    show("requesting");
    try {
      const result = await api.createRequest({
        url: context.attemptedURL,
        browserProfile: context.profile,
      });
      pendingRequestID = result.request_id || "";
      if (!pendingRequestID) {
        throw new Error("Request access failed: missing request ID");
      }
      submitting = false;
      handleRequestState(result);
    } catch (error) {
      pendingRequestID = "";
      submitting = false;
      show("error", { message: error.message });
    }
  }

  function resumePolling() {
    if (stopped || !pendingRequestID) {
      return;
    }
    show("waiting");
    schedulePoll();
  }

  async function approveParent({ approvalCode, action, durationSeconds }) {
    if (stopped || submitting) {
      return false;
    }

    const code = approvalCode.trim();
    if (!code) {
      show("parent-error", { message: "Enter the parent code." });
      return false;
    }
    if (action !== "temporary" && action !== "permanent") {
      show("parent-error", { message: "Choose temporary or permanent access." });
      return false;
    }
    if (action === "temporary" && (!Number.isInteger(durationSeconds) || durationSeconds <= 0)) {
      show("parent-error", { message: "Choose a valid temporary duration." });
      return false;
    }

    submitting = true;
    show("parent-submitting");
    try {
      await api.approveDirect({
        url: context.attemptedURL,
        browserProfile: context.profile,
        approvalCode: code,
        action,
        durationSeconds,
      });
      submitting = false;
      show("approved");
      retryOriginalURL();
      return true;
    } catch (error) {
      submitting = false;
      show("parent-error", { message: error.message });
      return false;
    }
  }

  function stop() {
    stopped = true;
    clearPoll();
  }

  show("idle");

  return {
    askParent,
    approveParent,
    resumePolling,
    stop,
  };
}
