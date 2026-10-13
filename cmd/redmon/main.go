package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	embredmon "git.fisica.unipg.it/redmon/embredmonwork.git"
	redmonmel "git.fisica.unipg.it/redmon/redmonmel.git"
	"github.com/gomodule/redigo/redis"
	"github.com/jroimartin/gocui"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	DEFAULTH           = 3
	WPAD               = 10
	VPAD               = 10
	VIEWCHANGEINTERVAL = 5
	UPDATEINTERVAL     = 300
	green              = "\033[32m"
	red                = "\033[31m"
	yellow             = "\033[33m"
	normal             = "\033[0m"
)

const webheader = `
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
<title>Sensor Editor</title>
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.5.2/css/bootstrap.min.css">
<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/font-awesome/4.7.0/css/font-awesome.min.css">
<script src="https://code.jquery.com/jquery-3.5.1.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/popper.js@1.16.1/dist/umd/popper.min.js"></script>
<script src="https://stackpath.bootstrapcdn.com/bootstrap/4.5.2/js/bootstrap.min.js"></script>
<style>
    p.title{
        padding: 2px;
        font-size: 12px;
        font-weight: bold;
        text-align: left;
		background: #dbdfe5;
		margin-bottom: 0px;
	}
	p.navig{
        padding: 0px 20px 20px 20px;
        font-size: 16px;
        font-weight: bold;
        text-align: center;
		background: #dbdfe5;
		margin-bottom: 10px;
    }
    p{
        padding: 30px;
        font-size: 16px;
        font-weight: bold;
        text-align: center;
        background: #dbdfe5;
    }
</style>`

var (
	up = prometheus.NewDesc(
		"redmon_up",
		"Was talking to redmon successful.",
		nil, nil,
	)
)

func (rm *RedmonManager) viewHandler(w http.ResponseWriter, req *http.Request) {
	view := req.FormValue("view")
	result := webheader
	result += `
</head>
<body>`
	if *navig {
		result += `
    <div><p class="title">View dashboard</p></div>
    <div><p class="navig"><a href="/views">All&nbsp;views</a>&nbsp;&nbsp;
    <a href="/missing">Missing configurations</a></p></div>`
	}
	result += `
    <div class="container-fluid mt-3">
		<div class="row">
		`
	sensors := rm.sensors
	for sensorname, sensor := range *sensors {
		if view == "" || IsIn(sensor.GetViews(), view) {
			result += "<div class=\"col-auto\"><p>" + sensorname + "<br /><span style=\"color: green;\">" + sensor.GetValue() + "</span></p></div>"
		}
	}
	result += `
        </div>
    </div>
</body>
</html>

`
	io.WriteString(w, result)
}

func (rm *RedmonManager) viewsHandler(w http.ResponseWriter, req *http.Request) {

	result := webheader
	result += `
</head>
<body>`
	if *navig {
		result += `
     <div><p class="title">All views</p></div>
    <div><p class="navig"><a href="/views">All&nbsp;views</a>&nbsp;&nbsp;
	<a href="/missing">Missing configurations</a></p></div>`
	}
	result += `
    <div class="container-fluid mt-3">
		<div class="row">
		`
	views := rm.views
	for _, view := range views {
		result += "<div class=\"col-auto\"><p><a href=\"/view?view=" + view + "\">" + view + "</a></p></div>"
	}
	result += `
        </div>
    </div>
</body>
</html>

`
	io.WriteString(w, result)
}

func (rm *RedmonManager) missing(w http.ResponseWriter, req *http.Request) {

	result := webheader
	result += `
</head>
<body>`
	if *navig {
		result += `
    <div><p class="title">Missing sensors summary</p></div>
    <div><p class="navig"><a href="/views">All&nbsp;views</a>&nbsp;&nbsp;
	<a href="/missing">Missing configurations</a></p></div>`
	}
	result += `
    <div class="container-fluid mt-3">
		<div class="row">
		`
	c := rm.config

	for _, redconfig := range c.Sources {

		stype := redconfig.Stype

		switch stype {
		case "redis":

			redmon_endpoint := redconfig.Endpoint
			redmon_key := redconfig.Key
			redmon_db := redconfig.Db

			c, err := redis.Dial("tcp", redmon_endpoint)
			if err == nil {
				Debug("Connection to Redis established")
			} else {
				Alert(err)
				continue
			}
			defer c.Close()

			// Authentication
			if redmon_key != "" {
				if _, ok := c.Do("AUTH", redmon_key); ok == nil {
					Debug("Authentication to Redis succeded")
				} else {
					Alert("Authentication to Redis failed")
					continue
				}
			}

			// Database selection

			if _, ok := c.Do("SELECT", redmon_db); ok == nil {
				Debug("Selected database " + redmon_db)
			} else {
				Alert("Database selection " + redmon_db + "Falied")
				continue
			}

			// Getting the list of configured metrics
			keys, _ := redis.Strings(c.Do("KEYS", "*"))

			for _, redmonkey := range keys {
				if !strings.HasSuffix(redmonkey, ":redmon") {
					if !IsIn(keys, redmonkey+":redmon") {
						result += "<div class=\"col-auto\"><p><a href=\"/template?sensor=" + redmonkey + "\">" + redmonkey + "</a></p></div>"
					}
				}
			}
		}
	}
	result += `
        </div>
    </div>
</body>
</html>

`
	io.WriteString(w, result)
}

