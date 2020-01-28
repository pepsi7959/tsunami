package main

//Conf configuration structure
type Conf struct {
	name        string
	url         string
	host        string
	port        string
	path        string
	method      string
	headers     map[string]string
	body        string
	concurrence int
	maxConns    int
	maxQueues   int
}
