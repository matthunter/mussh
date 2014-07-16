package server

import (
	"github.com/dancannon/gorethink"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/render"
	"net/http"
)

const TABLE string = "server"

type Server struct {
	Id       string   `gorethink:"id,omitempty" json:"id" form:"id"`
	Name     string   `json:"name" form:"name" binding:"required"`
	Addr     string   `json:"addr" form:"addr" binding:"required"`
	Port     int      `json:"port" form:"port" binding:"required"`
	Tunnel   string   `json:"tunnel" form:"tunnel"`
	BaseDir  string   `json:"basedir" form:"basedir"`
	GroupIds []string `json:"groupIds" form:"groupIds" binding:"required"`
}

func Get(session *gorethink.Session, r render.Render) {
	rows, _ := gorethink.Table(TABLE).Run(session)
	var servers []Server
	rows.ScanAll(&servers)
	r.JSON(http.StatusOK, servers)
}

func Post(session *gorethink.Session, r render.Render, svr Server) {
	response, _ := gorethink.Table(TABLE).Insert(svr).RunWrite(session)
	r.JSON(http.StatusOK, response.GeneratedKeys)
}

func Delete(session *gorethink.Session, r render.Render, params martini.Params) {
	gorethink.Table(TABLE).Get(params["id"]).Delete().RunWrite(session)
	r.Status(http.StatusOK)
}
