package httpx

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

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
	mux.HandleFunc("/room/", h.roomPage)
	mux.HandleFunc("/api/rooms", h.createRoom)
	mux.HandleFunc("/api/rooms/join", h.joinRoom)
	mux.HandleFunc("/api/rooms/", h.roomAPI)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	return mux, nil
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	h.renderPage(w, map[string]any{"AppName": h.cfg.AppName, "Domain": h.cfg.Domain, "PublicURL": h.cfg.PublicBaseURL, "RoomCode": ""})
}

func (h *Handler) roomPage(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/room/")
	if code == "" {
		http.NotFound(w, r)
		return
	}
	h.renderPage(w, map[string]any{"AppName": h.cfg.AppName, "Domain": h.cfg.Domain, "PublicURL": h.cfg.PublicBaseURL, "RoomCode": strings.ToUpper(code)})
}

func (h *Handler) renderPage(w http.ResponseWriter, data map[string]any) {
	if err := h.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.method(w)
		return
	}
	var in service.CreateRoomInput
	if !h.decodeJSON(w, r, &in) {
		return
	}
	out, err := h.app.CreateRoom(r.Context(), in)
	if err != nil {
		h.badRequest(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, out)
}

func (h *Handler) joinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.method(w)
		return
	}
	var in service.JoinRoomInput
	if !h.decodeJSON(w, r, &in) {
		return
	}
	out, err := h.app.JoinRoom(r.Context(), in)
	if err != nil {
		h.badRequest(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, out)
}

func (h *Handler) roomAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	code := strings.ToUpper(parts[0])
	if len(parts) == 1 && r.Method == http.MethodGet {
		room, err := h.app.GetRoom(code, r.URL.Query().Get("playerId"))
		if err != nil {
			h.badRequest(w, err)
			return
		}
		h.writeJSON(w, http.StatusOK, room)
		return
	}
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "events":
		h.events(w, r, code)
	case "start":
		var in service.StartRoundInput
		if !h.decodeJSON(w, r, &in) {
			return
		}
		in.Code = code
		room, err := h.app.StartRound(r.Context(), in)
		if err != nil {
			h.badRequest(w, err)
			return
		}
		h.writeJSON(w, http.StatusOK, room)
	case "guess":
		var in service.SendGuessInput
		if !h.decodeJSON(w, r, &in) {
			return
		}
		in.Code = code
		room, err := h.app.SendGuess(r.Context(), in)
		if err != nil {
			h.badRequest(w, err)
			return
		}
		h.writeJSON(w, http.StatusOK, room)
	case "confirm":
		var in service.ConfirmGuessInput
		if !h.decodeJSON(w, r, &in) {
			return
		}
		in.Code = code
		room, err := h.app.ConfirmGuess(r.Context(), in)
		if err != nil {
			h.badRequest(w, err)
			return
		}
		h.writeJSON(w, http.StatusOK, room)
	case "stroke":
		var in service.AddStrokeInput
		if !h.decodeJSON(w, r, &in) {
			return
		}
		in.Code = code
		room, err := h.app.AddStroke(r.Context(), in)
		if err != nil {
			h.badRequest(w, err)
			return
		}
		h.writeJSON(w, http.StatusOK, room)
	case "clear":
		var in service.ClearCanvasInput
		if !h.decodeJSON(w, r, &in) {
			return
		}
		in.Code = code
		room, err := h.app.ClearCanvas(r.Context(), in)
		if err != nil {
			h.badRequest(w, err)
			return
		}
		h.writeJSON(w, http.StatusOK, room)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request, code string) {
	if r.Method != http.MethodGet {
		h.method(w)
		return
	}
	playerID := r.URL.Query().Get("playerId")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, _, err := h.app.Subscribe(r.Context(), code, playerID)
	if err != nil {
		h.badRequest(w, err)
		return
	}
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case snapshot, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(snapshot.Room)
			fmt.Fprintf(w, "event: room\ndata: %s\n\n", b)
			flusher.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (h *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Method != http.MethodPost {
		h.method(w)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxBodyBytes)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}
func (h *Handler) method(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
