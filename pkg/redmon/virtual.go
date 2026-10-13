package redmon

type VirtualSensorConfig struct {
	Name     string   `json:"name"`
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Targets  []string `json:"targets"`
	Interval uint16   `json:"interval"`
}
