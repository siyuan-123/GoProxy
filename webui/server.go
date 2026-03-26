package webui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"proxy-pool/config"
	"proxy-pool/logger"
	"proxy-pool/storage"
)

// 简单内存 session
var (
	sessions   = make(map[string]time.Time)
	sessionsMu sync.Mutex
)

func newSession() string {
	token := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano()))))
	sessionsMu.Lock()
	sessions[token] = time.Now().Add(24 * time.Hour)
	sessionsMu.Unlock()
	return token
}

func validSession(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}
	sessionsMu.Lock()
	expiry, ok := sessions[cookie.Value]
	sessionsMu.Unlock()
	return ok && time.Now().Before(expiry)
}

type FetchTrigger func()

type Server struct {
	storage       *storage.Storage
	cfg           *config.Config
	fetchTrigger  FetchTrigger
	configChanged chan<- struct{}
}

func New(s *storage.Storage, cfg *config.Config, ft FetchTrigger, cc chan<- struct{}) *Server {
	return &Server{storage: s, cfg: cfg, fetchTrigger: ft, configChanged: cc}
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/api/stats", s.authMiddleware(s.apiStats))
	mux.HandleFunc("/api/proxies", s.authMiddleware(s.apiProxies))
	mux.HandleFunc("/api/proxy/delete", s.authMiddleware(s.apiDeleteProxy))
	mux.HandleFunc("/api/fetch", s.authMiddleware(s.apiFetch))
	mux.HandleFunc("/api/logs", s.authMiddleware(s.apiLogs))
	mux.HandleFunc("/api/config", s.authMiddleware(s.apiConfig))

	log.Printf("WebUI listening on %s", s.cfg.WebUIPort)
	go func() {
		if err := http.ListenAndServe(s.cfg.WebUIPort, mux); err != nil {
			log.Fatalf("webui: %v", err)
		}
	}()
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r) {
			if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !validSession(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, loginHTML)
		return
	}
	password := r.FormValue("password")
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))
	if hash != s.cfg.WebUIPasswordHash {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, loginHTMLWithError)
		return
	}
	token := newSession()
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		sessionsMu.Lock()
		delete(sessions, cookie.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	total, _ := s.storage.Count()
	httpCount, _ := s.storage.CountByProtocol("http")
	socks5Count, _ := s.storage.CountByProtocol("socks5")
	jsonOK(w, map[string]interface{}{
		"total":  total,
		"http":   httpCount,
		"socks5": socks5Count,
		"port":   s.cfg.ProxyPort,
	})
}

func (s *Server) apiProxies(w http.ResponseWriter, r *http.Request) {
	protocol := r.URL.Query().Get("protocol")
	var proxies []storage.Proxy
	var err error
	if protocol != "" {
		proxies, err = s.storage.GetByProtocol(protocol)
	} else {
		proxies, err = s.storage.GetAll()
	}
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, proxies)
}

func (s *Server) apiDeleteProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	s.storage.Delete(req.Address)
	jsonOK(w, map[string]string{"status": "deleted"})
}

func (s *Server) apiFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go s.fetchTrigger()
	jsonOK(w, map[string]string{"status": "fetch started"})
}

func (s *Server) apiLogs(w http.ResponseWriter, r *http.Request) {
	lines := logger.GetLines(200)
	jsonOK(w, map[string]interface{}{"lines": lines})
}

func (s *Server) apiConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if r.Method == http.MethodGet {
		jsonOK(w, map[string]interface{}{
			"fetch_interval":        cfg.FetchInterval,
			"check_interval":        cfg.CheckInterval,
			"validate_concurrency":  cfg.ValidateConcurrency,
			"validate_timeout":      cfg.ValidateTimeout,
		})
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		FetchInterval       int `json:"fetch_interval"`
		CheckInterval       int `json:"check_interval"`
		ValidateConcurrency int `json:"validate_concurrency"`
		ValidateTimeout     int `json:"validate_timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.FetchInterval <= 0 || req.CheckInterval <= 0 || req.ValidateConcurrency <= 0 || req.ValidateTimeout <= 0 {
		jsonError(w, "all values must be positive", http.StatusBadRequest)
		return
	}
	if err := config.Save(req.FetchInterval, req.CheckInterval, req.ValidateConcurrency, req.ValidateTimeout); err != nil {
		jsonError(w, "save config error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 通知定时器重置
	select {
	case s.configChanged <- struct{}{}:
	default:
	}
	log.Printf("[config] updated: fetch=%dm check=%dm concurrency=%d timeout=%ds",
		req.FetchInterval, req.CheckInterval, req.ValidateConcurrency, req.ValidateTimeout)
	jsonOK(w, map[string]string{"status": "saved"})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
