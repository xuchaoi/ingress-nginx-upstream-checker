package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"time"
)

func main() {
	for {
		kubeApiHealthUrl      := "https://192.168.56.6:6443/healthz"
		luaApiBackendsUrl  := "http://127.0.0.1:10246/configuration/backends"

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		kubeClient := http.Client{Transport: tr}
		kubeRes, err := kubeClient.Get(kubeApiHealthUrl)
		if err != nil {
			klog.Error(err)
		}

		if err == nil && kubeRes.StatusCode == 200 {
			klog.Infof("Check API server status: %s", kubeRes.Status)
		} else {
			luaClient := http.Client{}
			luaRes, err := luaClient.Get(luaApiBackendsUrl)
			if err != nil {
				klog.Error(err)
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
				fmt.Println(unmarshalErr)
				return
			}

			var change bool
			oldBackends := f.([]interface{})
			backends := f.([]interface{})
			for i, backendi := range backends {
				backend := backendi.(map[string]interface{})

				var healthEndpoints []map[string]interface{}
				endpoints := backend["endpoints"].([]interface{})
				for _, endpointi := range endpoints {
					endpoint := endpointi.(map[string]interface{})
					addr := endpoint["address"].(string)
					port := endpoint["port"].(string)
					epUrl := "http://" + addr + ":" + port

					tmpRes, err := luaClient.Get(epUrl)
					if err != nil {
						klog.Errorf("Check endpoint url: %s, err: v%", epUrl, err)
						continue
					}

					if tmpRes.StatusCode == 404 || tmpRes.StatusCode == 200 {
						klog.Infof("Check endpoint url: %s, status: %s", epUrl, tmpRes.Status)
						healthEndpoints = append(healthEndpoints, endpoint)
					} else {
						//todo: delete ep, update endpoint
						klog.Error("Check endpoint url: %s, status: %s", epUrl, tmpRes.Status)
					}
				}
				if len(healthEndpoints) < len(endpoints) {
					backend["endpoints"] = healthEndpoints
					backends[i] = backend
					change = true
				}
			}
			klog.Infof("old backends data: %v", oldBackends)
			if change {
				buf, err := json.Marshal(backends)
				if err != nil {
					klog.Errorf("Convert the backends to byte through json tool failed, err: %v", err)
				}
				res, err := luaClient.Post(luaApiBackendsUrl, "application/json", bytes.NewReader(buf))
				if err != nil {
					klog.Errorf("Update backends by lua API failed, err: %v", err)
				}
				defer res.Body.Close()
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					klog.Errorf("Get update backends result failed, err: %v", err)
				}
				klog.Infof("Update backends by lua API succeed, detail: %v", body)

				change = false
			} else {
				klog.Infof("Upstream backends is health, backends no change")
			}
		}

		time.Sleep(5 * time.Second)
	}
}
