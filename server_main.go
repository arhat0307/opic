//go:build server

package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed all:frontend/dist
var serverAssets embed.FS

func main() {
	service := NewAppService()
	mux := http.NewServeMux()
	registerAPI(mux, service)
	registerStatic(mux)

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("WAILS_SERVER_HOST"))
	}
	if host == "" {
		host = "127.0.0.1"
	}
	addr := host + ":" + port

	server := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("OPIc Flow server listening on http://%s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func registerAPI(mux *http.ServeMux, service *AppService) {
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, service.GetSettings())
	})
	mux.HandleFunc("POST /api/configure", func(w http.ResponseWriter, r *http.Request) {
		var request ConfigureRequest
		if !readJSON(w, r, &request) {
			return
		}
		result, err := service.Configure(request)
		writeResult(w, result, err)
	})
	mux.HandleFunc("POST /api/test-connection", func(w http.ResponseWriter, r *http.Request) {
		writeResult(w, map[string]bool{"ok": true}, service.TestConnection())
	})
	mux.HandleFunc("POST /api/speech", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Text string `json:"text"`
		}
		if !readJSON(w, r, &request) {
			return
		}
		result, err := service.GenerateSpeech(request.Text)
		writeResult(w, result, err)
	})
	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		var request ExamConfig
		if !readJSON(w, r, &request) {
			return
		}
		result, err := service.StartSession(request)
		writeResult(w, result, err)
	})
	mux.HandleFunc("POST /api/answers", func(w http.ResponseWriter, r *http.Request) {
		var request SubmitAnswerRequest
		if !readJSON(w, r, &request) {
			return
		}
		result, err := service.SubmitAnswer(request)
		writeResult(w, result, err)
	})
	mux.HandleFunc("POST /api/sessions/{sessionID}/finalize", func(w http.ResponseWriter, r *http.Request) {
		result, err := service.FinalizeSession(r.PathValue("sessionID"))
		writeResult(w, result, err)
	})
	mux.HandleFunc("GET /api/sessions/{sessionID}/report", func(w http.ResponseWriter, r *http.Request) {
		result, err := service.GetReport(r.PathValue("sessionID"))
		writeResult(w, result, err)
	})
}

func registerStatic(mux *http.ServeMux) {
	dist, err := fs.Sub(serverAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("frontend assets are missing: %v", err)
	}
	fileServer := http.FileServer(http.FS(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if _, err := fs.Stat(dist, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				r.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, value)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, strconv.FormatInt(time.Since(start).Milliseconds(), 10)+"ms")
	})
}