func (rm *RedmonManager) sensorEdit(w http.ResponseWriter, req *http.Request) {
	sensor := req.FormValue("sensor")
	template := req.FormValue("template")
	result := webheader
	result += `
</head>`
	if *navig {
		result += `
    <div><p class="title">Sensor Editor</p></div>
    <div><p class="navig"><a href="/views">All&nbsp;views</a>&nbsp;&nbsp;
	<a href="/missing">Missing configurations</a></p></div>`
	}
	result += `
<body>
    <div class="container-fluid mt-3">
		<div class="row">
		`

	templates := make([]string, 0)
	if files, err := ioutil.ReadDir(*templateDir); err == nil {
		for _, file := range files {
			templates = append(templates, file.Name())
		}
	} else {
		result += "<div><h3>Template dir reading failed</h3></div>"
	}
	result += "<div class=\"col-auto\"><p>" + sensor + "<br /><span style=\"color: green;\">" + template + "</span></p></div>"
	result += "<div class=\"col-auto\"><p><span style=\"color: red;\">Apply:</span></p></div>"

	for _, t := range templates {
		result += "<div class=\"col-auto\"><p><a href=\"/template?sensor=" + sensor + "&template=" + t + "\">" + t + "</a></p></div>"
	}

	if sensor != "" && template != "" {
		c := rm.config

		for _, redconfig := range c.Sources {

			stype := redconfig.Stype

			switch stype {
			case "redis":

				redmon_endpoint := redconfig.Endpoint
				redmon_key := redconfig.Key
				redmon_db := redconfig.Db

				c, err := redis.Dial("tcp", redmon_endpoint)
				if err == nil {
					Debug("Connection to Redis established")
				} else {
					Alert(err)
					continue
				}
				defer c.Close()

				// Authentication
				if redmon_key != "" {
					if _, ok := c.Do("AUTH", redmon_key); ok == nil {
						Debug("Authentication to Redis succeded")
					} else {
						Alert("Authentication to Redis failed")
						continue
					}
				}

				// Database selection

				if _, ok := c.Do("SELECT", redmon_db); ok == nil {
					Debug("Selected database " + redmon_db)
				} else {
					Alert("Database selection " + redmon_db + "Failed")
					continue
				}

				if file, err := os.Open(*templateDir + "/" + template); err == nil {

					configdata := ""
					scanner := bufio.NewScanner(file)
					for scanner.Scan() {
						line := scanner.Text()
						configdata = configdata + line
					}
					if !json.Valid([]byte(configdata)) {
						Alert("Invalid input json")
					} else {
						sensorc := new(SensorConfig)
						if err := json.Unmarshal([]byte(configdata), sensorc); err == nil {
							value, _ := json.Marshal(sensorc)
							value = []byte(strings.ReplaceAll(string(value), "[[key]]", sensor))
							if _, err := c.Do("SET", sensor+":redmon", string(value)); err != nil {
								Alert("SET key failed")
							}
						} else {
							Alert("Loaded JSON does not fit into sensorconfig data structure")
						}
					}
				}
			}
		}
	}

	result += `
        </div>
    </div>
</body>
</html>

`
	io.WriteString(w, result)
}

//////////////////////////////////////////////////////////////////// Prometheus

type RedmonCollector struct {
	rm *RedmonManager
}

func (r RedmonCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
}

func (r RedmonCollector) Collect(ch chan<- prometheus.Metric) {

	ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 1)

	sensors := r.rm.sensors

	for sensorname, sensor := range *sensors {
		if sensor.GetType() == "numeric" {
			if val, err := strconv.ParseFloat(sensor.GetValue(), 64); err == nil {

				pfiltername := strings.Replace(sensorname, "-", "", -1)

				if len(pfiltername) < 32 {

					desc := prometheus.NewDesc(pfiltername, "Redmon metric "+sensorname, nil, nil)
					ch <- prometheus.MustNewConstMetric(
						desc, prometheus.GaugeValue, float64(val))

				}
			}
		}
	}

}

//////////////////////////////////////////////////////////////////// Configurations

type RedmonConfig struct {
	Sources []struct {
		Name     string `json:"name"`
		Stype    string `json:"type"`
		Endpoint string `json:"endpoint"`
		Key      string `json:"key"`
		Db       string `json:"db"`
	} `json:"sources"`
	Sensors []SensorConfig `json:"sensors"`
}

type SensorConfig struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Key        string   `json:"key"`
	SensorType string   `json:"type"`
	Views      []string `json:"views"`
	Warning    []string `json:"warning"`
	Alert      []string `json:"alert"`
	Refresh    uint16   `json:"refresh"`
	TTL        uint16   `json:"ttl"`
}

func (c *RedmonConfig) String() string {
	return fmt.Sprint(*c)
}

//////////////////////////////////////////////////////////////////// Utils

func putText(w int, color string, text string) string {
	result := color + fmt.Sprintf("%[1]*s", -w, fmt.Sprintf("%[1]*s", (w+len(text))/2, text)) + normal
	return result
}

//////////////////////////////////////////////////////////////////// Requests and responses

type request struct {
	key string
}

type response struct {
	value string
	ok    bool
}

//////////////////////////////////////////////////////////////////// Sensors data

type Sensors map[string]Sensor

type Sensor interface {
	Initialize(rm *RedmonManager) error
	GetRefresh() uint16
	GetSource() string
	GetType() string
	GetViews() []string
	GetViewChan() chan string
	GetValue() string
	GetAlert() bool
	GetWarning() bool
	GetAlerts() []string
	GetWarnings() []string
	GetCollKillChan() chan struct{}
	GetViewKillChan() chan struct{}
}

// The InitializeSensors initialize the sensors present in the configuration the deamon is currently using.
// Static sensors are not modified further in the process run.
func (rm *RedmonManager) InitializeSensors() error {

	ss := new(Sensors)
	*ss = make(map[string]Sensor)

	rm.sensors = ss

	c := rm.config
	s := rm.sources

	msens := make(map[string]struct{})

	for _, sensconf := range c.Sensors {
		Info("Analyzing configured sensor \"" + yellow + sensconf.Name + normal + "\"")
		name := sensconf.Name
		if _, exists := msens[name]; exists {
			Alert("The sensor \"" + name + "\" already processed in configuration")
		} else {
			senstype := sensconf.SensorType
			switch senstype {
			case "numeric":
				newsensor := new(NumberSensor)
				newsensor.name = name
				newsensor.key = sensconf.Key
				newsensor.refresh = sensconf.Refresh
				newsensor.views = make([]string, len(sensconf.Views))
				for i, view := range sensconf.Views {
					newsensor.views[i] = view
				}
				newsensor.alert = make([]string, len(sensconf.Alert))
				for i, cond := range sensconf.Alert {
					newsensor.alert[i] = cond
				}
				newsensor.warning = make([]string, len(sensconf.Warning))
				for i, cond := range sensconf.Warning {
					newsensor.warning[i] = cond
				}
				if source, ok := (*s)[sensconf.Source]; ok {
					newsensor.source = source
					if err := newsensor.Initialize(rm); err != nil {
						Alert(err)
					} else {
						Info("The numeric sensor \"" + name + "\" has been correctly configured")
						msens[name] = struct{}{}
						(*ss)[name] = newsensor
					}
				} else {
					Alert("The source of the sensor " + name + " is unknown")
				}
			case "string":
				newsensor := new(StringSensor)
				newsensor.name = name
				newsensor.key = sensconf.Key
				newsensor.refresh = sensconf.Refresh
				newsensor.views = make([]string, len(sensconf.Views))
				for i, view := range sensconf.Views {
					newsensor.views[i] = view
				}
				newsensor.alert = make([]string, len(sensconf.Alert))
				for i, cond := range sensconf.Alert {
					newsensor.alert[i] = cond
				}
				newsensor.warning = make([]string, len(sensconf.Warning))
				for i, cond := range sensconf.Warning {
					newsensor.warning[i] = cond
				}
				if source, ok := (*s)[sensconf.Source]; ok {
					newsensor.source = source
					if err := newsensor.Initialize(rm); err != nil {
						Alert(err)
					} else {
						Info("The string sensor \"" + name + "\" has been correctly configured")
						msens[name] = struct{}{}
						(*ss)[name] = newsensor
					}
				} else {
					Alert("The source of the sensor " + name + " is unknown")
				}
			default:
				Alert("Unknown sensor type " + senstype)
			}
		}
	}

	return nil
}

