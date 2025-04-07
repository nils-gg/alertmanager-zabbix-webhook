package webhook

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
	//"reflect"

	zabbix "github.com/adubkov/go-zabbix"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var log = logrus.WithField("context", "webhook")

type WebHook struct {
	channel chan *Alert
	config  WebHookConfig
}

type WebHookConfig struct {
	Port                 int           `yaml:"port"`
	SecurePort           int           `yaml:"securePort"`
	CertFile             string        `yaml:"certFile"`
	KeyFile              string        `yaml:"keyFile"`
	QueueCapacity        int           `yaml:"queueCapacity"`
	RxLog                int           `yaml:"rxLog"`
	ZabbixServerHost     string        `yaml:"zabbixServerHost"`
	ZabbixServerPort     int           `yaml:"zabbixServerPort"`
	ZabbixHostAnnotation string        `yaml:"zabbixHostAnnotation"`
	ZabbixHostDefault    string        `yaml:"zabbixHostDefault"`
	ZabbixHostLabel      string        `yaml:"zabbixHostLabel"`
	ZabbixKeyDefault     string        `yaml:"zabbixKeyDefault"`
	ZabbixKeyLabel       string        `yaml:"zabbixKeyLabel"`
	ZabbixKeyPrefix      string        `yaml:"zabbixKeyPrefix"`
	ZabbixHostModifier   []ModifierMap `yaml:"zabbixHostModifier,omitempty"`
}

type ModifierMap struct {
	Name    string     `yaml:"name,omitempty"`
	Inspect string     `yaml:"inspect,omitempty"`
	Map     []Modifier `yaml:"map"`
}

type Modifier struct {
	Match  string `yaml:match"`
	Prefix string `yaml:prefix,omitempty"`
	Suffix string `yaml:suffix,omitempty"`
}

type HookRequest struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt,omitempty"`
	EndsAt       string            `json:"endsAt,omitempty"`
	GeneratorURL string            `json:"generatorURL"`
}

func New(cfg *WebHookConfig) *WebHook {

	return &WebHook{
		channel: make(chan *Alert, cfg.QueueCapacity),
		config:  *cfg,
	}
}

func ConfigFromFile(filename string) (cfg *WebHookConfig, err error) {
	log.Infof("Loading configuration at '%s'", filename)
	configFile, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("can't open the config file: %s", err)
	}

	// Default values
	config := WebHookConfig{
		Port:                 8080,
		SecurePort:           10443,
		CertFile:             "",
		KeyFile:              "",
		QueueCapacity:        500,
		RxLog:                0,
		ZabbixServerHost:     "127.0.0.1",
		ZabbixServerPort:     10051,
		ZabbixHostAnnotation: "zabbix_host",
		ZabbixHostDefault:    "",
		ZabbixHostLabel:      "zabbix_host",
		ZabbixKeyDefault:     "",
		ZabbixKeyLabel:       "",
		ZabbixKeyPrefix:      "prometheus",
	}

	err = yaml.Unmarshal(configFile, &config)
	if err != nil {
		return nil, fmt.Errorf("can't read the config file: %s", err)
	}

	log.Info("Configuration loaded")
	return &config, nil
}

func (hook *WebHook) Start() error {

	// Launch the process thread
	go hook.processAlerts()

	//log.Infof("hook.config.ZabbixHostModifier type: %T", hook.config.ZabbixHostModifier)
	if typeof(hook.config.ZabbixHostModifier) == "[]webhook.ModifierMap" {
		for _, zhm := range hook.config.ZabbixHostModifier {
			var zhmi = zhm.Inspect
			if 0 == len(zhmi) {
				zhmi = "generatorURL"
			}
			var zhmn = zhm.Name
			if 0 == len(zhmn) {
				zhmn = zhmi
			}
			log.Infof(" [%s] inspect: %s", zhmn, zhmi)
			var zhmm = zhm.Map
			//log.Infof(" zhm.Map type: %T", zhmm)
			if typeof(zhmm) == "[]webhook.Modifier" {
				//log.Infof(" zhm.Map: %s", zhmm)
				h := "server01"
				h1 := ""
				for _, m := range zhmm {
					// log.Infof("  Match: %s, Prefix: %s, Suffix: %s", m.Match, m.Prefix, m.Suffix)
					if len(m.Suffix) != 0 {
						h1 = h + m.Suffix
					} else if m.Prefix != "" {
						h1 = m.Prefix + h
					} else {
						h1 = h
					}
					log.Infof(" %s contains(%s) ? %s -> %s", zhmi, m.Match, h, h1)
				}
			}
		}
	}

	// Launch the listening thread
	// http.HandleFunc("/alerts", hook.alertsHandler)
	mux := http.NewServeMux()
	mux.HandleFunc("/alerts", hook.alertsHandler)
	var err error
	if hook.config.CertFile == "" || hook.config.KeyFile == "" {
		log.Println("Initializing HTTP server")
		srv := &http.Server{
			Addr:    ":" + strconv.Itoa(hook.config.Port),
			Handler: mux,
		}
		err = srv.ListenAndServe()
		// err = http.ListenAndServe(":"+strconv.Itoa(hook.config.Port), nil)
	} else {
		log.Println("Initializing HTTPS server")
		srv := &http.Server{
			Addr:    ":" + strconv.Itoa(hook.config.Port),
			Handler: mux,
			TLSConfig: &tls.Config{
				MinVersion:               tls.VersionTLS12,
				PreferServerCipherSuites: true,
			},
		}
		err = srv.ListenAndServeTLS(hook.config.CertFile, hook.config.KeyFile)
		// err = http.ListenAndServeTLS(":"+strconv.Itoa(hook.config.Port), hook.config.CertFile, hook.config.KeyFile, nil)
	}
	if err != nil {
		return fmt.Errorf("can't start the listening thread: %s", err)
	}

	log.Info("Exiting")
	close(hook.channel)

	return nil
}

