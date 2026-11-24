package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func baseEnv() map[string]string {
	return map[string]string{
		"OMADA_URL": "https://omada.local:8043/", "OMADA_ID": "id", "OMADA_CLIENT_ID": "cid",
		"OMADA_CLIENT_SECRET": "s3cret", "PIHOLE_URL": "https://pi.hole", "PIHOLE_PASSWORD": "pw-secret",
	}
}

func TestConfigRequired(t *testing.T) {
	_, err := Load(env(nil), 0, false)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, k := range []string{"OMADA_URL", "OMADA_ID", "OMADA_CLIENT_ID", "OMADA_CLIENT_SECRET", "PIHOLE_URL"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("missing %s in %v", k, err)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	c, err := Load(env(baseEnv()), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.OmadaSite != "Default" || c.DomainSuffix != "lan" || c.Interval != 5*time.Minute || c.DryRun ||
		c.OmadaURL != "https://omada.local:8043" {
		t.Fatalf("%+v", c)
	}
}

func TestConfigBadValues(t *testing.T) {
	cases := map[string]string{
		"DRY_RUN": "maybe", "SYNC_INTERVAL": "10s", "DOMAIN_SUFFIX": "bad suffix", "LOG_LEVEL": "trace",
		"OMADA_TLS_SKIP_VERIFY": "x",
	}
	for k, v := range cases {
		e := baseEnv()
		e[k] = v
		if _, err := Load(env(e), 0, false); err == nil {
			t.Errorf("%s=%s should fail", k, v)
		}
	}
	e := baseEnv()
	e["SYNC_INTERVAL"] = "notaduration"
	if _, err := Load(env(e), 0, false); err == nil {
		t.Error("bad duration should fail")
	}
}

func TestConfigMutuallyExclusiveTLS(t *testing.T) {
	e := baseEnv()
	e["OMADA_TLS_SKIP_VERIFY"], e["OMADA_CA_FILE"] = "true", "/ca.pem"
	if _, err := Load(env(e), 0, false); err == nil {
		t.Error("expected error")
	}
	e = baseEnv()
	e["PIHOLE_TLS_SKIP_VERIFY"], e["PIHOLE_CA_FILE"] = "true", "/ca.pem"
	if _, err := Load(env(e), 0, false); err == nil {
		t.Error("expected error")
	}
}

func TestConfigSuffixAndIntervalOverride(t *testing.T) {
	e := baseEnv()
	e["DOMAIN_SUFFIX"] = ".Omada.LAN."
	c, err := Load(env(e), time.Minute, false)
	if err != nil || c.DomainSuffix != "omada.lan" || c.Interval != time.Minute {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := Load(env(baseEnv()), 5*time.Second, false); err == nil {
		t.Error("override below minimum should fail")
	}
}

func TestConfigRedaction(t *testing.T) {
	c, _ := Load(env(baseEnv()), 0, false)
	s := c.String() + c.LogValue().String()
	if strings.Contains(s, "s3cret") || strings.Contains(s, "pw-secret") {
		t.Fatalf("secret leaked: %s", s)
	}
}

func TestConfigRejectsURLExtras(t *testing.T) {
	for _, u := range []string{"https://user:pw@pi.hole", "https://pi.hole/admin", "https://pi.hole?x=1"} {
		e := baseEnv()
		e["PIHOLE_URL"] = u
		_, err := Load(env(e), 0, false)
		if err == nil {
			t.Errorf("%s accepted", u)
		} else if strings.Contains(err.Error(), "pw@") {
			t.Errorf("credential echoed: %v", err)
		}
	}
}

func TestConfigOnceIgnoresIntervalMinimum(t *testing.T) {
	e := baseEnv()
	e["SYNC_INTERVAL"] = "10s"
	if _, err := Load(env(e), 0, true); err != nil {
		t.Fatal(err)
	}
}