func (rm *RedmonManager) UpdateAutomaticSensors() error {

	c := rm.config
	s := rm.sources
	ss := rm.sensors
	g := rm.gui

	rm.isConfChange = true

	for _, redconfig := range c.Sources {

		stype := redconfig.Stype
		sourcename := redconfig.Name
		source := (*s)[sourcename]

		Info("Analyzing configured source \"" + yellow + sourcename + normal + "\"")

		Debug("Getting the list of the already configured sensors for the source  \"" + yellow + sourcename + normal + "\"")

		sourcesensors := make(map[string]Sensor)

		for sensname, sens := range *ss {
			if sens.GetSource() == sourcename {
				sourcesensors[sensname] = sens
			}
		}

		Debug("Sensors running on  \"" + yellow + sourcename + normal + "\" are " + fmt.Sprint(sourcesensors))

		switch stype {
		case "redis":

			redmon_endpoint := redconfig.Endpoint
			redmon_key := redconfig.Key
			redmon_db := redconfig.Db

			c, err := redis.Dial("tcp", redmon_endpoint)
			if err == nil {
				Debug("Connection to Redis established")
			} else {
				Alert(err)
				continue
			}
			defer c.Close()

			// Authentication
			if redmon_key != "" {
				if _, ok := c.Do("AUTH", redmon_key); ok == nil {
					Debug("Authentication to Redis succeded")
				} else {
					Alert("Authentication to Redis failed")
					continue
				}
			}

			// Database selection

			if _, ok := c.Do("SELECT", redmon_db); ok == nil {
				Debug("Selected database " + redmon_db)
			} else {
				Alert("Database selection " + redmon_db + "Falied")
				continue
			}

			updatedsensors := make(map[string]*SensorConfig)

			// Getting the list of configured metrics
			keys, _ := redis.Strings(c.Do("KEYS", "*:redmon"))

			for _, redmonkey := range keys {

				if value, err := redis.String(c.Do("GET", redmonkey)); err == nil {
					sensconf := new(SensorConfig)
					if err := json.Unmarshal([]byte(value), sensconf); err != nil {
						Alert("The key \"" + yellow + redmonkey + normal + "\" contains a wrong configuration")
					} else {
						updatedsensors[sensconf.Name] = sensconf
					}
				}
			}

			Debug("Sensors configured on  \"" + yellow + sourcename + normal + "\" are " + fmt.Sprint(updatedsensors))

			Debug("Closing removed sensors down")
			for sensorname, sensor := range sourcesensors {
				if _, configured := updatedsensors[sensorname]; !configured {
					Debug("The sensor \"" + yellow + sensorname + normal + "\" is running and not configured, shutting it down")
					sensor.GetCollKillChan() <- struct{}{}
					sensor.GetViewKillChan() <- struct{}{}
					delete(*ss, sensorname)
					if g != nil {
						if v, err := g.View(sensorname); err == nil {
							v.Clear()
							g.DeleteView(sensorname)
						}
					}

				} else {
					Debug("The sensor \"" + yellow + sensorname + normal + "\" is running and configured, saving for later check")
				}
			}

			Debug("Starting new sensors")
			for newsensorname, newsensorconfig := range updatedsensors {

				if _, already := sourcesensors[newsensorname]; !already {
					Debug("The sensor \"" + yellow + newsensorname + normal + "\" is not running, starting a new one")

					senstype := newsensorconfig.SensorType
					switch senstype {
					case "numeric":
						newsensor := new(NumberSensor)
						newsensor.name = newsensorname
						newsensor.key = newsensorconfig.Key
						newsensor.refresh = newsensorconfig.Refresh
						newsensor.views = make([]string, len(newsensorconfig.Views))
						for i, view := range newsensorconfig.Views {
							newsensor.views[i] = view
						}
						newsensor.alert = make([]string, len(newsensorconfig.Alert))
						for i, cond := range newsensorconfig.Alert {
							newsensor.alert[i] = cond
						}
						newsensor.warning = make([]string, len(newsensorconfig.Warning))
						for i, cond := range newsensorconfig.Warning {
							newsensor.warning[i] = cond
						}
						newsensor.source = source
						if err := newsensor.Initialize(rm); err != nil {
							Alert(err)
						} else {
							Info("The numeric sensor \"" + yellow + newsensorname + normal + "\" has been correctly configured")
							(*ss)[newsensorname] = newsensor
						}
						delete(updatedsensors, newsensorname)
					case "string":
						newsensor := new(StringSensor)
						newsensor.name = newsensorname
						newsensor.key = newsensorconfig.Key
						newsensor.refresh = newsensorconfig.Refresh
						newsensor.views = make([]string, len(newsensorconfig.Views))
						for i, view := range newsensorconfig.Views {
							newsensor.views[i] = view
						}
						newsensor.alert = make([]string, len(newsensorconfig.Alert))
						for i, cond := range newsensorconfig.Alert {
							newsensor.alert[i] = cond
						}
						newsensor.warning = make([]string, len(newsensorconfig.Warning))
						for i, cond := range newsensorconfig.Warning {
							newsensor.warning[i] = cond
						}
						newsensor.source = source
						if err := newsensor.Initialize(rm); err != nil {
							Alert(err)
						} else {
							Info("The string sensor \"" + yellow + newsensorname + normal + "\" has been correctly configured")
							(*ss)[newsensorname] = newsensor
						}
						delete(updatedsensors, newsensorname)
					default:
						Alert("Unknown sensor type \"" + yellow + senstype + normal + "\"")
					}
				} else {
					Debug("The sensor \"" + yellow + newsensorname + normal + "\" is already running, saving for later check")
				}
			}

			Debug("Checking running sensors against configurations")

			for sensorname, sensor := range sourcesensors {
				if newsensorconfig, configured := updatedsensors[sensorname]; configured {
					Debug("The sensor \"" + yellow + sensorname + normal + "\" is running and configured checking consistency")

					needrestart := false

					if !needrestart && sensor.GetType() != newsensorconfig.SensorType {
						needrestart = true
					}
					if !needrestart && sensor.GetRefresh() != newsensorconfig.Refresh {
						needrestart = true
					}

					for _, view := range newsensorconfig.Views {
						viewok := false
						for _, sview := range sensor.GetViews() {
							if view == sview {
								viewok = true
								break
							}
						}
						if !viewok {
							needrestart = true
						}
					}

					for _, sview := range sensor.GetViews() {
						viewok := false
						for _, view := range newsensorconfig.Views {
							if view == sview {
								viewok = true
								break
							}
						}
						if !viewok {
							needrestart = true
						}
					}

					for _, warn := range newsensorconfig.Warning {
						warnok := false
						for _, swarn := range sensor.GetWarnings() {
							if warn == swarn {
								warnok = true
								break
							}
						}
						if !warnok {
							needrestart = true
						}
					}

					for _, swarn := range sensor.GetWarnings() {
						warnok := false
						for _, warn := range newsensorconfig.Warning {
							if warn == swarn {
								warnok = true
								break
							}
						}
						if !warnok {
							needrestart = true
						}
					}

					for _, alrt := range newsensorconfig.Alert {
						alrtok := false
						for _, salrt := range sensor.GetAlerts() {
							if alrt == salrt {
								alrtok = true
								break
							}
						}
						if !alrtok {
							needrestart = true
						}
					}

					for _, salrt := range sensor.GetAlerts() {
						alrtok := false
						for _, alrt := range newsensorconfig.Alert {
							if alrt == salrt {
								alrtok = true
								break
							}
						}
						if !alrtok {
							needrestart = true
						}
					}

					if needrestart {
						Debug("The sensor \"" + yellow + sensorname + normal + "\" is changed, shutting it down")
						sensor.GetCollKillChan() <- struct{}{}
						sensor.GetViewKillChan() <- struct{}{}
						delete(*ss, sensorname)

						if g != nil {
							if v, err := g.View(sensorname); err == nil {
								v.Clear()
								g.DeleteView(sensorname)
							}
						}

						Debug("The sensor \"" + yellow + sensorname + normal + "\" is changed, starting a new one")

						senstype := newsensorconfig.SensorType
						switch senstype {
						case "numeric":
							newsensor := new(NumberSensor)
							newsensor.name = sensorname
							newsensor.key = newsensorconfig.Key
							newsensor.refresh = newsensorconfig.Refresh
							newsensor.views = make([]string, len(newsensorconfig.Views))
							for i, view := range newsensorconfig.Views {
								newsensor.views[i] = view
							}
							newsensor.alert = make([]string, len(newsensorconfig.Alert))
							for i, cond := range newsensorconfig.Alert {
								newsensor.alert[i] = cond
							}
							newsensor.warning = make([]string, len(newsensorconfig.Warning))
							for i, cond := range newsensorconfig.Warning {
								newsensor.warning[i] = cond
							}
							newsensor.source = source
							if err := newsensor.Initialize(rm); err != nil {
								Alert(err)
							} else {
								Info("The numeric sensor \"" + yellow + sensorname + normal + "\" has been correctly configured")
								(*ss)[sensorname] = newsensor
							}
						case "string":
							newsensor := new(StringSensor)
							newsensor.name = sensorname
							newsensor.key = newsensorconfig.Key
							newsensor.refresh = newsensorconfig.Refresh
							newsensor.views = make([]string, len(newsensorconfig.Views))
							for i, view := range newsensorconfig.Views {
								newsensor.views[i] = view
							}
							newsensor.alert = make([]string, len(newsensorconfig.Alert))
							for i, cond := range newsensorconfig.Alert {
								newsensor.alert[i] = cond
							}
							newsensor.warning = make([]string, len(newsensorconfig.Warning))
							for i, cond := range newsensorconfig.Warning {
								newsensor.warning[i] = cond
							}
							newsensor.source = source
							if err := newsensor.Initialize(rm); err != nil {
								Alert(err)
							} else {
								Info("The string sensor \"" + yellow + sensorname + normal + "\" has been correctly configured")
								(*ss)[sensorname] = newsensor
							}
						default:
							Alert("Unknown sensor type \"" + yellow + senstype + normal + "\"")
						}

					}

				}
			}

		default:
			Alert("Unknown source type " + stype)
		}
	}

	rm.UpdateViews()

	rm.isConfChange = false

	return nil
}

