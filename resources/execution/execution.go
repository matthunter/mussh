package execution

import (
	"code.google.com/p/go.crypto/ssh"
	"github.com/dancannon/gorethink"
	"github.com/gorilla/websocket"
	"github.com/martini-contrib/render"
	"mussh/resources/command"
	"mussh/resources/server"
	"net/http"
	"sync"
)

type Execution struct {
	Id        string      `gorethink:"id,omitempty" json:"id"`
	Username  string      `json:"username" binding:"required"`
	Password  string      `json:"password" binding:"required"`
	GroupId   string      `json:"groupId" binding:"required"`
	CommandId string      `json:"commandId" binding:"required"`
	Results   []SshResult `json:"results"`
}

type SshResult struct {
	Server  server.Server `json:"server"`
	Output  string        `json:"output"`
	Success bool          `json:"success"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func Get(rw http.ResponseWriter, rq *http.Request, session *gorethink.Session, r render.Render) {
	ws, err := upgrader.Upgrade(rw, rq, nil)
	if err != nil {
		r.Status(http.StatusBadRequest)
	}
	defer ws.Close()

	var exe Execution
	if err = ws.ReadJSON(&exe); err != nil {
		r.Status(http.StatusBadRequest)
	}

	handleRequest(session, exe, ws)
}

func handleRequest(session *gorethink.Session, exe Execution, ws *websocket.Conn) {
	servers := getServers(session, exe.GroupId)
	cmd := getCommand(session, exe.CommandId)
	config := getConfig(exe)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.ReadMessage()
	}()

	sshResultChans := make([]<-chan SshResult, len(servers))
	for i, svr := range servers {
		sshResultChans[i] = runCommand(svr, cmd, config, done)
	}

	output := merge(sshResultChans...)
	for sshResult := range output {
		err := ws.WriteJSON(sshResult)
		if err != nil {
			return
		}
	}
}

func merge(chans ...<-chan SshResult) <-chan SshResult {
	var wg sync.WaitGroup
	out := make(chan SshResult)
	wg.Add(len(chans))

	for _, c := range chans {
		go func(c <-chan SshResult) {
			for n := range c {
				out <- n
			}
			wg.Done()
		}(c)
	}

	go func() {
		defer close(out)
		wg.Wait()
	}()

	return out
}

func getServers(session *gorethink.Session, groupId string) []server.Server {
	rows, _ := gorethink.Table("server").Filter(
		func(serverTerm gorethink.RqlTerm) gorethink.RqlTerm {
			return serverTerm.Field("GroupIds").Contains(groupId)
		}).Run(session)
	var servers []server.Server
	rows.ScanAll(&servers)
	return servers
}

func getCommand(session *gorethink.Session, commandId string) command.Command {
	row, _ := gorethink.Table("command").Get(commandId).RunRow(session)
	var command command.Command
	row.Scan(&command)
	return command
}

func getConfig(exe Execution) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: exe.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(exe.Password),
		},
	}
}
