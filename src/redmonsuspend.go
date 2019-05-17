package main

import (
	"embredmon"
	"encoding/json"
	"flag"
	"github.com/gomodule/redigo/redis"
	"log"
	"os"
	"strings"
)

const (
	green  = "\033[32m"
	red    = "\033[31m"
	yellow = "\033[33m"
	normal = "\033[0m"
)

//////////////////////////////////////////////////////////////////// Data struct

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

//////////////////////////////////////////////////////////////////// Log helpers

func Debug(logline interface{}) {
	if *debug {
		log.Println("\033[35m[Debug]\033[0m -", logline)
	}
}

func Info(logline interface{}) {
	if *verbose || *debug {
		log.Println("\033[32m[Info]\033[0m  -", logline)
	}
}

func Warning(logline interface{}) {
	log.Println("\033[33m[Warn]\033[0m  -", logline)
}

func Alert(logline interface{}) {
	log.Println("\033[31m[Alert]\033[0m -", logline)
}

//////////////////////////////////////////////////////////////////// Application specific data

var (
	verbose = flag.Bool("v", false, "Verbose")
	debug   = flag.Bool("d", false, "Debug")

	configfile = flag.String("c", "", "Use a config file (default is to use the embedded config)")

	fullkey   = flag.String("k", "", "Key (alternate + metric) to consider")
	metric    = flag.String("m", "", "Metric to consider")
	alternate = flag.String("a", "", "Specify an alterate metric prefix other than the hostname")

	on      = flag.Bool("on", false, "Suspend a sensor")
	off     = flag.Bool("off", false, "Remove suspension")
	mswitch = flag.Bool("s", false, "Switch suspension")
)

//////////////////////////////////////////////////////////////////// Initializzation

func init() {
	flag.Parse()
}

//////////////////////////////////////////////////////////////////// Main

