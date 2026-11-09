package main

import (
	"net/http"
	"strings"
	"time"
)

// tokenLifetime is how long a device's sync token stays valid.
const tokenLifetime = 30 * 24 * time.Hour

// requireToken rejects requests without a current device token.
func requireToken(store *Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		device, err := store.Device(token)
		if err != nil || time.Since(device.Issued) > tokenLifetime {
			http.Error(w, "sign in again", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
