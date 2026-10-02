// Package httpapi is the compile queue HTTP API.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/queue"
)

type Server struct {
	Q     *queue.Queue
	Token string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.authed(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /v1/jobs", s.authed(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Q.List())
	}))
	mux.HandleFunc("POST /v1/jobs", s.authed(s.create))
	mux.HandleFunc("GET /v1/jobs/{id}", s.authed(s.one))
	mux.HandleFunc("GET /v1/jobs/{id}/log", s.authed(s.log))
	mux.HandleFunc("GET /v1/jobs/{id}/artifacts", s.authed(s.artifacts))
	mux.HandleFunc("GET /v1/jobs/{id}/artifacts/{name}", s.authed(s.artifact))
	return mux
}

func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" || got != s.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type createReq struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Jobs int    `json:"jobs"`
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 30); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meta := r.FormValue("meta")
	var req createReq
	if err := json.Unmarshal([]byte(meta), &req); err != nil {
		http.Error(w, "bad meta", http.StatusBadRequest)
		return
	}
	switch req.Kind {
	case queue.KindAur:
		if req.Name == "" || strings.Contains(req.Name, "/") {
			http.Error(w, "bad package name", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusAccepted, s.Q.SubmitAur(req.Name))
	case queue.KindMake, queue.KindMakepkg:
		f, _, err := r.FormFile("tree")
		if err != nil {
			http.Error(w, "tree file required", http.StatusBadRequest)
			return
		}
		defer f.Close()
		body, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		job, err := s.Q.SubmitTree(req.Kind, req.Name, req.Jobs, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		http.Error(w, "unknown kind", http.StatusBadRequest)
	}
}

func (s *Server) one(w http.ResponseWriter, r *http.Request) {
	job, ok := s.Q.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Q.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(s.Q.LogPath(id))
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func (s *Server) artifacts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Q.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	paths, err := queue.Packages(s.Q.SrcDir(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var names []string
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	writeJSON(w, http.StatusOK, names)
}

func (s *Server) artifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")
	if _, ok := s.Q.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	if name != filepath.Base(name) || strings.Contains(name, "..") {
		http.Error(w, "bad name", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.Q.SrcDir(id), name))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