func (hook *WebHook) alertsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		hook.postHandler(w, r)
	default:
		http.Error(w, "unsupported HTTP method", 400)
	}
}

func (hook *WebHook) postHandler(w http.ResponseWriter, r *http.Request) {

	rawbody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}
	// set a new body, which will simulate the same data we read:
	r.Body = ioutil.NopCloser(bytes.NewBuffer(rawbody))

	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var m HookRequest
	if err := dec.Decode(&m); err != nil {
		log.Errorf("error decoding message: %v", err)
		http.Error(w, "request body is not valid json", 400)
		return
	}

	if hook.config.RxLog != 0 {
		body, err := json.Marshal(m)
		if err == nil {
			log.Infof("%s sent: '%s' -> '%s'", r.RemoteAddr, rawbody, body)
		}
	}

	for index := range m.Alerts {
		hook.channel <- &m.Alerts[index]
	}
}

func (hook *WebHook) processAlerts() {

	log.Info("Alerts queue started")

	// While there are alerts in the queue, batch them and send them over to Zabbix
	var metrics []*zabbix.Metric
	for {
		select {
		case a := <-hook.channel:
			if a == nil {
				log.Info("Queue Closed")
				return
			}

			host, exists := a.Annotations[hook.config.ZabbixHostAnnotation]
			if !exists {
				host, exists = a.Labels[hook.config.ZabbixHostLabel]
			}
			if !exists {
				host = hook.config.ZabbixHostDefault
			}

			// itemkey, exists := a.Labels[hook.config.ZabbixKeyLabel]
			// if !exists {
            itemkey := hook.config.ZabbixKeyDefault
			// 	exists = itemkey != ""
			// }

			// Send alerts only if a host annotation is present or configuration for default host is not empty
			if host != "" {
				if !exists {
					log.Errorf("*** message from host %q dropped; key not found or no default key set", host)
				} else {

					// with ZabbixHostModifierMap config: process needed substitutions
					if 0 != len(hook.config.ZabbixHostModifier) {
						h1 := hook.modifyHost(host, a)
						//log.Infof("  out: host: %s, h1: %s",  host, h1)
						if h1 != host {
							log.Infof("  ZabbixHostModifier result: %s -> %s", host, h1)
							host = h1
						} else {
							log.Infof("  host not altered ZabbixHostModifierMap: %s", host)
						}
					}

					// key := fmt.Sprintf("%s.%s", hook.config.ZabbixKeyPrefix, strings.ToLower(itemkey))
					key := strings.ToLower(itemkey)
					body, err := json.Marshal(a)
					if err == nil {
						// log.Infof("%s sent: '%s'", r.RemoteAddr, body)
						log.Infof("added Zabbix metrics, host: '%s' key: '%s', text: '%s'", host, key, body)
						metrics = append(metrics, zabbix.NewMetric(host, key, string(body[:])))
					} else {
						log.Errorf("*** host: '%s' key: '%s' json.Marshal failed: %v", host, key, err)
					}

					// value := "0"
					// if a.Status == "firing" {
					//	value = "1"
					// }
					// log.Infof("added Zabbix metrics, host: '%s' key: '%s', value: '%s'", host, key, value)
					// metrics = append(metrics, zabbix.NewMetric(host, key, value))
				}
			}
		default:
			if len(metrics) != 0 {
				hook.zabbixSend(metrics)
				metrics = metrics[:0]
			} else {
				time.Sleep(1 * time.Second)
			}
		}
	}
}

func (hook *WebHook) zabbixSend(metrics []*zabbix.Metric) {
	// Create instance of Packet class
	packet := zabbix.NewPacket(metrics)

	// Send packet to zabbix
	log.Infof("sending to zabbix '%s:%d'", hook.config.ZabbixServerHost, hook.config.ZabbixServerPort)
	z := zabbix.NewSender(hook.config.ZabbixServerHost, hook.config.ZabbixServerPort)
	_, err := z.Send(packet)
	if err != nil {
		log.Error(err)
	} else {
		log.Info("successfully sent")
	}

}

// modify hostnames, according to config in the zabbixHostModifier array
func (hook *WebHook) modifyHost(h string, a *Alert) string {
	//log.Infof(" modifyHost testing alert for host: %s", h)
	h1 := h
	for _, zhm := range hook.config.ZabbixHostModifier {
		//log.Infof("  -in: zhm.name: %s, h: %s", zhm.Name, h)
		// look for Alert field "f" referenced by zhm.Inspect, as Annotation, Label or direct - copy value to "v"
		f := zhm.Inspect
		v := ""
		exists := false
		if (0 == len(f)) || (f == "generatorURL") {
			f = "generatorURL"
			v = a.GeneratorURL
		} else {
			if v, exists = a.Annotations[f]; !exists {
				v, exists = a.Labels[f]
			}
			if !exists {
				continue // field to inspect absent on Alert: try next ModifierMap
			}
		}
		//log.Infof("  inspecting '%s': %s", f, v)
		// Relevant field 'f' found - loop over Match -> Prefix/Suffix rules
		for _, m := range zhm.Map {
			if strings.Contains(v, m.Match) {
				if len(m.Suffix) != 0 {
					h1 = h1 + m.Suffix
				} else if m.Prefix != "" {
					h1 = m.Prefix + h1
				}
				//log.Infof(" %s.contains(%s) ? %s -> %s", f, m.Match, h, h1)
			}
		}
		//}
	}
	return h1
}

// return a variable's type
func typeof(v interface{}) string {
	return fmt.Sprintf("%T", v)
}
