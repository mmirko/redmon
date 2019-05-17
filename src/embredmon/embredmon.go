package embredmon

type AutoGenerated struct {
	Sources []Sources `json:"sources"`
}
type Sources struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Endpoint     string `json:"endpoint"`
	Key          string `json:"key"`
	Db           string `json:"db"`
	ScriptPrefix string `json:"scriptprefix"`
}

var Config = AutoGenerated{Sources: []Sources{Sources{Name: "redmon", Type: "redis", Endpoint: "localhost:6379", Key: "", Db: "2", ScriptPrefix: ""}}}
