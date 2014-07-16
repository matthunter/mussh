package execution

import (
	"code.google.com/p/go.crypto/ssh"
	"errors"
	"github.com/dancannon/gorethink"
	"github.com/gorilla/websocket"
	"github.com/martini-contrib/render"
	"io"
	"mussh/resources/command"
	"mussh/resources/server"
	"net/http"
	"strconv"
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

func runCommand(svr server.Server, cmd command.Command, config *ssh.ClientConfig, done <-chan struct{}) <-chan SshResult {
	out := make(chan SshResult)

	go func() {
		client, err := createClient(config, svr)
		if err != nil {
			out <- SshResult{svr, err.Error(), false}
			close(out)
			return
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			out <- SshResult{svr, "Failed to create SSH session for server: " + svr.Addr + "\n\n" + err.Error(), false}
			close(out)
			return
		}
		defer session.Close()

		stdoutPipe, _ := session.StdoutPipe()
		stderrPipe, _ := session.StderrPipe()

		go handleOutput(out, done, stdoutPipe, stderrPipe, svr)

		run := make(chan struct{})
		go func() {
			defer close(run)
			session.Run(buildCommand(cmd, svr))
		}()
		select {
		case <-run:
		case <-done:
		}
	}()

	return out
}

func handleOutput(out chan<- SshResult, done <-chan struct{}, stdoutPipe io.Reader, stderrPipe io.Reader, svr server.Server) {
	defer close(out)

	var wg sync.WaitGroup
	pipe := make(chan SshResult)
	wg.Add(2)
	go func() {
		defer close(pipe)
		wg.Wait()
	}()

	go readFromPipe(&wg, pipe, stdoutPipe, false, svr)
	go readFromPipe(&wg, pipe, stderrPipe, true, svr)

	for {
		select {
		case sshResult, ok := <-pipe:
			if !ok {
				return
			}
			out <- sshResult
		case <-done:
			return
		}
	}
}

func readFromPipe(wg *sync.WaitGroup, out chan<- SshResult, pipe io.Reader, isErr bool, svr server.Server) {
	defer wg.Done()
	buffer := make([]byte, 4096)
	for {
		n, err := pipe.Read(buffer)
		if err != nil {
			return
		}
		out <- SshResult{svr, string(buffer[:n]), !isErr}
	}
}

func buildCommand(cmd command.Command, svr server.Server) string {
	script := cmd.Script
	if svr.BaseDir != "" {
		script = "cd " + svr.BaseDir + " && " + script
	}
	return script
}

func createClient(config *ssh.ClientConfig, svr server.Server) (*ssh.Client, error) {
	var client *ssh.Client
	if svr.Tunnel != "" {
		tunnelClient, err := ssh.Dial("tcp", svr.Tunnel+":"+strconv.Itoa(svr.Port), config)
		if err != nil {
			return nil, errors.New("Failed to dial tunnel address: " + svr.Tunnel + "\n\n" + err.Error())
		}
		tunnelConnection, err := tunnelClient.Dial("tcp", svr.Addr+":"+strconv.Itoa(svr.Port))
		if err != nil {
			return nil, errors.New("Failed to dial server: " + svr.Addr + "\n\n" + err.Error())
		}
		c, chans, reqs, err := ssh.NewClientConn(tunnelConnection, svr.Addr, config)
		if err != nil {
			return nil, errors.New("Failed to establish SSH connection with server: " + svr.Addr + "\n\n" + err.Error())
		}

		client = ssh.NewClient(c, chans, reqs)
	} else {
		client2, err := ssh.Dial("tcp", svr.Addr+":"+strconv.Itoa(svr.Port), config)
		if err != nil {
			return nil, errors.New("Failed to establish SSH connection with server: " + svr.Addr + "\n\n" + err.Error())
		}
		client = client2
	}

	return client, nil
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
