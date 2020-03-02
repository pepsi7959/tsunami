package tshttp

// google style guide
// https://google.github.io/styleguide/

// CmdConf command structure
type CmdConf struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	Concurrence int               `json:"concurrence"`
	Host        string            `json:"host"`
	Port        string            `json:"port"`
	Path        string            `json:"path"`
	Protocol    string            `json:"protocol"`
}

// Request sturcture
type Request struct {
	Cmd  string  `json:"cmd"`
	Conf CmdConf `json:"conf"`
}
