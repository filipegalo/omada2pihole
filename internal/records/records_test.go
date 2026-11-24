package records

import (
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestSanitizeLabel(t *testing.T) {
	cases := map[string]string{
		"My iPhone":        "my-iphone",
		"  --Foo__Bar--  ": "foo-bar",
		"a...b":            "a-b",
		"Ünïcode":          "n-code",
		"!!!":              "",
		"":                 "",
		"ok-name1":         "ok-name1",
	}
	for in, want := range cases {
		if got := sanitizeLabel(in); got != want {
			t.Errorf("sanitizeLabel(%q)=%q want %q", in, got, want)
		}
	}
	long := strings.Repeat("a", 62) + "-b"
	got := sanitizeLabel(long)
	if got != strings.Repeat("a", 62) || len(got) > 63 {
		t.Errorf("long label not trimmed: %q", got)
	}
}

func TestNameSourcePriority(t *testing.T) {
	mac := "AA-BB-CC-DD-EE-FF"
	cases := []struct {
		c    Device
		want string
	}{
		{Device{Name: "Laptop", HostName: "host", MAC: mac}, "laptop"},
		{Device{Name: "AA-BB-CC-DD-EE-FF", HostName: "host", MAC: mac}, "host"},
		{Device{Name: "aa:bb:cc:dd:ee:ff", HostName: "", MAC: mac}, "aa-bb-cc-dd-ee-ff"},
		{Device{Name: "", HostName: "", MAC: mac}, "aa-bb-cc-dd-ee-ff"},
		{Device{Name: "!!!", HostName: "h", MAC: mac}, "h"},
	}
	for _, tc := range cases {
		if got := baseLabel(tc.c, netip.MustParseAddr("10.0.0.1")); got != tc.want {
			t.Errorf("baseLabel(%+v)=%q want %q", tc.c, got, tc.want)
		}
	}
}

func entries(rs []Record) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Entry())
	}
	return out
}

func TestBuildDesiredBasicsAndSkips(t *testing.T) {
	got, err := Build([]Device{
		{Name: "TV", MAC: "00-00-00-00-00-01", IP: "192.168.1.10"},
		{Name: "noip", MAC: "00-00-00-00-00-02", IP: ""},
		{Name: "v6", MAC: "00-00-00-00-00-03", IP: "fe80::1"},
		{Name: "zero", MAC: "00-00-00-00-00-04", IP: "0.0.0.0"},
	}, "lan")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"192.168.1.10 tv.lan"}; !reflect.DeepEqual(entries(got), want) {
		t.Fatalf("got %v want %v", entries(got), want)
	}
}

func TestDuplicateIPKeepsLowestMAC(t *testing.T) {
	got, _ := Build([]Device{
		{Name: "b", MAC: "00-00-00-00-00-02", IP: "10.0.0.5"},
		{Name: "a", MAC: "00-00-00-00-00-01", IP: "10.0.0.5"},
	}, "lan")
	if want := []string{"10.0.0.5 a.lan"}; !reflect.DeepEqual(entries(got), want) {
		t.Fatalf("got %v", entries(got))
	}
}

func TestDuplicateLabelsAllGetOctet(t *testing.T) {
	in := []Device{
		{Name: "Phone", MAC: "00-00-00-00-00-01", IP: "10.0.0.5"},
		{Name: "phone", MAC: "00-00-00-00-00-02", IP: "10.0.0.6"},
		{Name: "solo", MAC: "00-00-00-00-00-03", IP: "10.0.0.7"},
	}
	want := []string{"10.0.0.5 phone-5.lan", "10.0.0.6 phone-6.lan", "10.0.0.7 solo.lan"}
	got, err := Build(in, "lan")
	if err != nil || !reflect.DeepEqual(entries(got), want) {
		t.Fatalf("got %v err %v", entries(got), err)
	}
	// Order independence.
	in[0], in[2] = in[2], in[0]
	got2, _ := Build(in, "lan")
	if !reflect.DeepEqual(entries(got2), want) {
		t.Fatalf("order dependent: %v", entries(got2))
	}
}

