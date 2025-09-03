package main

import (
	"encoding/json"
	"net/http"
)

type responseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.wroteHeader = true
	return rw.ResponseWriter.Write(b)
}

func writeJSONResponse(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	rw := &responseWriter{ResponseWriter: w}
	if err := json.NewEncoder(rw).Encode(v); err != nil {
		if !rw.wroteHeader {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return err
	}
	return nil
}
