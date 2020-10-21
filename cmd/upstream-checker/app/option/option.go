package option

import (
	"github.com/spf13/pflag"
	"strconv"
)

const (
	defaultKubeApiHealthUrl = "https://127.0.0.1:6443/healthz"
	defaultLuaApiPorts      = "10246" //"http://127.0.0.1:10246/configuration/backends"
	defaultCheckCycle       = 15
	defaultCheckRetry       = 2
)

type ServerRunOptions struct {
	KubeApiHealthUrl string
	LuaApiPorts      string
	CheckCycle       int
	CheckRetry       int
}

func NewServerRunOptions() *ServerRunOptions {
	s := ServerRunOptions{
		KubeApiHealthUrl : defaultKubeApiHealthUrl,
		LuaApiPorts      : defaultLuaApiPorts,
		CheckCycle       : defaultCheckCycle,
		CheckRetry       : defaultCheckRetry,
	}
	return &s
}

func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.KubeApiHealthUrl, "kubeApiHealthUrl", s.KubeApiHealthUrl,
		"The k8s API server health check url, default: " +defaultKubeApiHealthUrl)
	fs.StringVar(&s.LuaApiPorts, "luaApiPorts", s.LuaApiPorts,
		"The ingress-nginx lua get/update backend API url, default: " + defaultLuaApiPorts)
	fs.IntVar(&s.CheckCycle, "checkCycle", s.CheckCycle,
		"The upstream-checker check cycle, default: " + strconv.FormatInt(defaultCheckCycle, 10) + "s")
	fs.IntVar(&s.CheckRetry, "checkRetry", s.CheckRetry,
		"The upstream-checker check retry, default: " + strconv.FormatInt(defaultCheckRetry, 10))
}
