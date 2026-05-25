package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DataObject is one row of `fetch dt.system.data_objects | filter type == "table"`.
// Frontend uses this list to populate the Visual Query Builder's source dropdown.
type DataObject struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// dataObjectsTTL bounds how long we trust the cached list. Tables rarely
// appear or disappear, so an hour is generous; the upside is that if the
// tenant gets a new bucket type our dropdown picks it up without a restart.
const dataObjectsTTL = time.Hour

// dataObjectsDQL is the canonical list of queryable data objects.
// We exclude `view` (mostly deprecated dt.entity.* per-entity-type views)
// and the `metrics` pseudo-source — `metrics` is exposed by the frontend
// as a synthetic option because it uses the `timeseries` command, not `fetch`.
const dataObjectsDQL = `fetch dt.system.data_objects | filter type == "table" | fields name, display_name | sort name asc`

type dataObjectsCache struct {
	mu      sync.Mutex
	at      time.Time
	objects []DataObject
}

func (c *dataObjectsCache) get() ([]DataObject, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.objects == nil || time.Since(c.at) > dataObjectsTTL {
		return nil, false
	}
	return c.objects, true
}

func (c *dataObjectsCache) set(objs []DataObject) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = time.Now()
	c.objects = objs
}

// listDataObjects returns the cached data-objects list, refreshing from
// Grail on a cache miss. Errors propagate — the frontend falls back to a
// hardcoded default list in that case.
func (d *Datasource) listDataObjects(ctx context.Context) ([]DataObject, error) {
	if cached, ok := d.dataObjects.get(); ok {
		return cached, nil
	}
	resp, err := d.dt.Query(ctx, dataObjectsDQL, time.Time{}, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("query dt.system.data_objects: %w", err)
	}
	records := resp.GetRecords()
	out := make([]DataObject, 0, len(records))
	for _, r := range records {
		name, _ := r["name"].(string)
		if name == "" {
			continue
		}
		display, _ := r["display_name"].(string)
		if display == "" {
			display = name
		}
		out = append(out, DataObject{Name: name, DisplayName: display})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	d.dataObjects.set(out)
	return out, nil
}

// marshalDataObjects renders the response payload for the /data-objects
// resource endpoint.
func marshalDataObjects(objs []DataObject) []byte {
	b, _ := json.Marshal(objs)
	return b
}
