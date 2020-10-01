package app

import (
	"bytes"
	"encoding/json"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/cmd/upstream-checker/app/option"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/pkg/util"
	"github.com/huandu/go-clone"
	"io/ioutil"
	"k8s.io/klog"
	"sync"
	"time"
)

func Run(s *option.ServerRunOptions) error {
	for {
		kubeRes, err := util.InsecureHttpsGet(s.KubeApiHealthUrl)
		if err != nil {
			klog.Errorln(err)
		}
		if err == nil && kubeRes.StatusCode == 200 {
			klog.V(2).Infof("Check API server status: %s", kubeRes.Status)

		} else {
			luaRes, err := util.HttpGet(s.LuaApiBackendUrl)
			if err != nil {
				klog.Errorln(err)
				return err
			}
			if luaRes.StatusCode != 200 {
				klog.Errorf("Get backends by lua API err, code: %d", luaRes.StatusCode)
			}
			defer luaRes.Body.Close()
			data, err := ioutil.ReadAll(luaRes.Body)
			if err != nil {
				klog.Error(err)
			}
			rollbackShell := "curl -X POST --header \"content-type: application/json\" --data '" + string(data[:]) + "' http://ng_addr:status_port/configuration/backends"
			klog.V(2).Infof("[Rollback] If you want to restore, please execute the command: %s", rollbackShell)
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
						klog.V(4).Infof("Endpoint arg error, address: %s, port: %s", addr, port)
						wg.Done()
						continue
					}
					epUrl := "http://" + addr + ":" + port
					go func(epUrl string, endpoints []interface{}, j int) {
						epRes, err := util.HttpGet(epUrl)
						epRetry := 1

						//todo: response code judge...
						if err == nil {
							klog.V(4).Infof("Check endpoint success, url: %s, status: %s", epUrl, epRes.Status)
							//healthEndpoints = append(healthEndpoints, endpoint)
						} else {
							for epRetry <= s.CheckRetry {
								_, err := util.HttpGet(epUrl)
								if err == nil {
									klog.V(4).Infof("Retry check endpoint success, url: %s, retry: %d", epUrl, epRetry)
									//healthEndpoints = append(healthEndpoints, endpoint)
									break
								} else {
									// todo: Do you need to sleep for one second, then check
									klog.Errorf("Retry check endpoint failed, url: %s, err: %s, retry: %d", epUrl, err.Error(), epRetry)
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
			klog.V(4).Infof("old backends data: %v", oldBackends)
			klog.V(4).Infof("new backends data: %v", newBackends)
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
				klog.V(4).Infof("After data cleaning, new backends data: %v", newBackends)
				buf, err := json.Marshal(newBackends)
				if err != nil {
					klog.Errorf("Convert the backends to byte through json tool failed, err: %v", err)
				}
				_, err = util.HttpPost(s.LuaApiBackendUrl, bytes.NewReader(buf))
				if err != nil {
					klog.Errorf("Update backends by lua API failed, err: %v", err)
				}

				if err != nil {
					klog.Errorf("Get update backends result failed, err: %v", err)
				}
				klog.V(2).Infof("Update backends by lua API succeed")

				change = false
			} else {
				klog.V(2).Infof("Upstream backends is health, backends no change")
			}
		}
		time.Sleep(time.Duration(s.CheckCycle) * time.Second)
	}

}