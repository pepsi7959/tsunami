package main

/* Configuration */
type Conf struct {
	url         string
	host        string "localhost"
	port        string "80"
	path        string "/"
	concurrence int
	maxConns    int
	maxQueues   int
}
