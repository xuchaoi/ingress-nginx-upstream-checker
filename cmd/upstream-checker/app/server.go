package app

import (
	"bytes"
	"encoding/json"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/cmd/upstream-checker/app/option"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/pkg/util"
	"github.com/huandu/go-clone"
	"io/ioutil"
	"k8s.io/klog"
	"strings"
	"sync"
	"time"
)

func Run(s *option.ServerRunOptions) error {
	if !argCheck(s) {
		return nil
	}

	for {
		luaApiPorts := strings.Split(s.LuaApiPorts, ",")

		kubeRes, err := util.InsecureHttpsGet(s.KubeApiHealthUrl)
		if err != nil {
			klog.Errorln(err)
		}
		if err == nil && kubeRes.StatusCode == 200 {
			klog.V(2).Infof("Check API server status: %s", kubeRes.Status)

		} else {
			for _, luaApiPort := range luaApiPorts {
				go checker(s, luaApiPort)
			}
		}

		time.Sleep(time.Duration(s.CheckCycle) * time.Second)
	}
}

func checker(s *option.ServerRunOptions, luaApiPort string) error {
	luaApiBackendUrl := "http://127.0.0.1:" + luaApiPort + "/configuration/backends"
	luaRes, err := util.HttpGet(luaApiBackendUrl)
	if err != nil {
		klog.Errorln(err)
		return err
	}
	if luaRes.StatusCode != 200 {
		klog.Errorf("[%s] Get backends by lua API err, code: %d", luaApiPort, luaRes.StatusCode)
	}
	defer luaRes.Body.Close()
	data, err := ioutil.ReadAll(luaRes.Body)
	if err != nil {
		klog.Error(err)
	}
	rollbackShell := "curl -X POST --header \"content-type: application/json\" --data '" + string(data[:]) + "'" + luaApiBackendUrl
	klog.V(2).Infof("[Rollback:%s] If you want to restore, please execute the command: %s", luaApiPort, rollbackShell)
	var f interface{}
	unmarshalErr := json.Unmarshal(data, &f)
	if unmarshalErr != nil {
		klog.Error(unmarshalErr)
		return unmarshalErr
	}
	// Multi-threaded health check request
	wg := sync.WaitGroup{}
	var change bool
	oldBackends := f.([]interface{})
	newBackends := clone.Clone(oldBackends).([]interface{})
	for _, backendi := range newBackends {
		backend := backendi.(map[string]interface{})
		if backend["endpoints"] == nil || len(backend["endpoints"].([]interface{})) == 0 {
			continue
		}
		//var healthEndpoints []map[string]interface{}
		endpoints := backend["endpoints"].([]interface{})
		wg.Add(len(endpoints))
		for j, endpointi := range endpoints {
			endpoint := endpointi.(map[string]interface{})
			addr := endpoint["address"].(string)
			port := endpoint["port"].(string)
			if addr == "" || port == "" {
				klog.V(4).Infof("[%s] Endpoint arg error, address: %s, port: %s", luaApiPort, addr, port)
				wg.Done()
				continue
			}
			epUrl := "http://" + addr + ":" + port
			go func(epUrl string, endpoints []interface{}, j int) {
				epRes, err := util.HttpGet(epUrl)
				epRetry := 1

				//todo: response code judge...
				if err == nil {
					klog.V(4).Infof("[%s] Check endpoint success, url: %s, status: %s", luaApiPort, epUrl, epRes.Status)
					//healthEndpoints = append(healthEndpoints, endpoint)
				} else {
					klog.V(4).Infof("[%s] Check endpoint failed, url: %s, err: %v", luaApiPort, epUrl, err)
					for epRetry <= s.CheckRetry {
						_, err := util.HttpGet(epUrl)
						if err == nil {
							klog.V(4).Infof("[%s] Retry check endpoint success, url: %s, retry: %d", luaApiPort, epUrl, epRetry)
							//healthEndpoints = append(healthEndpoints, endpoint)
							break
						} else {
							// todo: Do you need to sleep for one second, then check
							klog.Errorf("[%s] Retry check endpoint failed, url: %s, err: %s, retry: %d", luaApiPort, epUrl, err.Error(), epRetry)
							epRetry++
							continue
						}
					}
					if epRetry > s.CheckRetry {
						tmpEp := make(map[string]interface{})
						tmpEp["address"] = ""
						tmpEp["port"] = ""
						endpoints[j] = tmpEp
						change = true
					}
				}
				wg.Done()
			}(epUrl, endpoints, j)
		}
	}
	wg.Wait()
	klog.V(6).Infof("[%s] old backends data: %v", luaApiPort, oldBackends)
	klog.V(6).Infof("[%s] new backends data: %v", luaApiPort, newBackends)
	if change {
		for i, backendi := range newBackends {
			backend := backendi.(map[string]interface{})
			var healthEndpoints []map[string]interface{}
			if backend["endpoints"] == nil || len(backend["endpoints"].([]interface{})) == 0 {
				continue
			}
			endpoints := backend["endpoints"].([]interface{})
			for _, endpointi := range endpoints {
				endpoint := endpointi.(map[string]interface{})
				addr := endpoint["address"].(string)
				port := endpoint["port"].(string)
				if addr == "" || port == "" {
					continue
				}
				healthEndpoints = append(healthEndpoints, endpoint)
			}
			if healthEndpoints == nil || backend["endpoints"] == nil {
				delete(backend, "endpoints")
			} else {
				backend["endpoints"] = healthEndpoints
			}
			newBackends[i] = backend
		}
		klog.V(6).Infof("[%s] After data cleaning, new backends data: %v", luaApiPort, newBackends)
		buf, err := json.Marshal(newBackends)
		if err != nil {
			klog.Errorf("[%s] Convert the backends to byte through json tool failed, err: %v", luaApiPort, err)
		}
		_, err = util.HttpPost(luaApiBackendUrl, bytes.NewReader(buf))
		if err != nil {
			klog.Errorf("[%s] Update backends by lua API failed, err: %v", luaApiPort, err)
		}

		if err != nil {
			klog.Errorf("[%s] Get update backends result failed, err: %v", luaApiPort, err)
		}
		klog.V(2).Infof("[%s] Update backends by lua API succeed", luaApiPort)

		change = false
	} else {
		klog.V(2).Infof("[%s] Upstream backends is health, backends no change", luaApiPort)
	}
	return nil
}

func argCheck(s *option.ServerRunOptions) bool {
	checkStatus := true
	if s.KubeApiHealthUrl == "" {
		klog.Error("Arg: KubeApiHealthUrl, is required!")
		checkStatus = false
	}
	if s.LuaApiPorts == "" {
		klog.Error("Arg: LuaApiPorts, is required!")
		checkStatus = false
	}
	return checkStatus
}