//////////////////////////////////////////////////////////////////// Number sensor

type NumberSensor struct {
	name         string
	source       DataSource
	key          string
	value        string
	refresh      uint16
	collkillchan chan struct{}
	viewkillchan chan struct{}
	viewchan     chan string
	views        []string
	alert        []string
	warning      []string
	inView       bool
	inAlert      bool
	inWarning    bool
}

func (sens *NumberSensor) Initialize(rm *RedmonManager) error {
	sens.collkillchan = make(chan struct{})
	sens.viewkillchan = make(chan struct{})
	sens.viewchan = make(chan string)
	go sens.runSensorCollector(rm)
	go sens.runSensorView(rm)
	return nil
}

func (sens *NumberSensor) GetRefresh() uint16 {
	return sens.refresh
}

func (sens *NumberSensor) GetSource() string {
	return sens.source.GetName()
}

func (sens *NumberSensor) GetType() string {
	return "numeric"
}

func (sens *NumberSensor) GetViews() []string {
	return sens.views
}

func (sens *NumberSensor) GetViewChan() chan string {
	return sens.viewchan
}

func (sens *NumberSensor) GetCollKillChan() chan struct{} {
	return sens.collkillchan
}

func (sens *NumberSensor) GetViewKillChan() chan struct{} {
	return sens.viewkillchan
}

func (sens *NumberSensor) GetValue() string {
	return sens.value
}

func (sens *NumberSensor) GetAlert() bool {
	return sens.inAlert
}

func (sens *NumberSensor) GetWarning() bool {
	return sens.inWarning
}

func (sens *NumberSensor) GetAlerts() []string {
	return sens.alert
}

func (sens *NumberSensor) GetWarnings() []string {
	return sens.warning
}

