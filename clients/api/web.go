package api

import (
	"fmt"
	"net/http"
	"time"
)

type App struct {
	Server   http.Server
	ServeMux *http.ServeMux
	Handler  http.Handler
}

// Help
func help(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "{}")
}

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

func (app *App) AddApi(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	app.ServeMux.HandleFunc(pattern, handler)
}

func (app *App) setRoute() {
	app.ServeMux = http.NewServeMux()
	app.ServeMux.HandleFunc("/help", help)
}

func (app *App) Run() {
	fmt.Println("Listening: ", app.Server.Addr)
	app.Server.ListenAndServe()
}

func (app *App) Stop() {
	fmt.Println("stop server: ", app.Server.Addr)
	app.Server.Close()
}
