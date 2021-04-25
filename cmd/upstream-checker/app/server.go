package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/huandu/go-clone"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/cmd/upstream-checker/app/option"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/pkg/util"
	"io/ioutil"
	"k8s.io/klog"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultBadBackendsPath = "/root/bad/"

type BadBackends struct {
	Backends []BadBackend `json:"backends"`
}

type BadBackend struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    string `json:"port"`
}

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
	var goodChange bool
	var updateChange bool
	var badBackends BadBackends
	var goodBackends BadBackends
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
			go func(addr, port string, endpoints []interface{}, j int) {
				epUrl := "http://" + addr + ":" + port
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
						// Improved: record the bad backend to the Object(badBackends).
						// 1.read the record file, and convert it into Object;
						// 2.update the Object, and write to record file.
						var badBackend BadBackend
						badBackend.Name = backend["name"].(string)
						badBackend.Address = addr
						badBackend.Port = port
						badBackends.Backends = append(badBackends.Backends, badBackend)

						tmpEp := make(map[string]interface{})
						tmpEp["address"] = ""
						tmpEp["port"] = ""
						endpoints[j] = tmpEp
						change = true
					}
				}
				wg.Done()
			}(addr, port, endpoints, j)
		}
	}
	wg.Wait()
	klog.V(6).Infof("[%s] old backends data: %v", luaApiPort, oldBackends)
	klog.V(6).Infof("[%s] new backends data: %v", luaApiPort, newBackends)

	// Improved: check the bad endpoints from record file.
	// json: {"backends": [{"name": "xxx", "address": "x.x.x.x", "port": "xxx"}]}
	// 1.read the record file, and convert it into Object;
	// 2.cur every endpoint from the Object. if check successful, update to the upstream
	badBackendsFromFile, err := getBadBackendsByFile(luaApiPort)
	if err != nil {
		klog.Errorln("[Improved] read json file has err: %s", err.Error())
		return err
	}

	// Multi-threaded health check request
	wgBad := sync.WaitGroup{}
	if badBackendsFromFile.Backends == nil || len(badBackendsFromFile.Backends) == 0 {
		klog.Info("[Improved] bad backends json file is empty")
	} else {
		wgBad.Add(len(badBackendsFromFile.Backends))
		for _, backend := range badBackendsFromFile.Backends {
			addrBad := backend.Address
			portBad := backend.Port
			go func(addrBad, portBad string, backend BadBackend) {
				epUrl := "http://" + addrBad + ":" + portBad
				_, err := util.HttpGet(epUrl)
				if err == nil {
					goodBackends.Backends = append(goodBackends.Backends, backend)
					change = true
					klog.Infof("[Improved] the bad backend change to good, backend name: %s, ep url: %s", backend.Name, epUrl)
				} else {
					klog.Infof("[Improved] the bad backend still bad, backend name: %s, ep url: %s", backend.Name, epUrl)
				}
				wgBad.Done()
			} (addrBad, portBad, backend)
		}
	}
	wgBad.Wait()

	if change {
		for i, backendi := range newBackends {
			backend := backendi.(map[string]interface{})
			var healthEndpoints []map[string]interface{}
			var endpoints []interface{}
			if backend["endpoints"] != nil {
				endpoints = backend["endpoints"].([]interface{})
			}
			// Improved: If has goodBackends, add goodBackends endpoint to the endpoints.
			if goodBackends.Backends != nil && len(goodBackends.Backends) > 0 {
				backendName := backend["name"].(string)
				endpoints, goodChange = addGoodEndpoint(goodBackends, endpoints, backendName, goodChange)
			}

			if endpoints == nil || len(endpoints) == 0 {
				continue
			}

			for _, endpointi := range endpoints {
				endpoint := endpointi.(map[string]interface{})
				addr := endpoint["address"].(string)
				port := endpoint["port"].(string)
				if addr == "" || port == "" {
					continue
				}
				healthEndpoints = append(healthEndpoints, endpoint)
			}
			if healthEndpoints == nil {
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
			return err
		} else {
			//goodBackends = BadBackends{}
			updateChange = true
		}

		klog.V(2).Infof("[%s] Update backends by lua API succeed, goodBackends: %v", luaApiPort, goodBackends)

		change = false
	} else {
		klog.V(2).Infof("[%s] Upstream backends is health, backends no change", luaApiPort)
	}

	// Improved:
	//oldBackendsFromFile, err := getBadBackendsByFile()
	var badChange bool
	var newBackendsToFile BadBackends
	if badBackendsFromFile.Backends == nil || len(badBackendsFromFile.Backends) == 0 {
		klog.Errorln("[Improved] bad backends from file is empty")
	} else {
		if updateChange && goodChange {
			newBackendsToFile.Backends = intersect(badBackendsFromFile.Backends, goodBackends.Backends)
			badChange = true
		} else {
			newBackendsToFile.Backends = badBackendsFromFile.Backends
		}
	}

	if badBackends.Backends != nil && len(badBackends.Backends) > 0 {
		newBackendsToFile.Backends = append(badBackends.Backends, newBackendsToFile.Backends...)
		badChange = true
	}

	if len(badBackendsFromFile.Backends) == len(newBackendsToFile.Backends) || !badChange {
		klog.V(2).Infof("[Improved] bad backends from file is no change")
	} else {
		badBackendsBytes, err := json.Marshal(newBackendsToFile)
		if err != nil {
			klog.Errorln("[Improved] from json to bytes has err: %s", err.Error())
			return err
		}
		filePathAll := fmt.Sprintf("%sbackends-%s.json", defaultBadBackendsPath, luaApiPort)
		err = ioutil.WriteFile(filePathAll, badBackendsBytes, os.ModeAppend)
		if err != nil {
			klog.Errorln("[Improved] write bytes to file has err: %s", err.Error())
			return err
		} else {
			goodBackends = BadBackends{}
			klog.Infof("[Improved] bad backends json file is updated")
		}
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

func getBadBackendsByFile(luaApiPort string) (BadBackends, error) {
	bbs := BadBackends{}
	filePathAll := fmt.Sprintf("%sbackends-%s.json", defaultBadBackendsPath, luaApiPort)
	jsonFile, err := os.Open(filePathAll)
	defer jsonFile.Close()
	if err != nil && os.IsNotExist(err) {
		_, err = os.Create(filePathAll)
		return bbs, err
	}
	if err != nil {
		return bbs, err
	}

	f, _ := jsonFile.Stat()
	if f.Size() == 0 {
		return bbs, nil
	}
	buffer := make([]byte, f.Size())
	_, err = jsonFile.Read(buffer)
	if err != nil {
		return bbs, err
	}
	err = json.Unmarshal(buffer, &bbs)
	if err != nil {
		return bbs, err
	}
	return bbs, nil
}

func addGoodEndpoint(goodBackends BadBackends, endpoints []interface{}, backendName string, goodChange bool) ([]interface{}, bool) {
	var change bool
	for _, goodBackend := range goodBackends.Backends {
		if backendName == goodBackend.Name {
			var exist bool
			if endpoints != nil && len(endpoints) > 0 {
				for _, endpointi := range endpoints {
					endpoint := endpointi.(map[string]interface{})
					addr := endpoint["address"].(string)
					port := endpoint["port"].(string)
					if addr == goodBackend.Address && port == goodBackend.Port {
						exist = true
					}
				}
				if !exist {
					goodEndpoint := map[string]interface{}{
						"address": goodBackend.Address,
						"port": goodBackend.Port,
					}
					endpoints = append(endpoints, goodEndpoint)
					change = true
				}
			} else {
				goodEndpoint := map[string]interface{}{
					"address": goodBackend.Address,
					"port": goodBackend.Port,
				}
				endpoints = append(endpoints, goodEndpoint)
				change = true
			}
		}
	}
	// When a change occurs, the change value becomes true
	if goodChange {
		change = goodChange
	}
	return endpoints, change
}

func intersect(b1, b2 []BadBackend) []BadBackend {
	var result []BadBackend
	for _, v2 := range b2 {
		for k1, v1 := range b1{
			if v1.Name == v2.Name && v1.Port == v2.Port && v1.Address == v2.Address {
				continue
			}
			result = append(result, b1[k1])
		}
	}
	return result
}
