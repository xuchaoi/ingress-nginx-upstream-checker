package app

import (
	"bytes"
	"encoding/json"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/cmd/upstream-checker/app/option"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/pkg/util"
	"io/ioutil"
	"k8s.io/klog"
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
			var f interface{}
			unmarshalErr := json.Unmarshal(data, &f)
			if unmarshalErr != nil {
				klog.Error(unmarshalErr)
				return unmarshalErr
			}

			var change bool
			oldBackends := f.([]interface{})
			backends := f.([]interface{})
			for i, backendi := range backends {
				backend := backendi.(map[string]interface{})
				if backend["endpoints"] == nil {
					continue
				}
				var healthEndpoints []map[string]interface{}
				endpoints := backend["endpoints"].([]interface{})
				for _, endpointi := range endpoints {
					endpoint := endpointi.(map[string]interface{})
					addr := endpoint["address"].(string)
					port := endpoint["port"].(string)
					epUrl := "http://" + addr + ":" + port
					epRes, err := util.HttpGet(epUrl)
					epRetry := 0

					//todo: response code judge...
					if err == nil {
						klog.V(4).Infof("Check endpoint success, url: %s, status: %s", epUrl, epRes.Status)
						healthEndpoints = append(healthEndpoints, endpoint)
					} else {
						for epRetry <= s.CheckRetry {
							_, err := util.HttpGet(epUrl)
							if err == nil {
								klog.V(4).Infof("Check endpoint success, url: %s, status: %s", epUrl, epRes.Status)
								healthEndpoints = append(healthEndpoints, endpoint)
								break
							} else {
								// todo: Do you need to sleep for one second, then check
								epRetry++
								klog.Errorf("Check endpoint failed, url: %s, err: v%", epUrl, err)
								continue
							}
						}
					}
				}
				if len(healthEndpoints) < len(endpoints) {
					backend["endpoints"] = healthEndpoints
					backends[i] = backend
					change = true
				}
			}

			klog.V(4).Infof("old backends data: %v", oldBackends)
			klog.V(4).Infof("new backends data: %v", backends)

			if change {
				buf, err := json.Marshal(backends)
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