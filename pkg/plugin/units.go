package plugin

import (
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// dqlToGrafanaUnit maps the Dynatrace unit strings we've seen in real Grail
// responses to the Grafana unit identifier (the suffix Grafana uses in
// `fieldConfig.unit`). Unknown values fall through to "" which is rendered as
// no unit.
var dqlToGrafanaUnit = map[string]string{
	// time
	"Nanosecond":  "ns",
	"Microsecond": "µs",
	"Millisecond": "ms",
	"Second":      "s",
	"Minute":      "m",
	"Hour":        "h",
	"Day":         "d",
	// bytes
	"Byte":     "bytes",
	"Kilobyte": "deckbytes",
	"Megabyte": "decmbytes",
	"Gigabyte": "decgbytes",
	"Terabyte": "dectbytes",
	"KibiByte": "kbytes",
	"MebiByte": "mbytes",
	"GibiByte": "gbytes",
	// per second
	"PerSecond":      "ops",
	"BytePerSecond":  "Bps",
	"BitPerSecond":   "bps",
	"PacketPerSecond": "pps",
	// percent / ratio
	"Percent": "percent",
	"Ratio":   "percentunit",
	// power / electrical
	"Watt":          "watt",
	"Kilowatt":      "kwatt",
	"WattHour":      "watth",
	"KilowattHour":  "kwatth",
	"Volt":          "volt",
	"Ampere":        "amp",
	"VoltAmpere":    "voltamp",
	// temperature
	"DegreeCelsius":    "celsius",
	"DegreeFahrenheit": "fahrenheit",
	"Kelvin":           "kelvin",
	// counts
	"Count":      "short",
	"Connection": "short",
	"Request":    "short",
	"Operation":  "short",
}

// grafanaUnit best-effort maps a DQL unit literal to the Grafana unit token.
// It accepts the literal as Dynatrace writes it ("Kilowatt"), lowercased
// ("kilowatt"), or with common abbreviation ("kW", "kWh") since the unit
// column in real responses is inconsistent.
func grafanaUnit(s string) string {
	if s == "" {
		return ""
	}
	if v, ok := dqlToGrafanaUnit[s]; ok {
		return v
	}
	// abbreviations seen in loxone data
	switch strings.ToLower(s) {
	case "kw":
		return "kwatt"
	case "kwh":
		return "kwatth"
	case "w":
		return "watt"
	case "wh":
		return "watth"
	case "v":
		return "volt"
	case "a":
		return "amp"
	case "%":
		return "percent"
	case "°c", "c":
		return "celsius"
	case "ppm":
		return "ppm"
	}
	return ""
}

// applyFieldConfig sets unit + display name on the given field based on the
// value-column's labels (e.g. control.name -> legend). It is safe to call on
// any data.Field; unknown inputs leave the config untouched.
func applyFieldConfig(f *data.Field, labels data.Labels) {
	if f == nil {
		return
	}
	unit := pickUnit(f.Name, labels)
	if unit != "" {
		if f.Config == nil {
			f.Config = &data.FieldConfig{}
		}
		f.Config.Unit = unit
	}
	if name := preferredDisplayName(f.Name, labels); name != "" {
		if f.Config == nil {
			f.Config = &data.FieldConfig{}
		}
		f.Config.DisplayNameFromDS = name
	}
}

// pickUnit looks for an explicit `unit` label on the series (Loxone DQL
// responses carry it), otherwise tries to infer from the field name.
func pickUnit(fieldName string, labels data.Labels) string {
	if labels != nil {
		if u, ok := labels["unit"]; ok {
			if g := grafanaUnit(u); g != "" {
				return g
			}
		}
		if u, ok := labels["value.unit"]; ok {
			if g := grafanaUnit(u); g != "" {
				return g
			}
		}
	}
	lower := strings.ToLower(fieldName)
	switch {
	case strings.Contains(lower, "percent"), strings.HasSuffix(lower, "_pct"):
		return "percent"
	case strings.Contains(lower, "bytes"):
		return "bytes"
	case strings.Contains(lower, "ms_"), strings.HasSuffix(lower, "_ms"):
		return "ms"
	case strings.HasSuffix(lower, "_kw"), strings.Contains(lower, "kilowatt"):
		return "kwatt"
	}
	return ""
}

// preferredDisplayName builds a legend string from the most relevant label.
// For Loxone series the most useful label is `control.name`; for entity
// metrics it's `host.name` / `service.name` (or the `name` field emitted by
// `smartscapeNodes` results). `dt.smartscape.host` / `.service` carry the
// entity ID — used as a last-resort label when no human name is available.
// Falls back to empty (let Grafana use its default).
func preferredDisplayName(fieldName string, labels data.Labels) string {
	if labels == nil {
		return ""
	}
	for _, key := range []string{
		"control.name",
		"host.name",
		"service.name",
		"k8s.namespace.name",
		"name",
		"dt.smartscape.host",
		"dt.smartscape.service",
	} {
		if v, ok := labels[key]; ok && v != "" {
			return v
		}
	}
	return ""
}
