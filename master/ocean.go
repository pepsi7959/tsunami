package main

import tshttp "github.com/tsunami/libs"

//APIVersion version of APIs
var APIVersion = "/api/v1"

// Conf configuruation
type Conf struct {
}

// Ocean structure
type Ocean struct {
	conf Conf
}

// Oceans keep all information regarding to user request
type Oceans struct {
	Jobs map[string]*Ocean
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
