import { createApprovalAPI } from "./approval_api.js";
import { createBlockedApprovalController } from "./blocked_approval.js";
import { parseBlockedPageURL } from "./blocked_page.js";

const context = parseBlockedPageURL(window.location.href);
const approvalAPI = createApprovalAPI();

const blockedURL = document.getElementById("blocked-url");
const blockedProfile = document.getElementById("blocked-profile");
const blockedReason = document.getElementById("blocked-reason");
const blockedReasonLabel = document.getElementById("blocked-reason-label");
const askParentButton = document.getElementById("ask-parent-button");
const accessStatus = document.getElementById("access-status");
const resumeStatusButton = document.getElementById("resume-status-button");
const parentToggleButton = document.getElementById("parent-toggle-button");
const parentPanel = document.getElementById("parent-panel");
const parentForm = document.getElementById("parent-form");
const parentCode = document.getElementById("parent-code");
const approvalAction = document.getElementById("approval-action");
const durationField = document.getElementById("duration-field");
const approvalDuration = document.getElementById("approval-duration");
const parentSubmitButton = document.getElementById("parent-submit-button");
const parentStatus = document.getElementById("parent-status");

blockedURL.textContent = context.attemptedURL || "Unknown site";
blockedProfile.textContent = context.profile || "Unknown profile";
blockedReason.textContent = context.reason;
if (!context.reason) {
  blockedReason.hidden = true;
  blockedReasonLabel.hidden = true;
}

function remoteStatusMessage(state) {
  switch (state.status) {
    case "requesting":
      return "Sending request to parent…";
    case "waiting":
      return "Waiting for parent approval…";
    case "approved":
      return "Access approved. Checking the page again…";
    case "rejected":
      return "Parent did not approve this request. You can ask again.";
    case "expired":
      return "Request expired. You can ask again.";
    case "error":
      return state.message || "Could not reach the approval service.";
    default:
      return "";
  }
}

function render(state) {
  accessStatus.textContent = remoteStatusMessage(state);
  accessStatus.classList.toggle("error", state.status === "error");

  const hasPendingRequest = Boolean(state.pendingRequestID);
  askParentButton.disabled = state.submitting || hasPendingRequest;
  resumeStatusButton.hidden = !state.canResume;
  parentSubmitButton.disabled = state.submitting;

  switch (state.status) {
    case "parent-submitting":
      parentStatus.textContent = "Checking parent code…";
      parentStatus.classList.remove("error");
      break;
    case "parent-error":
      parentStatus.textContent = state.message || "Parent approval failed.";
      parentStatus.classList.add("error");
      break;
    case "approved":
      parentStatus.textContent = "Access approved. Checking the page again…";
      parentStatus.classList.remove("error");
      break;
    default:
      if (!state.status.startsWith("parent-")) {
        parentStatus.textContent = "";
        parentStatus.classList.remove("error");
      }
  }
}

const controller = createBlockedApprovalController({
  context,
  api: approvalAPI,
  render,
  navigate(url) {
    window.location.replace(url);
  },
});

function updateDurationVisibility() {
  durationField.hidden = approvalAction.value !== "temporary";
}

askParentButton.addEventListener("click", () => {
  void controller.askParent();
});

resumeStatusButton.addEventListener("click", () => {
  controller.resumePolling();
});

parentToggleButton.addEventListener("click", () => {
  parentPanel.hidden = !parentPanel.hidden;
  if (!parentPanel.hidden) {
    parentCode.focus();
  }
});

approvalAction.addEventListener("change", updateDurationVisibility);
updateDurationVisibility();

parentForm.addEventListener("submit", (event) => {
  event.preventDefault();

  const approvalCode = parentCode.value;
  parentCode.value = "";
  const action = approvalAction.value;
  const durationSeconds = action === "temporary"
    ? Number.parseInt(approvalDuration.value, 10)
    : undefined;

  void controller.approveParent({
    approvalCode,
    action,
    durationSeconds,
  });
});

window.addEventListener("pagehide", () => {
  parentCode.value = "";
  controller.stop();
});