func TestDuplicateStillCollidingFallsBackToMAC(t *testing.T) {
	// Same last octet in different subnets: octet suffix still collides.
	in := []Device{
		{Name: "x", MAC: "00-00-00-00-00-01", IP: "10.0.0.5"},
		{Name: "x", MAC: "00-00-00-00-00-02", IP: "10.0.1.5"},
	}
	got, err := Build(in, "lan")
	if err != nil {
		t.Fatal(err)
	}
	e := entries(got)
	if e[0] != "10.0.0.5 x-5-000000000001.lan" || e[1] != "10.0.1.5 x-5-000000000002.lan" {
		t.Fatalf("got %v", e)
	}
}

func TestLongDuplicateLabelTruncated(t *testing.T) {
	base := strings.Repeat("a", 63)
	got, err := Build([]Device{
		{Name: base, MAC: "00-00-00-00-00-01", IP: "10.0.0.5"},
		{Name: base, MAC: "00-00-00-00-00-02", IP: "10.0.0.6"},
	}, "lan")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		label := strings.TrimSuffix(r.Name, ".lan")
		if len(label) > 63 || !ValidLabel(label) {
			t.Errorf("bad label %q", label)
		}
	}
}

func TestIsOwned(t *testing.T) {
	owned := []string{"10.0.0.1 a.lan", "192.168.1.20 my-host.lan"}
	foreign := []string{
		"10.0.0.1 a.other",         // other suffix
		"10.0.0.1 a.lan b.lan",     // alias
		"10.0.0.1 a.b.lan",         // multi-label
		"10.0.0.1  a.lan",          // odd whitespace
		" 10.0.0.1 a.lan",          // leading whitespace
		"10.0.0.1 a.lan ",          // trailing whitespace
		"10.0.0.1\ta.lan",          // tab
		"10.0.0.1 a.lan # comment", // comment
		"# 10.0.0.1 a.lan",         // commented out
		"10.0.0.1 A.lan",           // uppercase not canonical
		"::1 a.lan",                // ipv6
		"010.0.0.1 a.lan",          // non-canonical ip
		"10.0.0.1 .lan",            // empty label
		"10.0.0.1 -a.lan",          // bad label
		"10.0.0.1 lan",             // no label
		"10.0.0.1",                 // one token
		"",                         // empty
	}
	for _, e := range owned {
		if !IsOwned(e, "lan") {
			t.Errorf("%q should be owned", e)
		}
	}
	for _, e := range foreign {
		if IsOwned(e, "lan") {
			t.Errorf("%q should be foreign", e)
		}
	}
}

func rec(ip, name string) Record { return Record{IP: ip, Name: name} }

func TestDiffBasic(t *testing.T) {
	desired := []Record{rec("10.0.0.1", "a.lan"), rec("10.0.0.2", "b.lan"), rec("10.0.0.3", "c.lan")}
	current := []string{"10.0.0.1 a.lan", "10.0.0.9 old.lan", "10.0.0.2 b-old.lan"}
	p, err := Diff(desired, current, "lan", false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.ToAdd, []string{"10.0.0.2 b.lan", "10.0.0.3 c.lan"}) ||
		!reflect.DeepEqual(p.ToRemove, []string{"10.0.0.2 b-old.lan", "10.0.0.9 old.lan"}) ||
		!reflect.DeepEqual(p.Unchanged, []string{"10.0.0.1 a.lan"}) {
		t.Fatalf("plan %+v", p)
	}
}

