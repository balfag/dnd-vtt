package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"

	"github.com/balfag/dnd-vtt/spikes/srd-data/schema"
)

// Compendium holds all loaded content records by ID.
type Compendium struct {
	Spells      map[string]schema.Spell
	Classes     map[string]schema.Class
	Subclasses  map[string]schema.Subclass
	Species     map[string]schema.Species
	Backgrounds map[string]schema.Background
	Feats       map[string]schema.Feat
	Items       map[string]schema.Item
	Monsters    map[string]schema.Monster
	Conditions  map[string]schema.Condition
}

// loadFile strictly decodes a JSON array (unknown fields are an error) and
// checks the common Meta rules shared by SRD and homebrew packs.
func loadFile[T any](fsys fs.FS, name string, meta func(*T) schema.Meta) (map[string]T, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var list []T
	if err := dec.Decode(&list); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	out := make(map[string]T, len(list))
	for i := range list {
		m := meta(&list[i])
		if m.SchemaVersion != schema.SchemaVersion {
			return nil, fmt.Errorf("%s: %q has schemaVersion %d, want %d", name, m.ID, m.SchemaVersion, schema.SchemaVersion)
		}
		if m.Source != schema.SourceSRD && m.Source != schema.SourceHomebrew {
			return nil, fmt.Errorf("%s: %q has unknown source %q", name, m.ID, m.Source)
		}
		if _, dup := out[m.ID]; dup || m.ID == "" {
			return nil, fmt.Errorf("%s: duplicate or empty id %q", name, m.ID)
		}
		out[m.ID] = list[i]
	}
	return out, nil
}

// Load reads data/*.json from fsys and validates cross-references.
func Load(fsys fs.FS) (*Compendium, error) {
	var c Compendium
	var err error
	steps := []func() error{
		func() (e error) {
			c.Spells, e = loadFile(fsys, "spells.json", func(r *schema.Spell) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Classes, e = loadFile(fsys, "classes.json", func(r *schema.Class) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Subclasses, e = loadFile(fsys, "subclasses.json", func(r *schema.Subclass) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Species, e = loadFile(fsys, "species.json", func(r *schema.Species) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Backgrounds, e = loadFile(fsys, "backgrounds.json", func(r *schema.Background) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Feats, e = loadFile(fsys, "feats.json", func(r *schema.Feat) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Items, e = loadFile(fsys, "items.json", func(r *schema.Item) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Monsters, e = loadFile(fsys, "monsters.json", func(r *schema.Monster) schema.Meta { return r.Meta })
			return
		},
		func() (e error) {
			c.Conditions, e = loadFile(fsys, "conditions.json", func(r *schema.Condition) schema.Meta { return r.Meta })
			return
		},
	}
	for _, s := range steps {
		if err = s(); err != nil {
			return nil, err
		}
	}
	return &c, c.validate()
}

func (c *Compendium) validate() error {
	for id, sc := range c.Subclasses {
		if _, ok := c.Classes[sc.Class]; !ok {
			return fmt.Errorf("subclass %q: unknown class %q", id, sc.Class)
		}
	}
	for id, cl := range c.Classes {
		for _, s := range cl.Subclasses {
			if _, ok := c.Subclasses[s]; !ok {
				return fmt.Errorf("class %q: unknown subclass %q", id, s)
			}
		}
		have := map[string]bool{}
		for _, f := range cl.Features {
			have[f.ID] = true
		}
		for _, lv := range cl.Levels {
			for _, f := range lv.Features {
				if !have[f] {
					return fmt.Errorf("class %q level %d: unknown feature %q", id, lv.Level, f)
				}
			}
		}
	}
	for id, b := range c.Backgrounds {
		if _, ok := c.Feats[b.Feat]; !ok {
			return fmt.Errorf("background %q: unknown feat %q", id, b.Feat)
		}
	}
	return nil
}

// CoverageEntry counts effects of one record.
type CoverageEntry struct {
	Declarative int
	Special     int
	Handlers    []string
}

// Coverage counts, for every spell, class feature, subclass feature, species
// trait, feat, item and monster action, how many effect nodes are declarative
// and how many are "special" code handlers. Key: "<kind>/<id>[/<feature>]".
func (c *Compendium) Coverage() map[string]*CoverageEntry {
	out := map[string]*CoverageEntry{}
	add := func(key string, effs []schema.Effect) {
		e := out[key]
		if e == nil {
			e = &CoverageEntry{}
			out[key] = e
		}
		walkEffects(effs, func(ef schema.Effect) {
			if ef.Type == "special" {
				e.Special++
				e.Handlers = append(e.Handlers, ef.Handler)
			} else {
				e.Declarative++
			}
		})
		sort.Strings(e.Handlers)
	}
	for id, s := range c.Spells {
		add("spell/"+id, s.Effects)
	}
	for id, cl := range c.Classes {
		for _, f := range cl.Features {
			add("class/"+id+"/"+f.ID, f.Effects)
		}
	}
	for id, sc := range c.Subclasses {
		for _, f := range sc.Features {
			add("subclass/"+id+"/"+f.ID, f.Effects)
		}
	}
	for id, sp := range c.Species {
		for _, f := range sp.Traits {
			add("species/"+id+"/"+f.ID, f.Effects)
		}
	}
	for id, f := range c.Feats {
		add("feat/"+id, f.Effects)
	}
	for id, m := range c.Monsters {
		for _, a := range m.Actions {
			add("monster/"+id+"/"+a.Name, a.Effects)
		}
		for _, a := range m.BonusActions {
			add("monster/"+id+"/"+a.Name, a.Effects)
		}
	}
	return out
}

// walkEffects visits every effect node, descending into nested lists.
func walkEffects(effs []schema.Effect, fn func(schema.Effect)) {
	for _, e := range effs {
		fn(e)
		walkEffects(e.OnHit, fn)
		walkEffects(e.Apply, fn)
		walkEffects(e.OnEnd, fn)
		if e.Condition != nil {
			walkEffects(e.Condition.OnExpire, fn)
		}
		if e.Choice != nil {
			walkEffects(e.Choice.Then, fn)
			for _, v := range e.Choice.ThenBy {
				walkEffects(v, fn)
			}
		}
	}
}
