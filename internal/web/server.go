package web

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"

	"golang.org/x/crypto/acme/autocert"

	"github.com/mr-exz/cibot/internal/linear"
	"github.com/mr-exz/cibot/internal/storage"
)

type Server struct {
	db      *storage.DB
	linear  *linear.Client
	port    string
	domain  string
	certDir string
	mux     *http.ServeMux
}

func New(db *storage.DB, linearClient *linear.Client, port, domain, certDir string) *Server {
	s := &Server{
		db:      db,
		linear:  linearClient,
		port:    port,
		domain:  domain,
		certDir: certDir,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /ticket", s.handleTicketForm)
	s.mux.HandleFunc("POST /ticket", s.handleTicketSubmit)
	s.mux.HandleFunc("GET /api/topics", s.handleAPITopics)
	s.mux.HandleFunc("GET /api/categories", s.handleAPICategories)
	s.mux.HandleFunc("GET /api/types", s.handleAPITypes)
}

func (s *Server) Start(ctx context.Context) {
	if s.domain != "" {
		s.startTLS(ctx)
	} else {
		s.startPlain(ctx)
	}
}

func (s *Server) startPlain(ctx context.Context) {
	srv := &http.Server{
		Addr:    ":" + s.port,
		Handler: s.mux,
	}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()
	log.Printf("✓ Web server listening on :%s", s.port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("❌ Web server error: %v", err)
	}
}

func (s *Server) startTLS(ctx context.Context) {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(s.domain),
		Cache:      autocert.DirCache(s.certDir),
	}

	// HTTPS server on :443
	https := &http.Server{
		Addr:    ":443",
		Handler: s.mux,
		TLSConfig: &tls.Config{
			GetCertificate: m.GetCertificate,
		},
	}

	// HTTP server on :80 — only serves ACME challenges + redirects everything else to HTTPS
	http80 := &http.Server{
		Addr: ":80",
		Handler: m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + s.domain + r.RequestURI
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})),
	}

	go func() {
		<-ctx.Done()
		https.Shutdown(context.Background())  //nolint:errcheck
		http80.Shutdown(context.Background()) //nolint:errcheck
	}()

	go func() {
		log.Printf("✓ Web server (HTTP redirect) listening on :80")
		if err := http80.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ HTTP redirect server error: %v", err)
		}
	}()

	log.Printf("✓ Web server (HTTPS) listening on :443 for %s", s.domain)
	if err := https.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		log.Printf("❌ HTTPS server error: %v", err)
	}
}
