package vtt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/balfag/dnd-vtt/internal/dice"
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Limits of the initiative tracker (protocol.md, «Инициатива»).
const (
	maxCombatBatch = 50
	minInitiative  = -99
	maxInitiative  = 99
)

// combatantRow is a participant as stored, with the data of its token that the
// rendering and the access checks need.
type combatantRow struct {
	id             string
	sceneID        string
	tokenID        string
	initiative     *int // nil: not set
	addedSeq       int
	hidden         bool
	name           string
	nameVisibility string
	owners         []string
}

// turnOrder sorts combatants: initiative descending, nil last, then added_seq
// ascending. It returns a sorted copy.
func turnOrder(cs []combatantRow) []combatantRow {
	out := slices.Clone(cs)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.initiative != nil && b.initiative == nil:
			return true
		case a.initiative == nil && b.initiative != nil:
			return false
		case a.initiative != nil && *a.initiative != *b.initiative:
			return *a.initiative > *b.initiative
		}
		return a.addedSeq < b.addedSeq
	})
	return out
}

func indexOf(order []combatantRow, id string) int {
	return slices.IndexFunc(order, func(c combatantRow) bool { return c.id == id })
}

// next returns the combatant after cur (cur == "" -> the first) and whether the
// round grows. An empty order is ErrConflict.
func next(order []combatantRow, cur string) (string, bool, error) {
	if len(order) == 0 {
		return "", false, fmt.Errorf("%w: the fight has no combatants", ErrConflict)
	}
	i := indexOf(order, cur)
	switch {
	case i < 0:
		return order[0].id, false, nil
	case i == len(order)-1:
		return order[0].id, true, nil
	}
	return order[i+1].id, false, nil
}

// prev returns the combatant before cur and whether the round shrinks. At the
// first combatant of round 1, with no turn yet or with no combatants it is
// ErrConflict.
func prev(order []combatantRow, cur string, round int) (string, bool, error) {
	i := indexOf(order, cur)
	before := i - 1
	switch {
	case i < 0:
		return "", false, fmt.Errorf("%w: there is no turn to go back from", ErrConflict)
	case before >= 0 && before < len(order):
		return order[before].id, false, nil
	case round <= 1:
		return "", false, fmt.Errorf("%w: this is the first turn of the fight", ErrConflict)
	}
	return order[len(order)-1].id, true, nil
}

// CombatView is a tracker as stored: every participant, in turn order, with
// the data needed to render it for any viewer.
type CombatView struct {
	SceneID string
	Round   int
	Turn    string // "" before the first combat.next
	Members []combatantRow
}

// For renders the tracker for v (protocol.md, «Инициатива»): the GM sees all;
// a player does not see hidden participants nor the names that nameVisibility
// "gm" keeps back, and has no turn when it is a hidden participant's.
func (cv CombatView) For(v ws.Viewer) protocol.Combat {
	out := protocol.Combat{SceneID: cv.SceneID, Round: cv.Round, Combatants: []protocol.Combatant{}}
	turnShown := cv.Turn != ""
	for _, m := range cv.Members {
		if m.hidden && !v.IsGM {
			if m.id == cv.Turn {
				turnShown = false
			}
			continue
		}
		name := m.name
		if !v.IsGM && m.nameVisibility == "gm" {
			name = ""
		}
		out.Combatants = append(out.Combatants, protocol.Combatant{
			ID: m.id, TokenID: m.tokenID, Name: name, Initiative: m.initiative, Hidden: m.hidden,
		})
	}
	if turnShown {
		turn := cv.Turn
		out.TurnCombatantID = &turn
	}
	return out
}

// CombatResult is the state of a scene's tracker after a change and whether
// the scene was active at that moment; the renderer of combat.changed captures
// both. Combat is nil when the fight is over.
type CombatResult struct {
	SceneID string
	Active  bool
	Combat  *CombatView
}

const combatantSelect = `SELECT c.id, c.scene_id, c.token_id, c.initiative, c.added_seq, c.hidden, t.name, t.name_visibility, t.owners
	FROM combatant c JOIN token t ON t.id = c.token_id`

