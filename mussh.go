package main

import (
	"github.com/dancannon/gorethink"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/binding"
	"github.com/martini-contrib/render"
	"log"
	"mussh/resources/command"
	"mussh/resources/execution"
	"mussh/resources/group"
	"mussh/resources/server"
	"net/http"
)

func main() {
	session := connectToDb()

	m := martini.Classic()
	m.Use(render.Renderer())
	m.Use(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Add("Access-Control-Allow-Origin", "*")
		res.Header().Add("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	})
	m.Map(session)
	m.Action(getRouter())

	log.Println("listening on port: 7979")
	http.ListenAndServe(":7979", m)
	// if err := http.ListenAndServeTLS(":7979", "cert.pem", "key.pem", m); err != nil {
	// 	log.Fatal(err)
	// }
}

func connectToDb() *gorethink.Session {
	session, err := gorethink.Connect(gorethink.ConnectOpts{
		Address:  "localhost:28015",
		Database: "mussh",
	})
	if err != nil {
		log.Fatal(err)
	}

	return session
}

func getRouter() martini.Handler {
	r := martini.NewRouter()

	r.Get("/servers", server.Get)
	r.Post("/servers", binding.Bind(server.Server{}), server.Post)
	r.Delete("/servers/:id", server.Delete)

	r.Get("/groups", group.GetWithServer)
	r.Post("/groups", binding.Bind(group.Group{}), group.Post)
	r.Delete("/groups/:id", group.Delete)

	r.Get("/commands", command.Get)
	r.Post("/commands", binding.Bind(command.Command{}), command.Post)
	r.Delete("/commands/:id", command.Delete)

	r.Get("/executions", execution.Get)

	r.Options("**", func(r render.Render) {
		r.Status(http.StatusOK)
	})

	return r.Handle
}
