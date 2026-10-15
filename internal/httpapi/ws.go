package httpapi

import "net/http"

// getWS authenticates the request and hands it to the hub. Without a valid
// login session it answers 401 before any upgrade.
func (a *api) getWS(w http.ResponseWriter, r *http.Request) {
	acc, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	a.d.Hub.Serve(w, r, acc, sess)
}
