package tshttp

import (
	"fmt"
	"net/http"
	"time"
)

//App application structure
type App struct {
	Server   http.Server
	ServeMux *http.ServeMux
	Handler  http.Handler
}

// help display usage information
func help(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "{}")
}

//Init intilize server
func (app *App) Init(port string) {

	app.setRoute()

	app.Server = http.Server{
		Addr:           ":" + port,
		Handler:        app.ServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

}

//AddAPI app new api to the router
func (app *App) AddAPI(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	app.ServeMux.HandleFunc(pattern, handler)
}

//SetRoute set up the router
func (app *App) setRoute() {
	app.ServeMux = http.NewServeMux()
	app.ServeMux.HandleFunc("/help", help)
}

//Run start server
func (app *App) Run() {
	fmt.Println("Listening: ", app.Server.Addr)
	app.Server.ListenAndServe()
}

//Stop stop server
func (app *App) Stop() {
	fmt.Println("stop server: ", app.Server.Addr)
	app.Server.Close()
}