func scanCombatant(r rowScanner) (combatantRow, error) {
	var (
		c         combatantRow
		initiat   sql.NullInt64
		hidden    int
		ownersRaw string
	)
	if err := r.Scan(&c.id, &c.sceneID, &c.tokenID, &initiat, &c.addedSeq, &hidden, &c.name, &c.nameVisibility, &ownersRaw); err != nil {
		return combatantRow{}, err
	}
	if initiat.Valid {
		n := int(initiat.Int64)
		c.initiative = &n
	}
	c.hidden = hidden != 0
	if err := json.Unmarshal([]byte(ownersRaw), &c.owners); err != nil {
		return combatantRow{}, fmt.Errorf("decode owners of token %s: %w", c.tokenID, err)
	}
	return c, nil
}

// listCombatants reads the participants of a scene's fight in turn order.
func listCombatants(ctx context.Context, q queryer, sceneID string) ([]combatantRow, error) {
	rows, err := q.QueryContext(ctx, combatantSelect+` WHERE c.scene_id = ? ORDER BY c.added_seq`, sceneID)
	if err != nil {
		return nil, fmt.Errorf("list combatants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []combatantRow
	for rows.Next() {
		c, err := scanCombatant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan combatant: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list combatants: %w", err)
	}
	return turnOrder(out), nil
}

// loadCombatant reads a participant of the campaign; another campaign's is ErrNotFound.
func loadCombatant(ctx context.Context, q queryer, campaignID, combatantID string) (combatantRow, error) {
	c, err := scanCombatant(q.QueryRowContext(ctx,
		combatantSelect+` JOIN scene s ON s.id = c.scene_id WHERE c.id = ? AND s.campaign_id = ?`, combatantID, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return combatantRow{}, ErrNotFound
	}
	if err != nil {
		return combatantRow{}, fmt.Errorf("load combatant: %w", err)
	}
	return c, nil
}

// combatHead is the combat row of a scene.
type combatHead struct {
	round int
	turn  string
}

// loadHead reads the combat row of a scene of the campaign; no scene or no
// fight is ErrNotFound.
func loadHead(ctx context.Context, q queryer, campaignID, sceneID string) (combatHead, error) {
	var (
		h    combatHead
		turn sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT c.round, c.turn_combatant_id FROM combat c JOIN scene s ON s.id = c.scene_id WHERE c.scene_id = ? AND s.campaign_id = ?`,
		sceneID, campaignID).Scan(&h.round, &turn)
	if errors.Is(err, sql.ErrNoRows) {
		return combatHead{}, ErrNotFound
	}
	if err != nil {
		return combatHead{}, fmt.Errorf("load combat: %w", err)
	}
	h.turn = turn.String
	return h, nil
}

// loadCombatView reads the whole tracker of a scene.
func loadCombatView(ctx context.Context, q queryer, campaignID, sceneID string) (CombatView, error) {
	h, err := loadHead(ctx, q, campaignID, sceneID)
	if err != nil {
		return CombatView{}, err
	}
	members, err := listCombatants(ctx, q, sceneID)
	if err != nil {
		return CombatView{}, err
	}
	return CombatView{SceneID: sceneID, Round: h.round, Turn: h.turn, Members: members}, nil
}

// combatResult reads the tracker of a scene with the activity of the scene.
func combatResult(ctx context.Context, q queryer, campaignID, sceneID string) (CombatResult, error) {
	view, err := loadCombatView(ctx, q, campaignID, sceneID)
	if err != nil {
		return CombatResult{}, err
	}
	active, err := activeSceneID(ctx, q, campaignID)
	if err != nil {
		return CombatResult{}, err
	}
	return CombatResult{SceneID: sceneID, Active: active == sceneID, Combat: &view}, nil
}

// combatIfAny is combatResult, but a scene without a fight gives nil.
func combatIfAny(ctx context.Context, q queryer, campaignID, sceneID string) (*CombatResult, error) {
	res, err := combatResult(ctx, q, campaignID, sceneID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SceneCombat reads the fight of a scene; nil if there is none.
func (s *Service) SceneCombat(ctx context.Context, campaignID, sceneID string) (*CombatResult, error) {
	return combatIfAny(ctx, s.db, campaignID, sceneID)
}

// activeCombat renders the tracker of the active scene for v; nil if none.
func activeCombat(ctx context.Context, q queryer, campaignID string, v ws.Viewer) (*protocol.Combat, error) {
	active, err := activeSceneID(ctx, q, campaignID)
	if err != nil || active == "" {
		return nil, err
	}
	view, err := loadCombatView(ctx, q, campaignID, active)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := view.For(v)
	return &c, nil
}

func forbidden(what string) error {
	return ws.Errorf(protocol.CodeForbidden, "%s", what)
}

// playOnActive lets a player act on the active scene only; the GM anywhere.
func playOnActive(ctx context.Context, q queryer, campaignID string, v ws.Viewer, sceneID string) error {
	if v.IsGM {
		return nil
	}
	active, err := activeSceneID(ctx, q, campaignID)
	if err != nil {
		return err
	}
	if active != sceneID {
		return forbidden("the fight is not on the active scene")
	}
	return nil
}

func (c combatantRow) ownedBy(v ws.Viewer) bool {
	return v.PlayerID != "" && slices.Contains(c.owners, v.PlayerID)
}

// checkIDs checks a list of 1..maxCombatBatch unique ids.
func checkIDs(ids []string, what string) error {
	if len(ids) < 1 || len(ids) > maxCombatBatch {
		return invalid("%s must be 1..%d", what, maxCombatBatch)
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return invalid("%s must be unique", what)
		}
		seen[id] = true
	}
	return nil
}

// StartCombat creates an empty tracker for a scene of the campaign.
func (s *Service) StartCombat(ctx context.Context, campaignID, sceneID string) (CombatResult, error) {
	var res CombatResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := loadScene(ctx, tx, campaignID, sceneID); err != nil {
			return err
		}
		if _, err := loadHead(ctx, tx, campaignID, sceneID); err == nil {
			return fmt.Errorf("%w: the scene already has a fight", ErrConflict)
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO combat (scene_id, round, turn_combatant_id, created_at) VALUES (?, 1, NULL, ?)`,
			sceneID, s.now().UnixMilli()); err != nil {
			return fmt.Errorf("insert combat: %w", err)
		}
		var err error
		res, err = combatResult(ctx, tx, campaignID, sceneID)
		return err
	})
	return res, err
}

// EndCombat removes the tracker of a scene; its participants go with it.
func (s *Service) EndCombat(ctx context.Context, campaignID, sceneID string) (CombatResult, error) {
	var res CombatResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := loadHead(ctx, tx, campaignID, sceneID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM combat WHERE scene_id = ?`, sceneID); err != nil {
			return fmt.Errorf("delete combat: %w", err)
		}
		active, err := activeSceneID(ctx, tx, campaignID)
		if err != nil {
			return err
		}
		res = CombatResult{SceneID: sceneID, Active: active == sceneID}
		return nil
	})
	return res, err
}

// AddCombatants adds tokens of the scene to its fight; tokens that already
// take part are skipped. A participant starts hidden when its token is hidden
// or on the gm layer.
func (s *Service) AddCombatants(ctx context.Context, campaignID string, in protocol.CombatAdd) (CombatResult, error) {
	if err := checkIDs(in.TokenIDs, "tokenIds"); err != nil {
		return CombatResult{}, err
	}
	var res CombatResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := loadHead(ctx, tx, campaignID, in.SceneID); err != nil {
			return err
		}
		var seq int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(added_seq), 0) FROM combatant WHERE scene_id = ?`, in.SceneID).Scan(&seq); err != nil {
			return fmt.Errorf("next added_seq: %w", err)
		}
		type tokenInfo struct {
			id     string
			hidden bool
		}
		var add []tokenInfo
		for _, tokenID := range in.TokenIDs {
			var hidden int
			var layer string
			err := tx.QueryRowContext(ctx, `SELECT hidden, layer FROM token WHERE id = ? AND scene_id = ?`, tokenID, in.SceneID).Scan(&hidden, &layer)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("read token: %w", err)
			}
			var taken int
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM combatant WHERE scene_id = ? AND token_id = ?`, in.SceneID, tokenID).Scan(&taken)
			if err == nil {
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check combatant: %w", err)
			}
			add = append(add, tokenInfo{tokenID, hidden != 0 || layer == "gm"})
		}
		for _, t := range add {
			seq++
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO combatant (id, scene_id, token_id, initiative, added_seq, hidden) VALUES (?, ?, ?, NULL, ?, ?)`,
				newID(), in.SceneID, t.id, seq, t.hidden); err != nil {
				return fmt.Errorf("insert combatant: %w", err)
			}
		}
		var err error
		res, err = combatResult(ctx, tx, campaignID, in.SceneID)
		return err
	})
	return res, err
}

