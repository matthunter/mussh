package group

import (
	"github.com/dancannon/gorethink"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/render"
	"net/http"
)

const TABLE string = "group"

type Group struct {
	Id   string `gorethink:"id,omitempty" json:"id"`
	Name string `json:"name" form:"name" binding:"required"`
}

type GroupServer struct {
	Id      string `gorethink:"id,omitempty" json:"id"`
	Name    string `json:"name"`
	Servers []struct {
		Id      string `gorethink:"id,omitempty" json:"id" form:"id"`
		Name    string `json:"name" form:"name"`
		Addr    string `json:"addr" form:"addr"`
		Port    int    `json:"port" form:"port"`
		Tunnel  string `json:"tunnel" form:"tunnel"`
		BaseDir string `json:"basedir" form:"basedir"`
	} `json:"servers"`
}

func GetWithServer(session *gorethink.Session, r render.Render) {
	rows, _ := gorethink.Table(TABLE).Map(func(groupTerm gorethink.RqlTerm) gorethink.RqlTerm {
		return groupTerm.Merge(map[string]interface{}{"servers": gorethink.Table("server").Filter(
			func(serverTerm gorethink.RqlTerm) gorethink.RqlTerm {
				return serverTerm.Field("GroupIds").Contains(groupTerm.Field("id"))
			}).Without("GroupIds").CoerceTo("array")})
	}).Run(session)

	var groupServer []GroupServer
	rows.ScanAll(&groupServer)
	r.JSON(http.StatusOK, groupServer)
}

func Get(session *gorethink.Session, r render.Render) {
	rows, _ := gorethink.Table(TABLE).Run(session)
	var groups []Group
	rows.ScanAll(&groups)
	r.JSON(http.StatusOK, groups)
}

func Post(session *gorethink.Session, r render.Render, grp Group) {
	response, _ := gorethink.Table(TABLE).Insert(grp).RunWrite(session)
	r.JSON(http.StatusOK, response.GeneratedKeys)
}

func Delete(session *gorethink.Session, r render.Render, params martini.Params) {
	gorethink.Table(TABLE).Get(params["id"]).Delete().RunWrite(session)
	r.Status(http.StatusOK)
}
