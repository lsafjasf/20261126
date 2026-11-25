package match

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Decision records a user's lock or reject action on a candidate pair.
type Decision struct {
	ProxyID    string    `json:"proxy_id"`
	OriginalID string    `json:"original_id"`
	State      string    `json:"state"` // "locked" or "rejected"
	Score      float64   `json:"score"`
	At         time.Time `json:"at"`
}

// Decisions is the persistent store of user choices.
type Decisions struct {
	Items []Decision `json:"decisions"`
}

func pairKey(proxyID, origID string) string { return proxyID + "=>" + origID }

// stateOf returns the latest decision state for a pair, "" if none.
func (d *Decisions) stateOf(proxyID, origID string) string {
	state := ""
	for _, it := range d.Items {
		if pairKey(it.ProxyID, it.OriginalID) == pairKey(proxyID, origID) {
			state = it.State
		}
	}
	return state
}

// Lock marks a pair as confirmed. A rejected pair must be cleared first.
func (d *Decisions) Lock(proxyID, origID string, score float64) error {
	if s := d.stateOf(proxyID, origID); s == "rejected" {
		return fmt.Errorf("pair %s was rejected; clear the rejection before locking", pairKey(proxyID, origID))
	}
	d.Items = append(d.Items, Decision{
		ProxyID: proxyID, OriginalID: origID, State: "locked",
		Score: score, At: time.Now().UTC(),
	})
	return nil
}

// Reject marks a pair as refused. A locked pair must be unlocked first.
func (d *Decisions) Reject(proxyID, origID string, score float64) error {
	if s := d.stateOf(proxyID, origID); s == "locked" {
		return fmt.Errorf("pair %s is locked; unlock it before rejecting", pairKey(proxyID, origID))
	}
	d.Items = append(d.Items, Decision{
		ProxyID: proxyID, OriginalID: origID, State: "rejected",
		Score: score, At: time.Now().UTC(),
	})
	return nil
}

// Clear removes all decisions for a pair (unlock / un-reject).
func (d *Decisions) Clear(proxyID, origID string) {
	kept := d.Items[:0]
	for _, it := range d.Items {
		if pairKey(it.ProxyID, it.OriginalID) != pairKey(proxyID, origID) {
			kept = append(kept, it)
		}
	}
	d.Items = kept
}

// IsRejected reports whether the pair's latest state is "rejected".
func (d *Decisions) IsRejected(proxyID, origID string) bool {
	return d.stateOf(proxyID, origID) == "rejected"
}

// LockedProxySet returns proxy IDs that currently hold a lock.
func (d *Decisions) LockedProxySet() map[string]bool {
	out := map[string]bool{}
	for _, it := range d.Items {
		if it.State == "locked" && d.stateOf(it.ProxyID, it.OriginalID) == "locked" {
			out[it.ProxyID] = true
		}
	}
	return out
}

// LockedOriginalSet returns original IDs currently locked to some proxy.
func (d *Decisions) LockedOriginalSet() map[string]bool {
	out := map[string]bool{}
	for _, it := range d.Items {
		if it.State == "locked" && d.stateOf(it.ProxyID, it.OriginalID) == "locked" {
			out[it.OriginalID] = true
		}
	}
	return out
}

// LockedPairs returns all currently locked pairs.
func (d *Decisions) LockedPairs() []Decision {
	var out []Decision
	for _, it := range d.Items {
		if it.State == "locked" && d.stateOf(it.ProxyID, it.OriginalID) == "locked" {
			out = append(out, it)
		}
	}
	return out
}

// LoadDecisions reads a decisions file; a missing file yields an empty store.
func LoadDecisions(path string) (*Decisions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Decisions{}, nil
		}
		return nil, fmt.Errorf("read decisions: %w", err)
	}
	var d Decisions
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse decisions: %w", err)
	}
	return &d, nil
}

// Save writes decisions to disk as indented JSON.
func (d *Decisions) Save(path string) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
