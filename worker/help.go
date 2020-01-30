package main

import (
	"flag"
	"fmt"
	"os"

	tshttp "github.com/tsunami/libs"
)

// Usage information for using app
func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
}

// ReadConf read configruation
func ReadConf() tshttp.Conf {
	var url = flag.String("url", "", "URL : http[s]://<hostname>[:port]/<uri>")
	var host = flag.String("host", "", "hostname ex. example.com")
	var port = flag.String("port", "80", "listen port")
	var path = flag.String("uri", "/", "uri path")
	var concurrence = flag.Int("concurrence", 10, "number of clients, which send request to the tagets simultaneously")
	var maxQueues = flag.Int("maxQueues", 60000, "maxQueues are wating job generated controlled by master node")
	var maxConns = flag.Int("maxConns", 1024, "maxConns are number of connections")

	flag.Parse()

	if *url == "" && *host == "" {
		Usage()
		os.Exit(1)
	}

	return tshttp.Conf{URL: *url,
		Host:        *host,
		Port:        *port,
		Path:        *path,
		Concurrence: *concurrence,
		MaxQueues:   *maxQueues,
		MaxConns:    *maxConns}
}