// RemoveCombatant takes a participant out of its fight. When it held the turn,
// the turn first passes to the next one in the order (the round stays), then
// the participant is deleted: the other way round the foreign key would clear
// the turn.
func (s *Service) RemoveCombatant(ctx context.Context, campaignID, combatantID string) (CombatResult, error) {
	var res CombatResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		c, err := loadCombatant(ctx, tx, campaignID, combatantID)
		if err != nil {
			return err
		}
		head, err := loadHead(ctx, tx, campaignID, c.sceneID)
		if err != nil {
			return err
		}
		if head.turn == c.id {
			order, err := listCombatants(ctx, tx, c.sceneID)
			if err != nil {
				return err
			}
			var turn sql.NullString
			if len(order) > 1 {
				turn = sql.NullString{String: order[(indexOf(order, c.id)+1)%len(order)].id, Valid: true}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE combat SET turn_combatant_id = ? WHERE scene_id = ?`, turn, c.sceneID); err != nil {
				return fmt.Errorf("pass the turn: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM combatant WHERE id = ?`, c.id); err != nil {
			return fmt.Errorf("delete combatant: %w", err)
		}
		res, err = combatResult(ctx, tx, campaignID, c.sceneID)
		return err
	})
	return res, err
}

// mayRate checks the right of v to set or roll the initiative of c: the GM
// always; a player for a participant of own token, in the fight of the active scene.
func mayRate(ctx context.Context, q queryer, campaignID string, v ws.Viewer, c combatantRow) error {
	if v.IsGM {
		return nil
	}
	if err := playOnActive(ctx, q, campaignID, v, c.sceneID); err != nil {
		return err
	}
	if !c.ownedBy(v) {
		return forbidden("you cannot rate this combatant")
	}
	return nil
}

