package agent

import "net/http"

// Handler returns the blocked-page approval bridge HTTP handler.
func (a *BrowserApprovalAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	a.register(mux)
	return mux
}