func TestDiffNeverRemovesForeign(t *testing.T) {
	foreign := []string{
		"10.0.0.50 nas.home",
		"10.0.0.51 a.lan b.lan",
		"10.0.0.52 a.b.lan",
		"10.0.0.53  spaced.lan",
		"10.0.0.54 hand.lan # my record",
		"# 10.0.0.55 commented.lan",
		"10.0.0.56\ttab.lan",
		"::1 v6.lan",
		"garbage",
		"",
	}
	desired := []Record{rec("10.0.0.1", "a.lan")}
	current := append([]string{"10.0.0.99 gone.lan"}, foreign...)
	p, err := Diff(desired, current, "lan", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range p.ToRemove {
		for _, f := range foreign {
			if r == f {
				t.Errorf("foreign entry %q scheduled for removal", r)
			}
		}
	}
	if !reflect.DeepEqual(p.ToRemove, []string{"10.0.0.99 gone.lan"}) {
		t.Fatalf("toRemove=%v", p.ToRemove)
	}
}

func TestDiffForeignConflictSkipsDesired(t *testing.T) {
	desired := []Record{
		rec("10.0.0.51", "x.lan"),  // foreign has this IP
		rec("10.0.0.7", "nas.lan"), // foreign has this name as alias
		rec("10.0.0.8", "ok.lan"),
	}
	current := []string{"10.0.0.51 hand.home", "10.0.0.60 foo.home NAS.lan # c"}
	p, err := Diff(desired, current, "lan", false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Skipped, []string{"10.0.0.51 x.lan", "10.0.0.7 nas.lan"}) &&
		!reflect.DeepEqual(p.Skipped, []string{"10.0.0.7 nas.lan", "10.0.0.51 x.lan"}) {
		t.Fatalf("skipped=%v", p.Skipped)
	}
	if !reflect.DeepEqual(p.ToAdd, []string{"10.0.0.8 ok.lan"}) {
		t.Fatalf("toAdd=%v", p.ToAdd)
	}
}

func TestDiffEmptyDesiredGuard(t *testing.T) {
	p, err := Diff(nil, []string{"10.0.0.1 a.lan", "10.0.0.2 keep.home"}, "lan", false)
	if err == nil {
		t.Fatal("expected error when desired is empty and removals exist")
	}
	_ = p
	// Nothing owned: no error, nothing to do.
	p, err = Diff(nil, []string{"10.0.0.2 keep.home"}, "lan", false)
	if err != nil || len(p.ToRemove) != 0 || len(p.ToAdd) != 0 {
		t.Fatalf("unexpected: %+v %v", p, err)
	}
}

func TestDiffPartialListGuard(t *testing.T) {
	var current []string
	for i := 1; i <= 20; i++ {
		current = append(current, fmt.Sprintf("10.0.0.%d h%d.lan", i, i))
	}
	desired := []Record{{IP: "10.0.0.1", Name: "h1.lan"}}
	if _, err := Diff(desired, current, "lan", false); err == nil {
		t.Fatal("removing 19 of 20 owned entries must be refused")
	}
	p, err := Diff(desired, current, "lan", true)
	if err != nil || len(p.ToRemove) != 19 {
		t.Fatalf("override: %v removals=%d", err, len(p.ToRemove))
	}
	// Small churn stays allowed: 5 removals of 20.
	desired = nil
	for i := 1; i <= 15; i++ {
		desired = append(desired, Record{IP: fmt.Sprintf("10.0.0.%d", i), Name: fmt.Sprintf("h%d.lan", i)})
	}
	if p, err := Diff(desired, current, "lan", false); err != nil || len(p.ToRemove) != 5 {
		t.Fatalf("churn: %v removals=%d", err, len(p.ToRemove))
	}
}

func TestBuildDesiredFallbacks(t *testing.T) {
	got, err := Build([]Device{
		{Name: "", HostName: "--", MAC: "", IP: "10.0.0.6"},
		{HostName: "laptop.home.lan", MAC: "aa-bb-cc-dd-ee-01", IP: "10.0.0.7"},
		// Same MAC listed twice (pages shifted): kept once.
		{Name: "tv", MAC: "aa-bb-cc-dd-ee-02", IP: "10.0.0.8"},
		{Name: "tv", MAC: "AA:BB:CC:DD:EE:02", IP: "10.0.0.9"},
	}, "lan")
	if err != nil {
		t.Fatal(err)
	}
	want := []Record{{"10.0.0.6", "ip-10-0-0-6.lan"}, {"10.0.0.7", "laptop.lan"}, {"10.0.0.8", "tv.lan"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