func nullInt(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}

// SetInitiative sets the initiative of a participant; nil clears it.
func (s *Service) SetInitiative(ctx context.Context, campaignID string, v ws.Viewer, combatantID string, value *int) (CombatResult, error) {
	if value != nil && (*value < minInitiative || *value > maxInitiative) {
		return CombatResult{}, invalid("value must be %d..%d or null", minInitiative, maxInitiative)
	}
	var res CombatResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		c, err := loadCombatant(ctx, tx, campaignID, combatantID)
		if err != nil {
			return err
		}
		if err := mayRate(ctx, tx, campaignID, v, c); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE combatant SET initiative = ? WHERE id = ?`, nullInt(value), c.id); err != nil {
			return fmt.Errorf("set initiative: %w", err)
		}
		res, err = combatResult(ctx, tx, campaignID, c.sceneID)
		return err
	})
	return res, err
}

// RollInitiative rolls 1d20 with r for each participant and stores the totals.
// If v may not rate one of them, nothing is rolled.
func (s *Service) RollInitiative(ctx context.Context, campaignID string, v ws.Viewer, combatantIDs []string, r dice.Roller) (CombatResult, error) {
	if err := checkIDs(combatantIDs, "combatantIds"); err != nil {
		return CombatResult{}, err
	}
	formula, err := dice.Parse("1d20")
	if err != nil {
		return CombatResult{}, fmt.Errorf("parse initiative formula: %w", err)
	}
	var res CombatResult
	err = s.tx(ctx, func(tx *sql.Tx) error {
		rows := make([]combatantRow, 0, len(combatantIDs))
		for _, id := range combatantIDs {
			c, err := loadCombatant(ctx, tx, campaignID, id)
			if err != nil {
				return err
			}
			if len(rows) > 0 && c.sceneID != rows[0].sceneID {
				return invalid("combatantIds must belong to one fight")
			}
			rows = append(rows, c)
		}
		for _, c := range rows {
			if err := mayRate(ctx, tx, campaignID, v, c); err != nil {
				return err
			}
		}
		for _, c := range rows {
			total := formula.Roll(r).Total
			if _, err := tx.ExecContext(ctx, `UPDATE combatant SET initiative = ? WHERE id = ?`, total, c.id); err != nil {
				return fmt.Errorf("store initiative: %w", err)
			}
		}
		var err error
		res, err = combatResult(ctx, tx, campaignID, rows[0].sceneID)
		return err
	})
	return res, err
}

// SetCombatantHidden shows or hides a participant to the players.
func (s *Service) SetCombatantHidden(ctx context.Context, campaignID, combatantID string, hidden bool) (CombatResult, error) {
	var res CombatResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		c, err := loadCombatant(ctx, tx, campaignID, combatantID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE combatant SET hidden = ? WHERE id = ?`, hidden, c.id); err != nil {
			return fmt.Errorf("set hidden: %w", err)
		}
		res, err = combatResult(ctx, tx, campaignID, c.sceneID)
		return err
	})
	return res, err
}

