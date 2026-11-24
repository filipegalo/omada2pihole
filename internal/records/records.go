// Package records turns Omada clients into Pi-hole dns.hosts entries and
// computes the per-entry diff. It is pure logic with no I/O.
package records

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// Device is the subset of an Omada client record that the sync uses.
type Device struct {
	Name     string // user alias (Omada defaults it to the MAC)
	HostName string
	MAC      string
	IP       string
}

// Record is one desired DNS host entry.
type Record struct {
	IP   string
	Name string // fully qualified: <label>.<suffix>
}

// Entry returns the canonical Pi-hole dns.hosts string.
func (r Record) Entry() string { return r.IP + " " + r.Name }

const maxLabel = 63

// stripMACSeparators lowercases and removes ':', '-' and '.' separators.
func stripMACSeparators(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer(":", "", "-", "", ".", "").Replace(s)
}

func isHex12(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// sanitizeLabel turns arbitrary text into a DNS label (possibly empty).
func sanitizeLabel(s string) string {
	var b strings.Builder
	lastDash := false
	for _, c := range strings.ToLower(s) {
		ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if ok {
			b.WriteRune(c)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return truncLabel(strings.Trim(b.String(), "-"), maxLabel)
}

// truncLabel cuts to n bytes (labels are ASCII) and trims trailing dashes.
func truncLabel(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	return strings.TrimRight(s, "-")
}

// macLabel derives aa-bb-cc-dd-ee-ff from a MAC, or a sanitised fallback.
func macLabel(mac string) string {
	h := stripMACSeparators(mac)
	if isHex12(h) {
		parts := make([]string, 6)
		for i := range parts {
			parts[i] = h[i*2 : i*2+2]
		}
		return strings.Join(parts, "-")
	}
	return sanitizeLabel(mac)
}

// baseLabel picks name -> hostName -> MAC -> IP, first one that sanitises non-empty.
func baseLabel(c Device, ip netip.Addr) string {
	macNorm := stripMACSeparators(c.MAC)
	if n := strings.TrimSpace(c.Name); n != "" && stripMACSeparators(n) != macNorm {
		if l := sanitizeLabel(n); l != "" {
			return l
		}
	}
	// A device-reported host name may be an FQDN (laptop.home.lan): keep the host part.
	host, _, _ := strings.Cut(strings.TrimSpace(c.HostName), ".")
	if l := sanitizeLabel(host); l != "" {
		return l
	}
	if l := macLabel(c.MAC); l != "" {
		return l
	}
	return "ip-" + strings.ReplaceAll(ip.String(), ".", "-")
}

func parseIPv4(s string) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil || !a.Is4() || a.IsUnspecified() {
		return netip.Addr{}, false
	}
	return a, true
}

func withSuffix(base, extra string) string {
	maxBase := maxLabel - len(extra) - 1
	return truncLabel(base, maxBase) + "-" + extra
}

// Build returns the deterministic desired record set for the given devices.
func Build(clients []Device, suffix string) ([]Record, error) {
	type item struct {
		mac   string
		ip    netip.Addr
		label string
	}
	var items []item
	for _, c := range clients {
		ip, ok := parseIPv4(c.IP)
		if !ok {
			continue
		}
		items = append(items, item{mac: stripMACSeparators(c.MAC), ip: ip, label: baseLabel(c, ip)})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].mac != items[j].mac {
			return items[i].mac < items[j].mac
		}
		return items[i].ip.Less(items[j].ip)
	})
	// Same IP twice: first (by MAC) wins. Same MAC twice (e.g. pages shifted
	// while paginating): lowest IP wins.
	seenIP := map[netip.Addr]bool{}
	seenMAC := map[string]bool{}
	uniq := items[:0]
	for _, it := range items {
		if seenIP[it.ip] || (it.mac != "" && seenMAC[it.mac]) {
			continue
		}
		seenIP[it.ip] = true
		seenMAC[it.mac] = true
		uniq = append(uniq, it)
	}
	items = uniq

	count := func() map[string]int {
		m := map[string]int{}
		for _, it := range items {
			m[it.label]++
		}
		return m
	}
	// Stage 1: every member of a duplicate group gets the last octet.
	cnt := count()
	for i := range items {
		if cnt[items[i].label] > 1 {
			oct := items[i].ip.As4()
			items[i].label = withSuffix(items[i].label, fmt.Sprint(oct[3]))
		}
	}
	// Stage 2: still colliding -> mac hex without separators.
	cnt = count()
	for i := range items {
		if cnt[items[i].label] > 1 {
			items[i].label = withSuffix(items[i].label, items[i].mac)
		}
	}
	// Stage 3: still colliding -> full dashed IP (unique per IP).
	cnt = count()
	for i := range items {
		if cnt[items[i].label] > 1 {
			items[i].label = withSuffix(items[i].label, strings.ReplaceAll(items[i].ip.String(), ".", "-"))
		}
	}
	// Final assertion.
	seen := map[string]bool{}
	out := make([]Record, 0, len(items))
	for _, it := range items {
		if !ValidLabel(it.label) || seen[it.label] {
			return nil, fmt.Errorf("internal error: cannot build unique label for %s (%q)", it.ip, it.label)
		}
		seen[it.label] = true
		out = append(out, Record{IP: it.ip.String(), Name: it.label + "." + suffix})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Entry() < out[j].Entry() })
	return out, nil
}

