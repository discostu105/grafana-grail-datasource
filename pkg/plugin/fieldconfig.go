package plugin

import (
	"math"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// applyLegendFormat sets Field.Config.DisplayName on every non-time value
// field. The template is taken verbatim from the queryModel; Grafana
// resolves ${__field.labels.<x>} and `{{ x }}` style placeholders itself.
//
// We deliberately leave DisplayNameFromDS (set in units.go from the
// preferred label) alone — DisplayName wins, but the from-DS fallback is
// still useful when no template is provided.
func applyLegendFormat(frames []*data.Frame, format string) {
	if format == "" {
		return
	}
	for _, f := range frames {
		for _, fld := range f.Fields {
			if fld.Name == "time" {
				continue
			}
			if fld.Config == nil {
				fld.Config = &data.FieldConfig{}
			}
			fld.Config.DisplayName = format
		}
	}
}

// inferDecimals picks a sensible Field.Config.Decimals from the observed
// magnitude of the value series. Heuristic (matches what most operators
// hand-tune anyway):
//
//	|max abs val| < 1       → 4 decimals
//	|max abs val| < 100     → 3 decimals
//	|max abs val| < 10000   → 2 decimals
//	|max abs val| ≥ 10000   → 0 decimals
//
// Fields with Config.Decimals already set (e.g. via an override) are not
// touched. Fields with a unit hint like 'percent' get 1 by default.
func inferDecimals(frames []*data.Frame) {
	for _, f := range frames {
		for _, fld := range f.Fields {
			if fld.Name == "time" {
				continue
			}
			if fld.Config == nil {
				fld.Config = &data.FieldConfig{}
			}
			if fld.Config.Decimals != nil {
				continue
			}
			d := decimalsFor(fld)
			if d < 0 {
				continue
			}
			u16 := uint16(d)
			fld.Config.Decimals = &u16
		}
	}
}

func decimalsFor(fld *data.Field) int {
	switch fld.Config.Unit {
	case "percent":
		return 1
	}
	max := 0.0
	for i := 0; i < fld.Len(); i++ {
		v := fld.At(i)
		var f float64
		switch x := v.(type) {
		case float64:
			f = x
		case *float64:
			if x == nil {
				continue
			}
			f = *x
		default:
			continue
		}
		if math.IsNaN(f) {
			continue
		}
		if a := math.Abs(f); a > max {
			max = a
		}
	}
	switch {
	case max == 0:
		return -1
	case max < 1:
		return 4
	case max < 100:
		return 3
	case max < 10000:
		return 2
	default:
		return 0
	}
}