// NextTurn gives the turn to the next participant. The GM may always; a player
// only when the turn is of a participant of own token on the active scene.
func (s *Service) NextTurn(ctx context.Context, campaignID string, v ws.Viewer, sceneID string) (CombatResult, error) {
	return s.moveTurn(ctx, campaignID, v, sceneID, func(order []combatantRow, h combatHead) (string, int, error) {
		id, grows, err := next(order, h.turn)
		if grows {
			return id, h.round + 1, err
		}
		return id, h.round, err
	})
}

// PrevTurn gives the turn back to the previous participant; GM only (the
// handler checks), and the round never goes below 1.
func (s *Service) PrevTurn(ctx context.Context, campaignID string, v ws.Viewer, sceneID string) (CombatResult, error) {
	return s.moveTurn(ctx, campaignID, v, sceneID, func(order []combatantRow, h combatHead) (string, int, error) {
		id, shrinks, err := prev(order, h.turn, h.round)
		if shrinks {
			return id, h.round - 1, err
		}
		return id, h.round, err
	})
}

func (s *Service) moveTurn(ctx context.Context, campaignID string, v ws.Viewer, sceneID string,
	step func([]combatantRow, combatHead) (string, int, error)) (CombatResult, error) {
	var res CombatResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := playOnActive(ctx, tx, campaignID, v, sceneID); err != nil {
			return err
		}
		head, err := loadHead(ctx, tx, campaignID, sceneID)
		if err != nil {
			return err
		}
		order, err := listCombatants(ctx, tx, sceneID)
		if err != nil {
			return err
		}
		if !v.IsGM {
			holder := indexOf(order, head.turn)
			if holder < 0 || !order[holder].ownedBy(v) {
				return forbidden("it is not your turn")
			}
		}
		turn, round, err := step(order, head)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE combat SET turn_combatant_id = ?, round = ? WHERE scene_id = ?`, turn, round, sceneID); err != nil {
			return fmt.Errorf("move the turn: %w", err)
		}
		res, err = combatResult(ctx, tx, campaignID, sceneID)
		return err
	})
	return res, err
}