func (sens *NumberSensor) runSensorCollector(rm *RedmonManager) {

	Debug("Goroutine for sensor collector " + sens.name + " started")

	g := rm.gui

	reqchan := sens.source.GetRequestChan()
	reschan := sens.source.GetResponseChan()
	killchan := sens.collkillchan

loop:
	for {
		select {
		case <-killchan:
			break loop
		case <-time.After(time.Duration(sens.refresh) * time.Second):

			// Skip all sensors operation if the system is updating
			if !rm.isConfChange {
				Info("Request for key \"" + yellow + sens.key + normal + "\"")
				newvalue := ""

				reqchan <- request{sens.key + ":suspended"}
				if response := <-reschan; response.ok {

					reqchan <- request{sens.key}

					if response := <-reschan; response.ok {
						Info("Response for suspended key \"" + yellow + sens.key + normal + "\" value: " + response.value)
						newvalue = "SUSPENDED ACTIVE"
					} else {
						Info("Response for suspended key missing")
						newvalue = "SUSPENDED"
					}

					sens.inAlert = false
					sens.inWarning = false

				} else {
					reqchan <- request{sens.key}

					if response := <-reschan; response.ok {
						Info("Response for key \"" + yellow + sens.key + normal + "\" value: " + response.value)
						if response.value == "" {
							newvalue = "EMPTY"
						} else {
							newvalue = response.value
						}
					} else {
						Info("Response for key missing")
						newvalue = "MISSING"
					}

					sens.inAlert = redmonmel.ProcessConditions(sens.alert, newvalue)
					sens.inWarning = redmonmel.ProcessConditions(sens.warning, newvalue)
				}

				if newvalue != sens.value {
					sens.value = newvalue
					if *gui && (sens.inView || sens.inAlert || sens.inWarning) {
						g.Update(func(g *gocui.Gui) error {
							v, err := g.View(sens.name)

							if err == nil {
								w, _ := v.Size()
								v.Clear()
								if sens.inAlert {
									fmt.Fprint(v, putText(w, red, sens.value))
								} else if sens.inWarning {
									fmt.Fprint(v, putText(w, yellow, sens.value))
								} else {
									fmt.Fprint(v, putText(w, green, sens.value))
								}
							}
							return nil
						})
					}
				}
			}
		}
	}
	Debug("Goroutine for sensor " + sens.name + " stopped")
}

func (sens *NumberSensor) runSensorView(rm *RedmonManager) {

	killchan := sens.viewkillchan

	Debug("Goroutine for sensor view " + sens.name + " started")

loop:
	for {
		select {
		case <-killchan:
			break loop
		case newview := <-sens.viewchan:
			if IsIn(sens.views, newview) {
				sens.inView = true
			} else {
				sens.inView = false
			}
		}
	}

	Debug("Goroutine for sensor view " + sens.name + " stopped")
}

//////////////////////////////////////////////////////////////////// String sensor

type StringSensor struct {
	name         string
	source       DataSource
	key          string
	value        string
	refresh      uint16
	collkillchan chan struct{}
	viewkillchan chan struct{}
	viewchan     chan string
	views        []string
	alert        []string
	warning      []string
	inView       bool
	inAlert      bool
	inWarning    bool
}

func (sens *StringSensor) Initialize(rm *RedmonManager) error {
	sens.collkillchan = make(chan struct{})
	sens.viewkillchan = make(chan struct{})
	sens.viewchan = make(chan string)
	go sens.runSensorCollector(rm)
	go sens.runSensorView(rm)
	return nil
}

func (sens *StringSensor) GetRefresh() uint16 {
	return sens.refresh
}

func (sens *StringSensor) GetSource() string {
	return sens.source.GetName()
}

func (sens *StringSensor) GetType() string {
	return "string"
}

func (sens *StringSensor) GetViews() []string {
	return sens.views
}

func (sens *StringSensor) GetViewChan() chan string {
	return sens.viewchan
}

func (sens *StringSensor) GetCollKillChan() chan struct{} {
	return sens.collkillchan
}

func (sens *StringSensor) GetViewKillChan() chan struct{} {
	return sens.viewkillchan
}

func (sens *StringSensor) GetValue() string {
	return sens.value
}

func (sens *StringSensor) GetAlert() bool {
	return sens.inAlert
}

func (sens *StringSensor) GetWarning() bool {
	return sens.inWarning
}

func (sens *StringSensor) GetAlerts() []string {
	return sens.alert
}

func (sens *StringSensor) GetWarnings() []string {
	return sens.warning
}

func (sens *StringSensor) runSensorCollector(rm *RedmonManager) {

	Debug("Goroutine for sensor collector " + sens.name + " started")

	g := rm.gui

	reqchan := sens.source.GetRequestChan()
	reschan := sens.source.GetResponseChan()
	killchan := sens.collkillchan

loop:
	for {
		select {
		case <-killchan:
			break loop
		case <-time.After(time.Duration(sens.refresh) * time.Second):

			// Skip all sensors operation if the system is updating
			if !rm.isConfChange {
				Info("Request for key \"" + yellow + sens.key + normal + "\"")
				newvalue := ""

				reqchan <- request{sens.key + ":suspended"}
				if response := <-reschan; response.ok {
					newvalue = "SUSPENDED"
					sens.inAlert = false
					sens.inWarning = false
				} else {
					reqchan <- request{sens.key}

					if response := <-reschan; response.ok {
						Info("Response for key \"" + yellow + sens.key + normal + "\" value: " + response.value)
						if response.value == "" {
							newvalue = "EMPTY"
						} else {
							newvalue = response.value
						}
					} else {
						Info("Response for key missing")
						newvalue = "MISSING"
					}

					sens.inAlert = redmonmel.ProcessConditions(sens.alert, newvalue)
					sens.inWarning = redmonmel.ProcessConditions(sens.warning, newvalue)
				}

				if newvalue != sens.value {
					sens.value = newvalue
					if *gui && (sens.inView || sens.inAlert || sens.inWarning) {
						g.Update(func(g *gocui.Gui) error {
							v, err := g.View(sens.name)

							if err == nil {
								w, _ := v.Size()
								v.Clear()
								if sens.inAlert {
									fmt.Fprint(v, putText(w, red, sens.value))
								} else if sens.inWarning {
									fmt.Fprint(v, putText(w, yellow, sens.value))
								} else {
									fmt.Fprint(v, putText(w, green, sens.value))
								}
							}
							return nil
						})
					}
				}
			}
		}
	}
	Debug("Goroutine for sensor " + sens.name + " stopped")
}

func (sens *StringSensor) runSensorView(rm *RedmonManager) {

	killchan := sens.viewkillchan

	Debug("Goroutine for sensor view " + sens.name + " started")

loop:
	for {
		select {
		case <-killchan:
			break loop
		case newview := <-sens.viewchan:
			if IsIn(sens.views, newview) {
				sens.inView = true
			} else {
				sens.inView = false
			}
		}
	}

	Debug("Goroutine for sensor view " + sens.name + " stopped")
}

//////////////////////////////////////////////////////////////////// Data sources interface

type DataSources map[string]DataSource

type DataSource interface {
	Initialize(string) error
	GetName() string
	GetType() string
	GetRequestChan() chan request
	GetResponseChan() chan response
	GetKillChan() chan struct{}
}

