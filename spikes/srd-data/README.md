# spikes/srd-data

Spike #37: SRD 5.2 as structured data and a rules effects schema (ADR-0009).
Not product code: a separate Go module (stdlib only), kept out of the root `go test ./...`.

- `schema/` - Go structs for content records and the effects language
- `data/` - hand-encoded SRD 5.2 sample records (20 spells, 2 classes, 3 species, ...)
- `engine/` - prototype interpreter: character sheet and spell/attack application, with tests
- Run: `cd spikes/srd-data && go vet ./... && go test ./...`
- Report: [REPORT.md](REPORT.md)