func main() {

	// Connecting to redis
	var redmon_endpoint string
	var redmon_key string
	var redmon_db string

	if *configfile == "" {
		redmon_endpoint = embredmon.Config.Sources[0].Endpoint
		redmon_key = embredmon.Config.Sources[0].Key
		redmon_db = embredmon.Config.Sources[0].Db
		// Use the embedded configuration
	} else {
		Alert("Config file unimplemented")
		os.Exit(2)
	}

	c, err := redis.Dial("tcp", redmon_endpoint)
	if err == nil {
		Debug("Connection to Redis established")
	} else {
		Alert(err)
		os.Exit(2)
	}
	defer c.Close()

	// Authentication

	if _, ok := c.Do("AUTH", redmon_key); ok == nil {
		Debug("Authentication to Redis succeded")
	} else {
		Alert("Authentication to Redis failed")
		os.Exit(2)
	}

	// Database selection

	if _, ok := c.Do("SELECT", redmon_db); ok == nil {
		Debug("Selected database " + redmon_db)
	} else {
		Alert("Database selection " + redmon_db + "Falied")
		os.Exit(2)
	}

	var key string

	if *fullkey != "" {
		key = *fullkey
		if *alternate != "" {
			Warning("Alternate value will be ignored because the key has been specified")
		}
		if *metric != "" {
			Warning("Metric value will be ignored because the key has been specified")
		}
	} else {
		if *alternate != "" {
			key = *alternate + ":" + *metric
		} else {
			if hostname, err := os.Hostname(); err != nil {
				Alert("Hostname Falied")
				os.Exit(2)
			} else {
				key = hostname + ":" + *metric
			}
		}
	}

	Info("Key is " + key)

	switch {
	case *on:
		confkeys, err := redis.Strings(c.Do("KEYS", key+"*:redmon*"))
		if err != nil {
			Alert("KEYS Falied")
			os.Exit(2)
		}
		for _, confkey := range confkeys {
			if value, err := redis.String(c.Do("GET", confkey)); err != nil {
				Warning("Load sensor configuration failed for \"" + yellow + confkey + normal + "\"")
			} else {
				if !json.Valid([]byte(value)) {
					Warning("Invalid input json for \"" + yellow + confkey + normal + "\"")
				} else {
					sensor := new(SensorConfig)
					if err := json.Unmarshal([]byte(value), sensor); err != nil {
						Warning("Loaded json does not fit into sensorconfig data structure for \"" + yellow + confkey + normal + "\"")
					} else {
						datakey := sensor.Key
						Debug("The data key for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + datakey + normal + "\"")
						splitted := strings.Split(datakey, ":")
						metric := strings.Join(splitted[1:], ":")
						Debug("The metric for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + metric + normal + "\"")

						if _, err := c.Do("SET", datakey+":suspended", "yes"); err != nil {
							Warning("Suspension on key \"" + yellow + confkey + normal + "\" failed")
						} else {
							Debug("Suspension on key \"" + yellow + confkey + normal + "\" succeded")
						}
					}
				}
			}
		}
	case *off:
		confkeys, err := redis.Strings(c.Do("KEYS", key+"*:redmon*"))
		if err != nil {
			Alert("KEYS Falied")
			os.Exit(2)
		}
		for _, confkey := range confkeys {
			if value, err := redis.String(c.Do("GET", confkey)); err != nil {
				Warning("Load sensor configuration failed for \"" + yellow + confkey + normal + "\"")
			} else {
				if !json.Valid([]byte(value)) {
					Warning("Invalid input json for \"" + yellow + confkey + normal + "\"")
				} else {
					sensor := new(SensorConfig)
					if err := json.Unmarshal([]byte(value), sensor); err != nil {
						Warning("Loaded json does not fit into sensorconfig data structure for \"" + yellow + confkey + normal + "\"")
					} else {
						datakey := sensor.Key
						Debug("The data key for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + datakey + normal + "\"")
						splitted := strings.Split(datakey, ":")
						metric := strings.Join(splitted[1:], ":")
						Debug("The metric for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + metric + normal + "\"")

						if _, err := c.Do("DEL", datakey+":suspended"); err != nil {
							Warning("Suspension removal on key \"" + yellow + confkey + normal + "\" failed")
						} else {
							Debug("Suspension removal on key \"" + yellow + confkey + normal + "\" succeded")
						}
					}
				}
			}
		}
	case *mswitch:
		confkeys, err := redis.Strings(c.Do("KEYS", key+"*:redmon*"))
		if err != nil {
			Alert("KEYS Falied")
			os.Exit(2)
		}
		for _, confkey := range confkeys {
			if value, err := redis.String(c.Do("GET", confkey)); err != nil {
				Warning("Load sensor configuration failed for \"" + yellow + confkey + normal + "\"")
			} else {
				if !json.Valid([]byte(value)) {
					Warning("Invalid input json for \"" + yellow + confkey + normal + "\"")
				} else {
					sensor := new(SensorConfig)
					if err := json.Unmarshal([]byte(value), sensor); err != nil {
						Warning("Loaded json does not fit into sensorconfig data structure for \"" + yellow + confkey + normal + "\"")
					} else {
						datakey := sensor.Key
						Debug("The data key for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + datakey + normal + "\"")
						splitted := strings.Split(datakey, ":")
						metric := strings.Join(splitted[1:], ":")
						Debug("The metric for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + metric + normal + "\"")

						if _, err := redis.String(c.Do("GET", datakey+":suspended")); err != nil {
							Debug("Moving \"" + yellow + confkey + normal + "\" from unsuspended to suspended")
							if _, err := c.Do("SET", datakey+":suspended", "yes"); err != nil {
								Warning("Suspension on key \"" + yellow + confkey + normal + "\" failed")
							} else {
								Debug("Suspension on key \"" + yellow + confkey + normal + "\" succeded")
							}
						} else {
							Debug("Moving \"" + yellow + confkey + normal + "\" from suspended to unsuspended")
							if _, err := c.Do("DEL", datakey+":suspended"); err != nil {
								Warning("Suspension removal on key \"" + yellow + confkey + normal + "\" failed")
							} else {
								Debug("Suspension removal on key \"" + yellow + confkey + normal + "\" succeded")
							}
						}
					}
				}
			}
		}
	default:
		Alert("Operating mode missing")
		os.Exit(2)
	}
}
