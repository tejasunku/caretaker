package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
)

// Server serves the /mutate admission endpoint over HTTPS.
type Server struct {
	Addr     string
	CertFile string
	KeyFile  string
	Mutator  *Mutator
	Logger   *slog.Logger
}

// Run starts the HTTPS server and shuts it down when ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", s.handleMutate)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServeTLS(s.CertFile, s.KeyFile)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleMutate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "read request", http.StatusBadRequest)
		return
	}
	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		http.Error(w, "decode AdmissionReview", http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "missing admission request", http.StatusBadRequest)
		return
	}

	response := &admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	var deployment appsv1.Deployment
	if err := json.Unmarshal(review.Request.Object.Raw, &deployment); err != nil {
		s.logger().Error("decode deployment", "error", err)
	} else {
		ops, err := s.Mutator.Mutate(r.Context(), &deployment)
		switch {
		case err != nil:
			// Never block a rollout because the preference map or registry is
			// unavailable; failurePolicy should also be Ignore.
			s.logger().Error("mutate deployment", "name", deployment.Name, "namespace", deployment.Namespace, "error", err)
		case len(ops) > 0:
			patch, err := json.Marshal(ops)
			if err != nil {
				s.logger().Error("encode patch", "error", err)
				break
			}
			patchType := admissionv1.PatchTypeJSONPatch
			response.PatchType = &patchType
			response.Patch = patch
			s.logger().Info("mutated deployment", "name", deployment.Name, "namespace", deployment.Namespace, "operations", len(ops))
		}
	}

	review.Response = response
	review.Request = nil
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(review); err != nil {
		s.logger().Error("write AdmissionReview", "error", err)
	}
}

func (s *Server) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
