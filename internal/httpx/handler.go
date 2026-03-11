package httpx

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/vovengo/miha-shamanit/internal/config"
	"github.com/vovengo/miha-shamanit/internal/service"
)

type Handler struct {
	cfg  config.Config
	app  *service.App
	tmpl *template.Template
}

func NewHandler(cfg config.Config, app *service.App) (http.Handler, error) {
	tmpl, err := template.ParseGlob("web/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	h := &Handler{cfg: cfg, app: app, tmpl: tmpl}
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.index)
	mux.HandleFunc("/api/render", h.render)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(cfg.StorageDir))))
	return mux, nil
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := map[string]any{
		"AppName":   h.cfg.AppName,
		"Domain":    h.cfg.Domain,
		"Provider":  h.cfg.GeneratorProvider,
		"PublicURL": h.cfg.PublicBaseURL,
	}
	if err := h.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes)
	if err := r.ParseMultipartForm(h.cfg.MaxUploadBytes); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	refBytes, refName, err := optionalFile(r, "reference")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.app.CreateImage(r.Context(), service.CreateImageInput{
		Prompt:        strings.TrimSpace(r.FormValue("prompt")),
		SketchDataURL: r.FormValue("sketchDataUrl"),
		Reference:     refBytes,
		ReferenceName: refName,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func optionalFile(r *http.Request, key string) ([]byte, string, error) {
	file, header, err := r.FormFile(key)
	if err != nil {
		if err == http.ErrMissingFile {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("read file %s: %w", key, err)
	}
	defer file.Close()
	return readMultipart(file, header)
}

func readMultipart(file multipart.File, header *multipart.FileHeader) ([]byte, string, error) {
	name := filepath.Base(header.Filename)
	if name == "." || name == string(os.PathSeparator) {
		name = "upload.bin"
	}
	b, err := io.ReadAll(file)
	if err != nil {
		return nil, "", fmt.Errorf("read upload: %w", err)
	}
	return b, name, nil
}
