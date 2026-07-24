package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	privateNetworkEnv = "AOS_PRIVATE_NETWORK"
	privateProxyEnv   = "AOS_PRIVATE_PROXY"
)

type privateAccessConfig struct {
	Network string
	Proxy   string
}

var privateProxyEnvironmentNames = []string{
	"ALL_PROXY",
	"all_proxy",
	"HTTP_PROXY",
	"http_proxy",
	"HTTPS_PROXY",
	"https_proxy",
}

func privateAccessForRole(role string) (*privateAccessConfig, error) {
	if role != "advisor" && role != "director" {
		return nil, nil
	}
	network := strings.TrimSpace(os.Getenv(privateNetworkEnv))
	proxy := strings.TrimSpace(os.Getenv(privateProxyEnv))
	if network == "" || proxy == "" {
		return nil, fmt.Errorf(
			"role %s needs %s and %s for private live observation",
			role,
			privateNetworkEnv,
			privateProxyEnv,
		)
	}
	parsed, err := url.Parse(proxy)
	if err != nil || parsed.Scheme != "socks5h" || parsed.Host == "" {
		return nil, fmt.Errorf("%s must be a socks5h URL with a host", privateProxyEnv)
	}
	return &privateAccessConfig{Network: network, Proxy: proxy}, nil
}

func (config privateAccessConfig) environment() []string {
	environment := make([]string, 0, len(privateProxyEnvironmentNames))
	for _, name := range privateProxyEnvironmentNames {
		environment = append(environment, name+"="+config.Proxy)
	}
	return environment
}

func (config privateAccessConfig) environmentNames() []string {
	return append([]string(nil), privateProxyEnvironmentNames...)
}