func (rm *RedmonManager) InitializeDataSources() error {

	d := new(DataSources)
	*d = make(map[string]DataSource)

	rm.sources = d
	c := rm.config

	// The configuration for the source is checked and eventually created
	multiple := make(map[string]struct{})
	for _, dsconf := range c.Sources {
		Info("Analyzing configured source " + dsconf.Name)
		name := dsconf.Name
		if _, exists := multiple[name]; exists {
			Alert("The data source " + name + " already processed in configuration")
		} else {
			stype := dsconf.Stype
			switch stype {
			case "redis":
				newsource := new(RedisSource)
				newsource.c = c
				newsource.name = name
				newsource.stype = stype
				if err := newsource.Initialize(name); err != nil {
					Alert(err)
				} else {
					Info("The data source " + name + " correctly configured")
					multiple[name] = struct{}{}
					(*d)[name] = newsource
				}
			default:
				Alert("Unknown source type " + stype)
			}
		}
	}
	return nil
}

//////////////////////////////////////////////////////////////////// REDIS data source

type RedisSource struct {
	c        *RedmonConfig
	name     string
	stype    string
	reqchan  chan request
	reschan  chan response
	killchan chan struct{}
}

func (ds *RedisSource) runSource(name string) {

	defer swg.Done()

	ds.name = name

	Debug("Goroutine for source " + ds.name + " started")

	Info("Connecting Redis server for source " + ds.name)

	// Connecting to redis
	var redmon_endpoint string
	var redmon_key string
	var redmon_db string

	for _, redconfig := range ds.c.Sources {
		if name == redconfig.Name {
			redmon_endpoint = redconfig.Endpoint
			redmon_key = redconfig.Key
			redmon_db = redconfig.Db
		}
	}

	c, err := redis.Dial("tcp", redmon_endpoint)
	if err == nil {
		Debug("Connection to Redis established")
	} else {
		Alert(err)
		return
	}
	defer c.Close()

	// Authentication
	if redmon_key != "" {
		if _, ok := c.Do("AUTH", redmon_key); ok == nil {
			Debug("Authentication to Redis succeded")
		} else {
			Alert("Authentication to Redis failed")
			return
		}
	}

	// Database selection

	if _, ok := c.Do("SELECT", redmon_db); ok == nil {
		Debug("Selected database " + redmon_db)
	} else {
		Alert("Database selection " + redmon_db + "Falied")
		return
	}

	reqchan := ds.reqchan
	reschan := ds.reschan
	killchan := ds.killchan

	Debug("Goroutine for source " + ds.name + " running")

	for {
		select {
		case <-killchan:
			return
		case req := <-reqchan:
			key := req.key
			if value, err := redis.String(c.Do("GET", key)); err == nil {
				reschan <- response{value, true}
			} else {
				reschan <- response{"", false}
			}
		}
	}
	c.Close()

	Debug("Goroutine for source " + ds.name + " stopped")
}

func (ds *RedisSource) Initialize(init string) error {
	ds.reqchan = make(chan request)
	ds.reschan = make(chan response)
	ds.killchan = make(chan struct{})

	swg.Add(1)
	go ds.runSource(init)

	return nil
}

func (ds *RedisSource) GetName() string {
	return ds.name
}

func (ds *RedisSource) GetType() string {
	return ds.stype
}

func (ds *RedisSource) GetRequestChan() chan request {
	return ds.reqchan
}

func (ds *RedisSource) GetResponseChan() chan response {
	return ds.reschan
}

func (ds *RedisSource) GetKillChan() chan struct{} {
	return ds.killchan
}

//////////////////////////////////////////////////////////////////// Log helpers

func Debug(logline interface{}) {
	if !*gui && *debug {
		log.Println("\033[35m[Debug]\033[0m -", logline)
	}
}

func Info(logline interface{}) {
	if !*gui && (*verbose || *debug) {
		log.Println("\033[32m[Info]\033[0m  -", logline)
	}
}

func Warning(logline interface{}) {
	if !*gui {
		log.Println("\033[33m[Warn]\033[0m  -", logline)
	}
}

func Alert(logline interface{}) {
	if !*gui {
		log.Println("\033[31m[Alert]\033[0m -", logline)
	}
}

//////////////////////////////////////////////////////////////////// Utils

func AppendIfMissing(slice []string, i string) ([]string, bool) {
	for _, ele := range slice {
		if ele == i {
			return slice, false
		}
	}
	return append(slice, i), true
}

func IsIn(slice []string, i string) bool {
	for _, ele := range slice {
		if ele == i {
			return true
		}
	}
	return false
}

//////////////////////////////////////////////////////////////////// Application specific data

var (
	swg         sync.WaitGroup
	verbose     = flag.Bool("v", false, "Verbose")
	debug       = flag.Bool("d", false, "Debug")
	configfile  = flag.String("c", "", "Configuration file (default is to use the embedded config)")
	gui         = flag.Bool("g", false, "Load gui (false)")
	useprom     = flag.Bool("p", false, "Open a prometheus exporter (false)")
	serve       = flag.Bool("s", false, "Serve a minimal web interface (false)")
	navig       = flag.Bool("n", false, "Navigation bar on the web interface (false)")
	edit        = flag.Bool("e", false, "Enable metric editing on the web interface (false)")
	templateDir = flag.String("t", "", "Allow the application of templates on the web interface (false)")
)

//////////////////////////////////////////////////////////////////// Main data struct

type RedmonManager struct {
	config         *RedmonConfig
	sources        *DataSources
	sensors        *Sensors
	gui            *gocui.Gui
	views          []string
	viewweights    map[string]int
	updatein       int
	currview       int
	isConfChange   bool
	isViewChange   *sync.Mutex
	killchan       chan struct{}
	changeviewchan chan int
	freeze         bool
}

// The UpdateViews function occurs: on program startup or on configuration changes
func (rm *RedmonManager) UpdateViews() error {

	Debug("Views array update started")

	newviews := make([]string, 0)
	newviewweigths := make(map[string]int)

	curr_view := 0
	curr_view_name := ""

	if len(rm.views) > 0 {
		curr_view = rm.currview
		curr_view_name = rm.views[rm.currview]
	}

	view_ok := false

	for _, sens := range *rm.sensors {
		for _, view := range sens.GetViews() {
			var ok bool
			newviews, ok = AppendIfMissing(newviews, view)
			if ok && view == curr_view_name {
				curr_view = len(newviews) - 1
				view_ok = true
			}
			if weight, ok := newviewweigths[view]; ok {
				newviewweigths[view] = weight + 1
			} else {
				newviewweigths[view] = 1
			}
		}
	}

	rm.views = newviews
	rm.viewweights = newviewweigths

	if !view_ok {
		curr_view = 0
	}

	rm.currview = curr_view
	//curr_view_name = rm.views[curr_view]

	for _, sens := range *rm.sensors {
		sens.GetViewChan() <- rm.views[curr_view]
	}

	Debug("Views array update finished")

	return nil
}

