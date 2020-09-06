package option

import (
	"github.com/spf13/pflag"
	"strconv"
)

const (
	defaultKubeApiHealthUrl = "https://127.0.0.1:6443/healthz"
	defaultLuaApiBackendUrl = "http://127.0.0.1:10246/configuration/backends"
	defaultCheckCycle       = 30
	defaultCheckRetry       = 2
)

type ServerRunOptions struct {
	KubeApiHealthUrl string
	LuaApiBackendUrl string
	CheckCycle       int
	CheckRetry       int
}

func NewServerRunOptions() *ServerRunOptions {
	s := ServerRunOptions{
		KubeApiHealthUrl: defaultKubeApiHealthUrl,
		LuaApiBackendUrl: defaultLuaApiBackendUrl,
		CheckCycle      : defaultCheckCycle,
		CheckRetry      : defaultCheckRetry,
	}
	return &s
}

func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.KubeApiHealthUrl, "kubeApiHealthUrl", s.KubeApiHealthUrl,
		"The k8s API server health check url, default: " +defaultKubeApiHealthUrl)
	fs.StringVar(&s.LuaApiBackendUrl, "luaApiBackendUrl", s.LuaApiBackendUrl,
		"The ingress-nginx lua get/update backend API url, default: " +defaultLuaApiBackendUrl)
	fs.IntVar(&s.CheckCycle, "checkCycle", s.CheckCycle,
		"The upstream-checker check cycle, default: " + strconv.FormatInt(defaultCheckCycle, 10) + "s")
	fs.IntVar(&s.CheckRetry, "checkRetry", s.CheckRetry,
		"The upstream-checker check retry, default: " + strconv.FormatInt(defaultCheckRetry, 10))
}
