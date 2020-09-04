package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"k8s.io/klog"
	"time"
)

func main() {
	kubeApiHealthUrl := "https://192.168.56.6:6443/healthz"
	luaApiGetBackendsUrl := "http://127.0.0.1:10246/configuration/backends"

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	kubeClient := http.Client{Transport: tr}
	kubeRes, err := kubeClient.Get(kubeApiHealthUrl)
	if err != nil {
		klog.Error(err)
	}
	klog.Infof("Check API server status: ", kubeRes.Status)

	if kubeRes.StatusCode == 200 {
		time.Sleep(5 * time.Second)
	} else {
		luaClient := http.Client{}
		luaRes, err := luaClient.Get(luaApiGetBackendsUrl)
		if err != nil {
			klog.Error(err)
		}
		if luaRes.StatusCode != 200 {
			klog.Errorf("Get backends by lua API err, code: ", luaRes.StatusCode)
		}
		defer  luaRes.Body.Close()
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

		backends := f.([]interface{})
		for _, backendi := range backends {
			backend := backendi.(map[string]interface{})

			endpoints := backend["endpoints"].([]interface{})
			for _, endpointi := range endpoints {
				endpoint := endpointi.(map[string]interface{})
				addr := endpoint["address"].(string)
				port := endpoint["port"].(string)
				epUrl := "http://" + addr + port

				tmpRes, err := luaClient.Get(epUrl)
				if err != nil {
					klog.Error(err)
				}

				if tmpRes.StatusCode == 404 || tmpRes.StatusCode == 200 {
					klog.Infof("Check endpoint url: %s, status: %s", epUrl, tmpRes.Status)
				} else {
					//todo: delete ep, update endpoint
				}

			}
		}


	}


}
