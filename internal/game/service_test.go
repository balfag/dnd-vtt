package game

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/store"
)

type fixture struct {
	game  *Service
	auth  *auth.Service
	db    *sql.DB
	clock time.Time
	gm    auth.Account
}

// newFixture opens a migrated database, creates the GM account and returns
// both services on one fake clock.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	f := &fixture{db: st.DB(), clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	now := func() time.Time { return f.clock }
	f.auth, err = auth.NewService(f.db, auth.Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	f.game = NewService(f.db, Options{Now: now})

	setup, err := f.auth.IssueSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	f.gm, err = f.auth.CreateGM(ctx, setup, "master", "master-password")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) campaign(t *testing.T, name string) Campaign {
	t.Helper()
	c, err := f.game.CreateCampaign(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fixture) start(t *testing.T, campaignID string) (GameSession, string) {
	t.Helper()
	gs, token, err := f.game.StartGameSession(context.Background(), campaignID, f.gm)
	if err != nil {
		t.Fatal(err)
	}
	return gs, token
}

func (f *fixture) register(t *testing.T, gs GameSession, login string) (auth.Account, Player) {
	t.Helper()
	acc, p, err := f.game.RegisterPlayer(context.Background(), gs, login, "player-password", "Player "+login)
	if err != nil {
		t.Fatal(err)
	}
	return acc, p
}

func (f *fixture) accountCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM account`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCampaigns(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	c, err := f.game.CreateCampaign(ctx, "  Кампания  ")
	if err != nil || c.Name != "Кампания" {
		t.Fatalf("create = %+v, %v; want trimmed name", c, err)
	}
	for _, name := range []string{"", "   ", strings.Repeat("я", 201)} {
		if _, err := f.game.CreateCampaign(ctx, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("CreateCampaign(%d runes) err = %v, want ErrInvalidName", len([]rune(name)), err)
		}
	}
	if _, err := f.game.CreateCampaign(ctx, strings.Repeat("я", 200)); err != nil {
		t.Errorf("200-rune name: %v", err)
	}

	f.clock = f.clock.Add(time.Second)
	second := f.campaign(t, "Вторая")
	list, err := f.game.ListCampaigns(ctx)
	if err != nil || len(list) != 3 || list[0].ID != c.ID || list[2].ID != second.ID {
		t.Fatalf("list = %+v, %v; want creation order", list, err)
	}

	renamed, err := f.game.RenameCampaign(ctx, c.ID, " Новое имя ")
	if err != nil || renamed.Name != "Новое имя" {
		t.Errorf("rename = %+v, %v", renamed, err)
	}
	if _, err := f.game.RenameCampaign(ctx, "nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename unknown: err = %v, want ErrNotFound", err)
	}
	if _, err := f.game.GetCampaign(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get unknown: err = %v, want ErrNotFound", err)
	}
}

func TestEnsureDevCampaign(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	created, err := f.game.EnsureDevCampaign(ctx)
	if err != nil || !created {
		t.Fatalf("first call = %v, %v; want true", created, err)
	}
	list, err := f.game.ListCampaigns(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Dev campaign" {
		t.Fatalf("campaigns = %+v, %v", list, err)
	}
	if created, err := f.game.EnsureDevCampaign(ctx); err != nil || created {
		t.Errorf("second call = %v, %v; want false", created, err)
	}
}

func TestStartGameSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c := f.campaign(t, "Camp")

	gs, token := f.start(t, c.ID)
	if len(token) != 43 {
		t.Errorf("token length = %d, want 43", len(token))
	}
	got, err := f.game.ResolveLink(ctx, token)
	if err != nil || got.ID != gs.ID || got.CampaignID != c.ID {
		t.Fatalf("ResolveLink = %+v, %v", got, err)
	}
	if active, err := f.game.ActiveGameSession(ctx); err != nil || active.ID != gs.ID {
		t.Errorf("ActiveGameSession = %+v, %v", active, err)
	}

	players, err := f.game.ListPlayers(ctx, c.ID)
	if err != nil || len(players) != 1 {
		t.Fatalf("players = %+v, %v", players, err)
	}
	if p := players[0]; p.Role != RoleGM || p.Color != playerColors[0] || p.AccountID != f.gm.ID || p.DisplayName != "master" {
		t.Errorf("GM participant = %+v", p)
	}

	// Starting again in the same campaign must not duplicate the GM.
	f.start(t, c.ID)
	if players, _ := f.game.ListPlayers(ctx, c.ID); len(players) != 1 {
		t.Errorf("GM participants after restart = %d, want 1", len(players))
	}

	camp, err := f.game.GetCampaign(ctx, c.ID)
	if err != nil || camp.ActiveGameSession == nil {
		t.Fatalf("campaign = %+v, %v; want an active game session", camp, err)
	}
	list, _ := f.game.ListCampaigns(ctx)
	if len(list) != 1 || list[0].ActiveGameSession == nil {
		t.Errorf("ListCampaigns = %+v, want ActiveGameSession set", list)
	}

	if _, _, err := f.game.StartGameSession(ctx, "nope", f.gm); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign: err = %v, want ErrNotFound", err)
	}
}

func TestStartGameSessionEndsPreviousGame(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a, b := f.campaign(t, "A"), f.campaign(t, "B")

	gsA, tokenA := f.start(t, a.ID)
	player, _ := f.register(t, gsA, "bob")
	playerToken, _, err := f.auth.CreatePlayerSession(ctx, player.ID, gsA.ID)
	if err != nil {
		t.Fatal(err)
	}
	gmToken, _, err := f.auth.CreateSession(ctx, f.gm.ID, auth.GMSessionTTL)
	if err != nil {
		t.Fatal(err)
	}

	gsB, _ := f.start(t, b.ID)

	if _, err := f.game.ResolveLink(ctx, tokenA); !errors.Is(err, ErrNoActiveGame) {
		t.Errorf("old link: err = %v, want ErrNoActiveGame", err)
	}
	if _, _, err := f.auth.ResolveSession(ctx, playerToken); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("player session of the ended game: err = %v, want ErrNoSession", err)
	}
	if _, _, err := f.auth.ResolveSession(ctx, gmToken); err != nil {
		t.Errorf("GM session must survive: %v", err)
	}
	if active, err := f.game.ActiveGameSession(ctx); err != nil || active.ID != gsB.ID {
		t.Errorf("active = %+v, %v; want the new game", active, err)
	}
}

func TestEndGameSessionAndRotateLink(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c := f.campaign(t, "Camp")
	gs, token := f.start(t, c.ID)

	fresh, err := f.game.RotateLink(ctx, gs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.game.ResolveLink(ctx, token); !errors.Is(err, ErrNoActiveGame) {
		t.Errorf("old token after rotate: err = %v, want ErrNoActiveGame", err)
	}
	if got, err := f.game.ResolveLink(ctx, fresh); err != nil || got.ID != gs.ID {
		t.Errorf("new token: %+v, %v", got, err)
	}

	if err := f.game.EndGameSession(ctx, gs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.game.ResolveLink(ctx, fresh); !errors.Is(err, ErrNoActiveGame) {
		t.Errorf("link of an ended game: err = %v, want ErrNoActiveGame", err)
	}
	if _, err := f.game.ActiveGameSession(ctx); !errors.Is(err, ErrNoActiveGame) {
		t.Errorf("ActiveGameSession: err = %v, want ErrNoActiveGame", err)
	}
	if err := f.game.EndGameSession(ctx, gs.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second end: err = %v, want ErrNotFound", err)
	}
	if _, err := f.game.RotateLink(ctx, gs.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("rotate an ended game: err = %v, want ErrNotFound", err)
	}
}

func TestRegisterPlayer(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c := f.campaign(t, "Camp")
	gs, _ := f.start(t, c.ID)

	acc, p, err := f.game.RegisterPlayer(ctx, gs, "Bob", "player-password", " Боб ")
	if err != nil {
		t.Fatal(err)
	}
	if acc.IsGM || acc.Login != "bob" {
		t.Errorf("account = %+v, want non-GM login bob", acc)
	}
	if p.Role != RolePlayer || p.Color != playerColors[1] || p.DisplayName != "Боб" || p.AccountID != acc.ID {
		t.Errorf("player = %+v", p)
	}
	if got, err := f.auth.Authenticate(ctx, "bob", "player-password"); err != nil || got.ID != acc.ID {
		t.Errorf("Authenticate = %+v, %v", got, err)
	}

	before := f.accountCount(t)
	for _, tt := range []struct {
		name                   string
		login, password, label string
		want                   error
	}{
		{"login taken", "bob", "player-password", "X", ErrLoginTaken},
		{"login taken, other case", "BOB", "player-password", "X", ErrLoginTaken},
		{"bad login", "a b", "player-password", "X", auth.ErrInvalidLogin},
		{"short password", "carol", "short", "X", auth.ErrInvalidPassword},
		{"empty name", "carol", "player-password", "  ", ErrInvalidName},
	} {
		if _, _, err := f.game.RegisterPlayer(ctx, gs, tt.login, tt.password, tt.label); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if after := f.accountCount(t); after != before {
		t.Errorf("accounts = %d after failed registrations, want %d", after, before)
	}

	if err := f.game.EndGameSession(ctx, gs.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.game.RegisterPlayer(ctx, gs, "dave", "player-password", "Dave"); !errors.Is(err, ErrNoActiveGame) {
		t.Errorf("register in an ended game: err = %v, want ErrNoActiveGame", err)
	}
	if after := f.accountCount(t); after != before {
		t.Errorf("accounts = %d after registering into an ended game, want %d", after, before)
	}
}

func TestJoinPlayer(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a, b := f.campaign(t, "A"), f.campaign(t, "B")
	gsA, _ := f.start(t, a.ID)
	acc, first := f.register(t, gsA, "bob")

	again, err := f.game.JoinPlayer(ctx, gsA, acc)
	if err != nil || again.ID != first.ID {
		t.Fatalf("rejoin = %+v, %v; want player %s", again, err, first.ID)
	}

	gsB, _ := f.start(t, b.ID)
	other, err := f.game.JoinPlayer(ctx, gsB, acc)
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID || other.CampaignID != b.ID || other.DisplayName != acc.Login || other.Role != RolePlayer {
		t.Errorf("player in the new campaign = %+v", other)
	}

	gm, err := f.game.JoinPlayer(ctx, gsB, f.gm)
	if err != nil || gm.Role != RoleGM {
		t.Errorf("GM join = %+v, %v; want RoleGM", gm, err)
	}

	if _, err := f.game.JoinPlayer(ctx, gsA, acc); !errors.Is(err, ErrNoActiveGame) {
		t.Errorf("join an ended game: err = %v, want ErrNoActiveGame", err)
	}
}

func TestRemovePlayer(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c := f.campaign(t, "Camp")
	gs, _ := f.start(t, c.ID)
	acc, p := f.register(t, gs, "bob")
	token, _, err := f.auth.CreatePlayerSession(ctx, acc.ID, gs.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.game.RemovePlayer(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.game.PlayerForAccount(ctx, c.ID, acc.ID)
	if err != nil || !got.Removed {
		t.Errorf("player = %+v, %v; want Removed", got, err)
	}
	if _, _, err := f.auth.ResolveSession(ctx, token); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("login session of a removed player: err = %v, want ErrNoSession", err)
	}
	if _, err := f.game.JoinPlayer(ctx, gs, acc); !errors.Is(err, ErrRemoved) {
		t.Errorf("join after removal: err = %v, want ErrRemoved", err)
	}
	if err := f.game.RemovePlayer(ctx, p.ID); err != nil {
		t.Errorf("repeated removal: %v", err)
	}
	if err := f.game.RemovePlayer(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown player: err = %v, want ErrNotFound", err)
	}

	players, err := f.game.ListPlayers(ctx, c.ID)
	if err != nil || len(players) != 2 {
		t.Fatalf("players = %+v, %v; want GM and the removed player listed", players, err)
	}
	if err := f.game.RemovePlayer(ctx, players[0].ID); !errors.Is(err, ErrCannotRemoveGM) {
		t.Errorf("remove GM: err = %v, want ErrCannotRemoveGM", err)
	}
	if _, err := f.game.ListPlayers(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ListPlayers of an unknown campaign: err = %v, want ErrNotFound", err)
	}
	if _, err := f.game.PlayerForAccount(ctx, c.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("PlayerForAccount unknown: err = %v, want ErrNotFound", err)
	}
}

func TestPlayerColors(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c := f.campaign(t, "Camp")
	gs, _ := f.start(t, c.ID) // the GM is participant 1

	for i := range 8 {
		f.register(t, gs, "player"+string(rune('a'+i)))
	}
	players, err := f.game.ListPlayers(ctx, c.ID)
	if err != nil || len(players) != 9 {
		t.Fatalf("players = %d, %v; want 9", len(players), err)
	}
	valid := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	for i, p := range players {
		if want := playerColors[i%len(playerColors)]; p.Color != want {
			t.Errorf("participant %d color = %s, want %s", i+1, p.Color, want)
		}
		if !valid.MatchString(p.Color) {
			t.Errorf("participant %d color %q does not match the schema CHECK", i+1, p.Color)
		}
	}
	if players[8].Color != playerColors[0] {
		t.Errorf("9th color = %s, want it to wrap to %s", players[8].Color, playerColors[0])
	}
}