// ValidLabel reports whether l is a valid lowercase DNS label.
func ValidLabel(l string) bool {
	if l == "" || len(l) > maxLabel || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for _, c := range l {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// IsOwned reports whether entry is exactly "<ipv4> <label>.<suffix>" in
// canonical form. Everything else is foreign and must never be touched.
func IsOwned(entry, suffix string) bool {
	f := strings.Fields(entry)
	if len(f) != 2 {
		return false
	}
	ip, ok := parseIPv4(f[0])
	if !ok || ip.String() != f[0] {
		return false
	}
	label, found := strings.CutSuffix(f[1], "."+suffix)
	if !found || !ValidLabel(label) {
		return false
	}
	return entry == f[0]+" "+f[1]
}

// foreignTokens extracts (ip, lowercase names) from a foreign entry,
// ignoring comments.
func foreignTokens(entry string) (string, []string) {
	if i := strings.Index(entry, "#"); i >= 0 {
		entry = entry[:i]
	}
	f := strings.Fields(entry)
	if len(f) == 0 {
		return "", nil
	}
	names := make([]string, 0, len(f)-1)
	for _, n := range f[1:] {
		names = append(names, strings.ToLower(n))
	}
	return f[0], names
}

// Plan is the result of diffing desired against current.
type Plan struct {
	ToAdd     []string
	ToRemove  []string
	Unchanged []string
	Skipped   []string // desired entries skipped due to foreign conflicts
}

// massDeleteMin is the number of removals above which the proportional
// mass-delete guard applies.
const massDeleteMin = 5

// Diff computes the per-entry operations. It never selects a foreign entry
// for removal. Unless allowMassDelete is set, it refuses plans that would
// remove every owned entry, or more than half of them (and more than
// massDeleteMin), since that usually means Omada returned a partial list.
func Diff(desired []Record, current []string, suffix string, allowMassDelete bool) (Plan, error) {
	var p Plan
	cur := map[string]bool{}
	foreignIPs := map[string]bool{}
	foreignNames := map[string]bool{}
	for _, e := range current {
		if IsOwned(e, suffix) {
			cur[e] = true
			continue
		}
		ip, names := foreignTokens(e)
		if ip != "" {
			foreignIPs[ip] = true
		}
		for _, n := range names {
			foreignNames[n] = true
		}
	}
	// Raw current set, for exact-string matches (owned ones only count here).
	want := map[string]bool{}
	for _, r := range desired {
		want[r.Entry()] = true
	}
	for _, r := range desired {
		e := r.Entry()
		if cur[e] {
			p.Unchanged = append(p.Unchanged, e)
			continue
		}
		if foreignIPs[r.IP] || foreignNames[strings.ToLower(r.Name)] {
			p.Skipped = append(p.Skipped, e)
			continue
		}
		p.ToAdd = append(p.ToAdd, e)
	}
	for e := range cur {
		if !want[e] {
			p.ToRemove = append(p.ToRemove, e)
		}
	}
	sort.Strings(p.ToAdd)
	sort.Strings(p.ToRemove)
	sort.Strings(p.Unchanged)
	sort.Strings(p.Skipped)
	if allowMassDelete || len(p.ToRemove) == 0 {
		return p, nil
	}
	if len(desired) == 0 {
		return p, fmt.Errorf("refusing to remove %d owned entries: Omada returned no usable clients (set ALLOW_MASS_DELETE=true for one run if intended)", len(p.ToRemove))
	}
	if len(p.ToRemove) > massDeleteMin && 2*len(p.ToRemove) > len(cur) {
		return p, fmt.Errorf("refusing to remove %d of %d owned entries: Omada likely returned a partial client list (set ALLOW_MASS_DELETE=true for one run if intended)", len(p.ToRemove), len(cur))
	}
	return p, nil
}
