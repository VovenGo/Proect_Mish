package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vovengo/miha-shamanit/internal/config"
)

func testConfig() config.Config {
	return config.Config{
		PublicBaseURL:   "http://localhost:8080",
		RoundDuration:   60,
		MaxChatMessages: 50,
		RoomCodeLength:  6,
	}
}

func TestSecretPhraseVisibleOnlyToCurrentDrawer(t *testing.T) {
	app := New(testConfig(), nil)
	ctx := context.Background()

	owner, err := app.CreateRoom(ctx, CreateRoomInput{Name: "Drawer"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	guest, err := app.JoinRoom(ctx, JoinRoomInput{Code: owner.Room.Code, Name: "Guest"})
	if err != nil {
		t.Fatalf("join room: %v", err)
	}
	started, err := app.StartRound(ctx, StartRoundInput{Code: owner.Room.Code, PlayerID: owner.Player.ID})
	if err != nil {
		t.Fatalf("start round: %v", err)
	}
	if started.Round.PhraseForDrawer == "" {
		t.Fatal("drawer should receive secret phrase")
	}

	guestView, err := app.GetRoom(owner.Room.Code, guest.Player.ID)
	if err != nil {
		t.Fatalf("get room for guest: %v", err)
	}
	if guestView.Round.PhraseForDrawer != "" {
		t.Fatal("guest must not receive secret phrase via room api snapshot")
	}

	anonView, err := app.GetRoom(owner.Room.Code, "")
	if err != nil {
		t.Fatalf("get room for anon: %v", err)
	}
	if anonView.Round.PhraseForDrawer != "" {
		t.Fatal("anonymous viewer must not receive secret phrase")
	}
}

func TestSecretPhraseHiddenInSSESnapshotForNonDrawer(t *testing.T) {
	app := New(testConfig(), nil)
	ctx := context.Background()

	owner, _ := app.CreateRoom(ctx, CreateRoomInput{Name: "Drawer"})
	guest, _ := app.JoinRoom(ctx, JoinRoomInput{Code: owner.Room.Code, Name: "Guest"})
	if _, err := app.StartRound(ctx, StartRoundInput{Code: owner.Room.Code, PlayerID: owner.Player.ID}); err != nil {
		t.Fatalf("start round: %v", err)
	}

	guestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, cleanup, err := app.Subscribe(guestCtx, owner.Room.Code, guest.Player.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cleanup()

	select {
	case snap := <-ch:
		if snap.Room.Round.PhraseForDrawer != "" {
			t.Fatal("guest SSE snapshot leaked secret phrase")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}
}

func TestRoundCloseMessagesDoNotLeakSecretPhrase(t *testing.T) {
	app := New(testConfig(), nil)
	ctx := context.Background()

	owner, _ := app.CreateRoom(ctx, CreateRoomInput{Name: "Drawer"})
	guest, _ := app.JoinRoom(ctx, JoinRoomInput{Code: owner.Room.Code, Name: "Guest"})
	started, err := app.StartRound(ctx, StartRoundInput{Code: owner.Room.Code, PlayerID: owner.Player.ID})
	if err != nil {
		t.Fatalf("start round: %v", err)
	}
	phrase := started.Round.PhraseForDrawer
	if phrase == "" {
		t.Fatal("expected phrase for drawer")
	}

	if _, err := app.SendGuess(ctx, SendGuessInput{Code: owner.Room.Code, PlayerID: guest.Player.ID, Text: "моя догадка"}); err != nil {
		t.Fatalf("send guess: %v", err)
	}
	room, err := app.ConfirmGuess(ctx, ConfirmGuessInput{Code: owner.Room.Code, PlayerID: owner.Player.ID, WinnerID: guest.Player.ID})
	if err != nil {
		t.Fatalf("confirm guess: %v", err)
	}
	for _, msg := range room.Chat {
		if strings.Contains(msg.Text, phrase) {
			t.Fatalf("phrase leaked into chat/system message after confirm: %q", msg.Text)
		}
	}
}

func TestClearCanvasBroadcastsAndAllowsFreshDrawing(t *testing.T) {
	app := New(testConfig(), nil)
	ctx := context.Background()

	owner, _ := app.CreateRoom(ctx, CreateRoomInput{Name: "Drawer"})
	guest, _ := app.JoinRoom(ctx, JoinRoomInput{Code: owner.Room.Code, Name: "Guest"})
	if _, err := app.StartRound(ctx, StartRoundInput{Code: owner.Room.Code, PlayerID: owner.Player.ID}); err != nil {
		t.Fatalf("start round: %v", err)
	}
	if _, err := app.AddStroke(ctx, AddStrokeInput{Code: owner.Room.Code, PlayerID: owner.Player.ID, Color: "#ffffff", Width: 4, Points: []StrokePoint{{X: 0.1, Y: 0.2}, {X: 0.3, Y: 0.4}}}); err != nil {
		t.Fatalf("add stroke before clear: %v", err)
	}

	guestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, cleanup, err := app.Subscribe(guestCtx, owner.Room.Code, guest.Player.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cleanup()
	<-ch // initial snapshot

	cleared, err := app.ClearCanvas(ctx, ClearCanvasInput{Code: owner.Room.Code, PlayerID: owner.Player.ID})
	if err != nil {
		t.Fatalf("clear canvas: %v", err)
	}
	if len(cleared.Strokes) != 0 {
		t.Fatalf("expected cleared room to have no strokes, got %d", len(cleared.Strokes))
	}

	select {
	case snap := <-ch:
		if len(snap.Room.Strokes) != 0 {
			t.Fatalf("expected subscriber to observe cleared canvas, got %d strokes", len(snap.Room.Strokes))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clear broadcast")
	}

	redrawn, err := app.AddStroke(ctx, AddStrokeInput{Code: owner.Room.Code, PlayerID: owner.Player.ID, Color: "#00ffd0", Width: 6, Points: []StrokePoint{{X: 0.5, Y: 0.5}, {X: 0.7, Y: 0.8}}})
	if err != nil {
		t.Fatalf("add stroke after clear: %v", err)
	}
	if len(redrawn.Strokes) != 1 {
		t.Fatalf("expected fresh drawing after clear, got %d strokes", len(redrawn.Strokes))
	}
}
