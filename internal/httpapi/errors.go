package httpapi

import "net/http"

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST")
	write(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
}
