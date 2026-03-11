package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vovengo/miha-shamanit/internal/config"
)

type App struct {
	cfg     config.Config
	mu      sync.RWMutex
	rooms   map[string]*Room
	phrases []string
}

type Room struct {
	Code             string        `json:"code"`
	ShareURL         string        `json:"shareUrl"`
	Players          []*Player     `json:"players"`
	Chat             []ChatMessage `json:"chat"`
	Strokes          []Stroke      `json:"strokes"`
	Round            RoundState    `json:"round"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
	LastWinner       string        `json:"lastWinner,omitempty"`
	LastWinningGuess string        `json:"lastWinningGuess,omitempty"`
	Version          int64         `json:"version"`
	watchers         map[chan RoomSnapshot]struct{}
}

type Player struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Role       string    `json:"role"`
	Connected  bool      `json:"connected"`
	JoinedAt   time.Time `json:"joinedAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type ChatMessage struct {
	ID        string    `json:"id"`
	PlayerID  string    `json:"playerId"`
	Player    string    `json:"player"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type Stroke struct {
	ID        string        `json:"id"`
	PlayerID  string        `json:"playerId"`
	Color     string        `json:"color"`
	Width     float64       `json:"width"`
	Points    []StrokePoint `json:"points"`
	CreatedAt time.Time     `json:"createdAt"`
}

type StrokePoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type RoundState struct {
	Status          string    `json:"status"`
	Number          int       `json:"number"`
	DrawerID        string    `json:"drawerId,omitempty"`
	DrawerName      string    `json:"drawerName,omitempty"`
	PhraseMasked    string    `json:"phraseMasked"`
	PhraseForDrawer string    `json:"phraseForDrawer,omitempty"`
	StartedAt       time.Time `json:"startedAt,omitempty"`
	EndsAt          time.Time `json:"endsAt,omitempty"`
	WinnerID        string    `json:"winnerId,omitempty"`
	WinnerName      string    `json:"winnerName,omitempty"`
	WinningGuess    string    `json:"winningGuess,omitempty"`
	LastConfirmedAt time.Time `json:"lastConfirmedAt,omitempty"`
}

type RoomSnapshot struct {
	Room     *Room  `json:"room"`
	ViewerID string `json:"viewerId"`
}

type CreateRoomInput struct {
	Name string `json:"name"`
}
type JoinRoomInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
type StartRoundInput struct {
	Code     string `json:"code"`
	PlayerID string `json:"playerId"`
}
type SendGuessInput struct {
	Code     string `json:"code"`
	PlayerID string `json:"playerId"`
	Text     string `json:"text"`
}
type ConfirmGuessInput struct {
	Code     string `json:"code"`
	PlayerID string `json:"playerId"`
	WinnerID string `json:"winnerId"`
}
type AddStrokeInput struct {
	Code     string        `json:"code"`
	PlayerID string        `json:"playerId"`
	Color    string        `json:"color"`
	Width    float64       `json:"width"`
	Points   []StrokePoint `json:"points"`
}

type RoomAccess struct {
	Room   *Room   `json:"room"`
	Player *Player `json:"player"`
}

func New(cfg config.Config, _ any) *App {
	return &App{cfg: cfg, rooms: make(map[string]*Room), phrases: absurdPhrases()}
}

func (a *App) CreateRoom(ctx context.Context, in CreateRoomInput) (RoomAccess, error) {
	_ = ctx
	name := cleanName(in.Name)
	if name == "" {
		return RoomAccess{}, errors.New("name is required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	code := a.uniqueCodeLocked()
	player := &Player{ID: newID(), Name: name, Role: "drawer", Connected: true, JoinedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC()}
	room := &Room{Code: code, ShareURL: strings.TrimRight(a.cfg.PublicBaseURL, "/") + "/room/" + code, Players: []*Player{player}, Chat: []ChatMessage{}, Strokes: []Stroke{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), watchers: map[chan RoomSnapshot]struct{}{}, Round: RoundState{Status: "waiting", PhraseMasked: "Ждём второго шамана, укуси меня пчела."}}
	room.Version = 1
	a.rooms[code] = room
	a.broadcastLocked(room)
	return RoomAccess{Room: cloneRoomForPlayer(room, player.ID), Player: clonePlayer(player)}, nil
}

func (a *App) JoinRoom(ctx context.Context, in JoinRoomInput) (RoomAccess, error) {
	_ = ctx
	name := cleanName(in.Name)
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if name == "" || code == "" {
		return RoomAccess{}, errors.New("room code and name are required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	room := a.rooms[code]
	if room == nil {
		return RoomAccess{}, errors.New("room not found")
	}
	player := &Player{ID: newID(), Name: name, Role: "guesser", Connected: true, JoinedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC()}
	room.Players = append(room.Players, player)
	room.UpdatedAt = time.Now().UTC()
	room.Version++
	a.addSystemMessageLocked(room, fmt.Sprintf("%s ввалился в избу по ссылке.", name))
	a.assignRolesLocked(room)
	a.broadcastLocked(room)
	return RoomAccess{Room: cloneRoomForPlayer(room, player.ID), Player: clonePlayer(player)}, nil
}

func (a *App) GetRoom(code, viewerID string) (*Room, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	room := a.rooms[strings.ToUpper(strings.TrimSpace(code))]
	if room == nil {
		return nil, errors.New("room not found")
	}
	return cloneRoomForPlayer(room, viewerID), nil
}

func (a *App) StartRound(ctx context.Context, in StartRoundInput) (*Room, error) {
	_ = ctx
	a.mu.Lock()
	defer a.mu.Unlock()
	room, player, err := a.roomAndPlayerLocked(in.Code, in.PlayerID)
	if err != nil {
		return nil, err
	}
	if len(room.Players) < 2 {
		return nil, errors.New("need at least 2 players")
	}
	if room.Round.Status == "active" {
		return nil, errors.New("round already active")
	}
	if player.Role != "drawer" {
		return nil, errors.New("only drawer can start the round")
	}
	phrase := a.phrases[mrand.Intn(len(a.phrases))]
	room.Strokes = nil
	room.Round = RoundState{Status: "active", Number: room.Round.Number + 1, DrawerID: player.ID, DrawerName: player.Name, PhraseMasked: maskPhrase(phrase), PhraseForDrawer: phrase, StartedAt: time.Now().UTC(), EndsAt: time.Now().UTC().Add(time.Duration(a.cfg.RoundDuration) * time.Second)}
	room.LastWinner = ""
	room.LastWinningGuess = ""
	room.UpdatedAt = time.Now().UTC()
	room.Version++
	a.addSystemMessageLocked(room, fmt.Sprintf("Раунд %d стартовал. %s шаманит, остальные угадывают.", room.Round.Number, player.Name))
	a.broadcastLocked(room)
	go a.watchRound(room.Code, room.Round.Number, room.Round.EndsAt)
	return cloneRoomForPlayer(room, in.PlayerID), nil
}

func (a *App) SendGuess(ctx context.Context, in SendGuessInput) (*Room, error) {
	_ = ctx
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, errors.New("message is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	room, player, err := a.roomAndPlayerLocked(in.Code, in.PlayerID)
	if err != nil {
		return nil, err
	}
	player.LastSeenAt = time.Now().UTC()
	if room.Round.Status != "active" {
		a.addChatLocked(room, player, text, "chat")
		a.broadcastLocked(room)
		return cloneRoomForPlayer(room, in.PlayerID), nil
	}
	kind := "guess"
	if player.ID == room.Round.DrawerID {
		kind = "chat"
	}
	a.addChatLocked(room, player, text, kind)
	a.broadcastLocked(room)
	return cloneRoomForPlayer(room, in.PlayerID), nil
}

func (a *App) ConfirmGuess(ctx context.Context, in ConfirmGuessInput) (*Room, error) {
	_ = ctx
	a.mu.Lock()
	defer a.mu.Unlock()
	room, player, err := a.roomAndPlayerLocked(in.Code, in.PlayerID)
	if err != nil {
		return nil, err
	}
	if room.Round.Status != "active" {
		return nil, errors.New("no active round")
	}
	if player.ID != room.Round.DrawerID {
		return nil, errors.New("only drawer can confirm")
	}
	winner := a.findPlayerLocked(room, in.WinnerID)
	if winner == nil {
		return nil, errors.New("winner not found")
	}
	if winner.ID == player.ID {
		return nil, errors.New("drawer cannot confirm self")
	}
	winningGuess := lastGuessByPlayer(room, winner.ID)
	room.Round.Status = "guessed"
	room.Round.WinnerID = winner.ID
	room.Round.WinnerName = winner.Name
	room.Round.WinningGuess = winningGuess
	room.Round.LastConfirmedAt = time.Now().UTC()
	room.LastWinner = winner.Name
	room.LastWinningGuess = winningGuess
	a.rotateDrawerLocked(room, winner.ID)
	room.UpdatedAt = time.Now().UTC()
	room.Version++
	a.addSystemMessageLocked(room, fmt.Sprintf("%s угадал. Фраза была: «%s».", winner.Name, room.Round.PhraseForDrawer))
	a.broadcastLocked(room)
	return cloneRoomForPlayer(room, in.PlayerID), nil
}

func (a *App) AddStroke(ctx context.Context, in AddStrokeInput) (*Room, error) {
	_ = ctx
	if len(in.Points) < 1 {
		return nil, errors.New("points are required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	room, player, err := a.roomAndPlayerLocked(in.Code, in.PlayerID)
	if err != nil {
		return nil, err
	}
	if room.Round.Status != "active" {
		return nil, errors.New("no active round")
	}
	if room.Round.DrawerID != player.ID {
		return nil, errors.New("only drawer can draw")
	}
	stroke := Stroke{ID: newID(), PlayerID: player.ID, Color: safeColor(in.Color), Width: safeWidth(in.Width), Points: limitPoints(in.Points), CreatedAt: time.Now().UTC()}
	room.Strokes = append(room.Strokes, stroke)
	room.UpdatedAt = time.Now().UTC()
	room.Version++
	a.broadcastLocked(room)
	return cloneRoomForPlayer(room, in.PlayerID), nil
}

func (a *App) Subscribe(ctx context.Context, code, viewerID string) (<-chan RoomSnapshot, func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	room := a.rooms[strings.ToUpper(strings.TrimSpace(code))]
	if room == nil {
		return nil, nil, errors.New("room not found")
	}
	ch := make(chan RoomSnapshot, 8)
	room.watchers[ch] = struct{}{}
	ch <- RoomSnapshot{Room: cloneRoomForPlayer(room, viewerID), ViewerID: viewerID}
	cancel := func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if live := a.rooms[room.Code]; live != nil {
			delete(live.watchers, ch)
		}
		close(ch)
	}
	go func() { <-ctx.Done(); cancel() }()
	return ch, cancel, nil
}

func (a *App) roomAndPlayerLocked(code, playerID string) (*Room, *Player, error) {
	room := a.rooms[strings.ToUpper(strings.TrimSpace(code))]
	if room == nil {
		return nil, nil, errors.New("room not found")
	}
	player := a.findPlayerLocked(room, playerID)
	if player == nil {
		return nil, nil, errors.New("player not found")
	}
	return room, player, nil
}

func (a *App) findPlayerLocked(room *Room, playerID string) *Player {
	for _, p := range room.Players {
		if p.ID == playerID {
			return p
		}
	}
	return nil
}

func (a *App) addChatLocked(room *Room, player *Player, text, kind string) {
	room.Chat = append(room.Chat, ChatMessage{ID: newID(), PlayerID: player.ID, Player: player.Name, Kind: kind, Text: text, CreatedAt: time.Now().UTC()})
	if len(room.Chat) > a.cfg.MaxChatMessages {
		room.Chat = room.Chat[len(room.Chat)-a.cfg.MaxChatMessages:]
	}
	room.UpdatedAt = time.Now().UTC()
	room.Version++
}

func (a *App) addSystemMessageLocked(room *Room, text string) {
	room.Chat = append(room.Chat, ChatMessage{ID: newID(), Player: "Шаманский матюгальник", Kind: "system", Text: text, CreatedAt: time.Now().UTC()})
	if len(room.Chat) > a.cfg.MaxChatMessages {
		room.Chat = room.Chat[len(room.Chat)-a.cfg.MaxChatMessages:]
	}
}

func (a *App) assignRolesLocked(room *Room) {
	for i, p := range room.Players {
		if room.Round.Status == "active" && p.ID == room.Round.DrawerID {
			p.Role = "drawer"
			continue
		}
		if i == 0 {
			p.Role = "drawer"
		} else {
			p.Role = "guesser"
		}
	}
}

func (a *App) rotateDrawerLocked(room *Room, preferredID string) {
	for _, p := range room.Players {
		if p.ID == preferredID {
			p.Role = "drawer"
		} else {
			p.Role = "guesser"
		}
	}
}

func (a *App) uniqueCodeLocked() string {
	for {
		code := strings.ToUpper(randomAlphaNum(a.cfg.RoomCodeLength))
		if _, exists := a.rooms[code]; !exists {
			return code
		}
	}
}

func (a *App) broadcastLocked(room *Room) {
	room.Version++
	snapshotPlayers := append([]*Player(nil), room.Players...)
	sort.Slice(snapshotPlayers, func(i, j int) bool { return snapshotPlayers[i].JoinedAt.Before(snapshotPlayers[j].JoinedAt) })
	for ch := range room.watchers {
		select {
		case ch <- RoomSnapshot{Room: cloneRoomForPlayer(room, ""), ViewerID: ""}:
		default:
		}
	}
}

func (a *App) watchRound(code string, roundNumber int, endsAt time.Time) {
	timer := time.NewTimer(time.Until(endsAt))
	defer timer.Stop()
	<-timer.C
	a.mu.Lock()
	defer a.mu.Unlock()
	room := a.rooms[code]
	if room == nil || room.Round.Number != roundNumber || room.Round.Status != "active" {
		return
	}
	room.Round.Status = "timeout"
	a.addSystemMessageLocked(room, fmt.Sprintf("Время вышло. Фраза была: «%s».", room.Round.PhraseForDrawer))
	room.UpdatedAt = time.Now().UTC()
	room.Version++
	a.broadcastLocked(room)
}

func cloneRoomForPlayer(room *Room, viewerID string) *Room {
	copyRoom := *room
	copyRoom.Chat = append([]ChatMessage(nil), room.Chat...)
	copyRoom.Strokes = append([]Stroke(nil), room.Strokes...)
	copyRoom.Players = make([]*Player, 0, len(room.Players))
	for _, p := range room.Players {
		clone := *p
		copyRoom.Players = append(copyRoom.Players, &clone)
	}
	if viewerID == "" || viewerID != room.Round.DrawerID {
		copyRoom.Round.PhraseForDrawer = ""
	}
	copyRoom.watchers = nil
	return &copyRoom
}

func clonePlayer(p *Player) *Player { cp := *p; return &cp }

func cleanName(v string) string {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > 24 {
		v = string([]rune(v)[:24])
	}
	return v
}

func maskPhrase(phrase string) string {
	words := strings.Fields(phrase)
	masked := make([]string, 0, len(words))
	for _, w := range words {
		r := []rune(w)
		for i := range r {
			if (r[i] >= 'А' && r[i] <= 'я') || (r[i] >= 'A' && r[i] <= 'z') || (r[i] >= '0' && r[i] <= '9') {
				r[i] = '•'
			}
		}
		masked = append(masked, string(r))
	}
	return strings.Join(masked, " ")
}

func lastGuessByPlayer(room *Room, playerID string) string {
	for i := len(room.Chat) - 1; i >= 0; i-- {
		if room.Chat[i].PlayerID == playerID && room.Chat[i].Kind == "guess" {
			return room.Chat[i].Text
		}
	}
	return ""
}

func safeColor(v string) string {
	v = strings.TrimSpace(v)
	if len(v) != 7 || !strings.HasPrefix(v, "#") {
		return "#00ffd0"
	}
	return v
}
func safeWidth(v float64) float64 {
	if v < 1 {
		return 1
	}
	if v > 24 {
		return 24
	}
	return v
}
func limitPoints(in []StrokePoint) []StrokePoint {
	if len(in) > 64 {
		in = in[:64]
	}
	out := make([]StrokePoint, 0, len(in))
	for _, p := range in {
		if p.X < 0 {
			p.X = 0
		}
		if p.X > 1 {
			p.X = 1
		}
		if p.Y < 0 {
			p.Y = 0
		}
		if p.Y > 1 {
			p.Y = 1
		}
		out = append(out, p)
	}
	return out
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func randomAlphaNum(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, n)
	seed := make([]byte, n)
	_, _ = rand.Read(seed)
	for i := 0; i < n; i++ {
		buf[i] = alphabet[int(seed[i])%len(alphabet)]
	}
	return string(buf)
}

func absurdPhrases() []string {
	return []string{
		"дед кибер-пасечник на дроновом тракторе",
		"неоновый гусь чинит антенну на бане",
		"самовар с вайфаем колдует над картошкой",
		"бабка-хакер в лаптях запускает спутник из сарая",
		"робо-коза с лазерным колокольчиком пасётся у реактора",
		"кибер-гармошка вызывает дождь из пиксельных огурцов",
		"трактор-шаман варит блокчейн в чугунке",
		"дрон-петух будит деревню сиреной и техно",
	}
}
