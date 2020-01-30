package main

import tshttp "github.com/tsunami/libs"

//APIVersion version of APIs
var APIVersion = "/api/v1"

// Ocean structure
type Ocean struct {
	conf tshttp.Conf
}

//Worker information of worker
type Worker struct {
	//endpoint is "ip:port"
	endpoint string
	name     string
}

// Oceans keep all information regarding to user request
type Oceans struct {
	workers []Worker
	Jobs    map[string]*Ocean
}

func main() {
	ocs := Oceans{Jobs: make(map[string]*Ocean)}
	app := &tshttp.App{}
	app.Init("8080")
	app.AddAPI(APIVersion+"/start", ocs.Start)
	app.AddAPI(APIVersion+"/stop", ocs.Stop)
	app.AddAPI(APIVersion+"/metrics", ocs.GetMetrics)
	app.Run()
}