func (rm *RedmonManager) UpdateManager() {

	killchan := rm.killchan

	Debug("Update Manager started")

	// Start the first sensors sweep
	Debug("Update sensors started")
	rm.isConfChange = true

	rm.UpdateAutomaticSensors()

	rm.isConfChange = false
	Debug("Update sensors finished")

	rm.updatein = UPDATEINTERVAL

	for {
		select {
		case <-killchan:
			return
		case <-time.After(time.Second):

			if rm.updatein == 0 {
				rm.updatein = UPDATEINTERVAL
				Debug("Update sensors started")
				rm.isConfChange = true

				rm.UpdateAutomaticSensors()

				rm.isConfChange = false
				Debug("Update sensors finished")
			} else {
				rm.updatein = rm.updatein - 1
			}
		}
	}

	Debug("Update Manager stopped")

}

func (rm *RedmonManager) ViewsManager() {
	killchan := rm.killchan
	changeviewchan := rm.changeviewchan
	sensors := rm.sensors

	newview := "none"
	var autowait int
	var timer int

	for {
		if newview == "none" {
			autowait = 0
		} else {
			autowait = rm.viewweights[newview]
		}

		select {
		case <-killchan:
			return
		case howmany := <-changeviewchan:

			numviews := len(rm.views)
			numsens := len(*(rm.sensors))

			switch howmany {
			case 0:
				if rm.freeze {
					rm.freeze = false
				} else {
					rm.freeze = true
				}
			default:
				if !rm.isConfChange {

					rm.isViewChange.Lock()

					if rm.currview+howmany >= len(rm.views) {
						rm.currview = 0
					} else if rm.currview+howmany <= 0 {
						rm.currview = len(rm.views) - 1
					} else {
						rm.currview += howmany
					}

					timer = 0
					newview = "none"
					if numviews != 0 {
						newview = rm.views[rm.currview]
					}

					for _, sens := range *sensors {
						sens.GetViewChan() <- newview
					}

					rm.isViewChange.Unlock()
				}

			}

			rm.gui.Update(func(g *gocui.Gui) error {
				v, err := g.View("statusbar")
				if err == nil {
					v.Clear()
					update := ""
					freeze := ""
					if rm.isConfChange {
						update = red + "Updating Sensors" + normal + " - "
					}
					if rm.freeze {
						freeze = red + "Freezed" + normal + " - "
					}

					fmt.Fprintln(v, freeze+update+"Sensors:"+green, numsens, normal+"- Update in:"+green, strconv.Itoa(rm.updatein), normal+"- Views:"+green, numviews, normal+"- Current View:"+green, newview, normal+"- Change View in:"+green, strconv.Itoa(VIEWCHANGEINTERVAL+autowait-timer), normal)
				}
				return nil
			})

		case <-time.After(time.Second):

			// If the system is updating the sensors skip the view change
			if !rm.isConfChange {
				numviews := len(rm.views)
				numsens := len(*(rm.sensors))

				if !rm.freeze {
					if timer == VIEWCHANGEINTERVAL+autowait {

						rm.isViewChange.Lock()

						if rm.currview+1 == len(rm.views) {
							rm.currview = 0
						} else {
							rm.currview += 1
						}

						timer = 0
						newview = "none"
						if numviews != 0 {
							newview = rm.views[rm.currview]
						}

						for _, sens := range *sensors {
							sens.GetViewChan() <- newview
						}

						rm.isViewChange.Unlock()
					} else {
						timer++
					}
				}
				rm.gui.Update(func(g *gocui.Gui) error {
					v, err := g.View("statusbar")
					if err == nil {
						v.Clear()
						update := ""
						freeze := ""
						if rm.isConfChange {
							update = red + "Updating Sensors" + normal + " - "
						}
						if rm.freeze {
							freeze = red + "Freezed" + normal + " - "
						}

						fmt.Fprintln(v, freeze+update+"Sensors:"+green, numsens, normal+"- Update in:"+green, strconv.Itoa(rm.updatein), normal+"- Views:"+green, numviews, normal+"- Current View:"+green, newview, normal+"- Change View in:"+green, strconv.Itoa(VIEWCHANGEINTERVAL+autowait-timer), normal)
					}
					return nil
				})
			}
		}
	}
}

//////////////////////////////////////////////////////////////////// Gui specific

