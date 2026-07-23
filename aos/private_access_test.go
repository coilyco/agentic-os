package main

import "testing"

func TestPrivateAccessIsLimitedToLiveObserveRoles(t *testing.T) {
	t.Setenv(privateNetworkEnv, "private-network")
	t.Setenv(privateProxyEnv, "socks5h://private-proxy:1055")
	for _, role := range []string{"advisor", "director"} {
		config, err := privateAccessForRole(role)
		if err != nil {
			t.Fatalf("privateAccessForRole(%q): %v", role, err)
		}
		if config == nil || config.Network != "private-network" {
			t.Fatalf("privateAccessForRole(%q) = %#v, want configured access", role, config)
		}
	}
	for _, role := range []string{"engineer", "ops", "pm", "qa"} {
		config, err := privateAccessForRole(role)
		if err != nil {
			t.Fatalf("privateAccessForRole(%q): %v", role, err)
		}
		if config != nil {
			t.Fatalf("privateAccessForRole(%q) = %#v, want nil", role, config)
		}
	}
}

func TestPrivateAccessFailsClosedWhenLiveObserveConfigIsAbsent(t *testing.T) {
	t.Setenv(privateNetworkEnv, "")
	t.Setenv(privateProxyEnv, "")
	if _, err := privateAccessForRole("director"); err == nil {
		t.Fatal("director private access accepted missing configuration")
	}
}

func TestPrivateAccessRequiresRemoteDNSProxy(t *testing.T) {
	t.Setenv(privateNetworkEnv, "private-network")
	t.Setenv(privateProxyEnv, "socks5://private-proxy:1055")
	if _, err := privateAccessForRole("advisor"); err == nil {
		t.Fatal("advisor private access accepted a proxy without remote DNS")
	}
}
