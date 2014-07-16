package command

import (
	"github.com/dancannon/gorethink"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/render"
	"net/http"
)

const TABLE string = "command"

type Command struct {
	Id     string `gorethink:"id,omitempty" json:"id" form:"id"`
	Name   string `json:"name" form:"name" binding:"required"`
	Note   string `json:"note" form:"note"`
	Script string `json:"script" form:"script" binding:"required"`
}

func Get(session *gorethink.Session, r render.Render) {
	rows, _ := gorethink.Table(TABLE).Run(session)
	var commands []Command
	rows.ScanAll(&commands)
	r.JSON(http.StatusOK, commands)
}

func Post(session *gorethink.Session, r render.Render, cmd Command) {
	response, _ := gorethink.Table(TABLE).Insert(cmd).RunWrite(session)
	r.JSON(http.StatusOK, response.GeneratedKeys)
}

func Delete(session *gorethink.Session, r render.Render, params martini.Params) {
	gorethink.Table(TABLE).Get(params["id"]).Delete().RunWrite(session)
	r.Status(http.StatusOK)
}
