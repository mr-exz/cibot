package web

import (
	"context"
	"log"
	"net/http"

	"github.com/mr-exz/cibot/internal/linear"
	"github.com/mr-exz/cibot/internal/storage"
)

type Server struct {
	db      *storage.DB
	linear  *linear.Client
	port    string
	mux     *http.ServeMux
}

func New(db *storage.DB, linearClient *linear.Client, port string) *Server {
	s := &Server{
		db:     db,
		linear: linearClient,
		port:   port,
		mux:    http.NewServeMux(),
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