func (rm *RedmonManager) Layout(g *gocui.Gui) error {

	rm.isViewChange.Lock()
	maxX, _ := g.Size()
	sensors := *rm.sensors

	numviews := len(rm.views)

	currviewname := "none"

	if numviews != 0 {
		currviewname = rm.views[int(rm.currview)]
	}

	if _, err := g.SetView("statusbar", 0, 0, maxX-19, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}

	if v, err := g.SetView("prevview", maxX-18, 0, maxX-13, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		fmt.Fprintln(v, "Prev")
	}
	if v, err := g.SetView("stopview", maxX-12, 0, maxX-7, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		fmt.Fprintln(v, "Stop")
	}

	if v, err := g.SetView("nextview", maxX-6, 0, maxX-1, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		fmt.Fprintln(v, "Next")
	}

	currX := 0
	currY := 3
	currL := 0

	alertkeys := make([]string, 0)
	warnkeys := make([]string, 0)
	shownkeys := make([]string, 0)

	for sensname, sens := range sensors {
		if sens.GetAlert() {
			alertkeys = append(alertkeys, sensname)
		} else if sens.GetWarning() {
			warnkeys = append(warnkeys, sensname)
		} else if IsIn(sens.GetViews(), currviewname) {
			shownkeys = append(shownkeys, sensname)
		}
	}

	shown := make(map[string]int)
	sline := make(map[string]int)

	shownidx := make([]string, len(alertkeys)+len(warnkeys)+len(shownkeys))

	sort.Strings(alertkeys)
	sort.Strings(warnkeys)
	sort.Strings(shownkeys)

	for i, sensname := range alertkeys {
		sens := sensors[sensname]
		newvalue := sens.GetValue()
		w := len(newvalue) + VPAD
		if w < len(sensname)+WPAD {
			w = len(sensname) + WPAD
		}

		if currX+w >= maxX {
			currL = currL + 1
			currX = 0
		}

		currX = currX + w

		shown[sensname] = w
		sline[sensname] = currL

		shownidx[i] = sensname

	}

	for i, sensname := range warnkeys {
		sens := sensors[sensname]
		newvalue := sens.GetValue()
		w := len(newvalue) + VPAD
		if w < len(sensname)+WPAD {
			w = len(sensname) + WPAD
		}

		if currX+w >= maxX {
			currL = currL + 1
			currX = 0
		}

		currX = currX + w

		shown[sensname] = w
		sline[sensname] = currL

		shownidx[len(alertkeys)+i] = sensname

	}

	for i, sensname := range shownkeys {
		sens := sensors[sensname]
		newvalue := sens.GetValue()
		w := len(newvalue) + VPAD
		if w < len(sensname)+WPAD {
			w = len(sensname) + WPAD
		}

		if currX+w >= maxX {
			currL = currL + 1
			currX = 0
		}

		currX = currX + w

		shown[sensname] = w
		sline[sensname] = currL

		shownidx[len(alertkeys)+len(warnkeys)+i] = sensname

	}

	linew := make([]int, currL+1)

	for i := 0; i < currL+1; i++ {
		linew[i] = 0
	}

	for sensname, line := range sline {
		linew[line] = linew[line] + shown[sensname]
	}

	for {
		doneany := false
		for _, sensname := range shownidx {
			line := sline[sensname]
			if line != currL && linew[line] < maxX {
				doneany = true
				linew[line] = linew[line] + 1
				shown[sensname] = shown[sensname] + 1
			}
		}
		if !doneany {
			break
		}
	}

	for sensname, _ := range sensors {
		if _, ok := shown[sensname]; !ok {
			if v, err := g.View(sensname); err == nil {
				v.Clear()
				g.DeleteView(sensname)
			}
		}
	}
	currX = 0
	currL = 0

	for _, sensname := range alertkeys {
		sens := sensors[sensname]
		newvalue := sens.GetValue()
		w := shown[sensname]

		if sline[sensname] != currL {
			currX = 0
			currY = currY + DEFAULTH
			currL = sline[sensname]
		}

		if v, err := g.SetView(sensname, currX, currY, currX+w-1, currY+DEFAULTH-1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = sensname
			fmt.Fprint(v, putText(w, red, newvalue))
		}

		currX = currX + w
	}

	for _, sensname := range warnkeys {
		sens := sensors[sensname]
		newvalue := sens.GetValue()
		w := shown[sensname]

		if sline[sensname] != currL {
			currX = 0
			currY = currY + DEFAULTH
			currL = sline[sensname]
		}

		if v, err := g.SetView(sensname, currX, currY, currX+w-1, currY+DEFAULTH-1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = sensname
			fmt.Fprint(v, putText(w, yellow, newvalue))
		}

		currX = currX + w
	}

	for _, sensname := range shownkeys {
		sens := sensors[sensname]
		newvalue := sens.GetValue()
		w := shown[sensname]

		if sline[sensname] != currL {
			currX = 0
			currY = currY + DEFAULTH
			currL = sline[sensname]
		}

		if v, err := g.SetView(sensname, currX, currY, currX+w-1, currY+DEFAULTH-1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = sensname
			fmt.Fprint(v, putText(w, green, newvalue))
		}

		currX = currX + w
	}

	rm.isViewChange.Unlock()
	return nil
}

func keybindings(rm *RedmonManager, g *gocui.Gui) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, rm.quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("prevview", gocui.MouseLeft, gocui.ModNone, rm.prev); err != nil {
		return err
	}
	if err := g.SetKeybinding("stopview", gocui.MouseLeft, gocui.ModNone, rm.stop); err != nil {
		return err
	}
	if err := g.SetKeybinding("nextview", gocui.MouseLeft, gocui.ModNone, rm.next); err != nil {
		return err
	}
	return nil
}

func (rm *RedmonManager) quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func (rm *RedmonManager) prev(g *gocui.Gui, v *gocui.View) error {
	rm.changeviewchan <- -1
	return nil
}

func (rm *RedmonManager) stop(g *gocui.Gui, v *gocui.View) error {
	rm.changeviewchan <- 0
	return nil
}

func (rm *RedmonManager) next(g *gocui.Gui, v *gocui.View) error {
	rm.changeviewchan <- 1
	return nil
}

//////////////////////////////////////////////////////////////////// Initializzation

func init() {
	flag.Parse()
}

//////////////////////////////////////////////////////////////////// Main

func main() {
	var config *RedmonConfig

	if *configfile != "" {
		Debug("Loading configuration from the file \"" + *configfile + "\"")
		content, err := ioutil.ReadFile(*configfile)
		if err != nil {
			Alert(err)
		}

		config = new(RedmonConfig)

		json.Unmarshal(content, config)

		Debug("Configuration loaded: " + config.String())
	} else {
		Debug("Loading configuration from the embedded configuration")

		config = new(RedmonConfig)
		config.Sources = make([]struct {
			Name     string `json:"name"`
			Stype    string `json:"type"`
			Endpoint string `json:"endpoint"`
			Key      string `json:"key"`
			Db       string `json:"db"`
		}, len(embredmon.Config.Sources))

		for i, source := range embredmon.Config.Sources {
			config.Sources[i].Name = source.Name
			config.Sources[i].Stype = source.Type
			config.Sources[i].Endpoint = source.Endpoint
			config.Sources[i].Key = source.Key
			config.Sources[i].Db = source.Db
		}

		Debug("Configuration loaded: " + config.String())
	}

	var g *gocui.Gui
	if *gui {
		Debug("Loading gui")
		gg, err := gocui.NewGui(gocui.OutputNormal)
		if err != nil {
			log.Panicln(err)
		} else {
			g = gg
			defer g.Close()
		}
	}

	rm := new(RedmonManager)
	rm.killchan = make(chan struct{})
	rm.changeviewchan = make(chan int)
	rm.config = config
	rm.gui = g
	rm.views = make([]string, 0)
	rm.currview = 0
	rm.isViewChange = new(sync.Mutex)
	rm.isConfChange = false

	rm.InitializeDataSources()

	rm.InitializeSensors()

	rm.UpdateViews()

	go rm.UpdateManager()

	if g != nil {

		go rm.ViewsManager()

		g.SetManager(rm)

		g.Mouse = true

		if err := keybindings(rm, g); err != nil {
			log.Panicln(err)
		}

		if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
			log.Panicln(err)
		}
	} else if *useprom {
		c := RedmonCollector{}
		c.rm = rm
		prometheus.MustRegister(c)
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":8000", nil))
	} else if *serve {
		http.HandleFunc("/view", rm.viewHandler)
		http.HandleFunc("/views", rm.viewsHandler)
		if *edit && *templateDir != "" {
			http.HandleFunc("/missing", rm.missing)
			http.HandleFunc("/template", rm.sensorEdit)
		}
		log.Fatal(http.ListenAndServe(":8080", nil))
	} else {
		swg.Wait()

	}
}
