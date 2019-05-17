package main

/* Configuration */
type Conf struct {
	url         string
	host        string "localhost"
	port        string "80"
	path        string "/"
	method      string "GET"
	headers     map[string]string
	body        string
	concurrence int
	maxConns    int
	maxQueues   int
}
