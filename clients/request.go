package main

// google style guide
// https://google.github.io/styleguide/

// Conf structure
type CmdConf struct {
	Name        string `json:"name"`
	Url         string `json:"url"`
	Concurrence int    `json:"concurrence"`
	Host        string `json:"host"`
}

// Requst structure
type Request struct {
	Cmd     string  `json:"cmd"`
	CmdConf CmdConf `json:"conf"`
}
