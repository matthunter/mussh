package execution

import (
	"code.google.com/p/go.crypto/ssh"
	"errors"
	"github.com/dancannon/gorethink"
	"github.com/gorilla/websocket"
	"github.com/martini-contrib/render"
	"io"
	"log"
	"mussh/resources/command"
	"mussh/resources/server"
	"net/http"
	"strconv"
	"sync"
)

type Execution struct {
	Id        string      `gorethink:"id,omitempty" json:"id"`
	Username  string      `json:"username"`
	Password  string      `json:"password"`
	GroupId   string      `json:"groupId"`
	CommandId string      `json:"commandId"`
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

	resultChan := make(chan SshResult)
	doneChan := make(chan struct{})

	var wg sync.WaitGroup
	for _, svr := range servers {
		wg.Add(1)
		go func(svr server.Server) {
			defer wg.Done()

			sshSession, err := createSession(config, svr)
			if err != nil {
				resultChan <- SshResult{svr, err.Error(), false}
				return
			}
			defer sshSession.Close()

			err = runCommand(svr, cmd, sshSession, resultChan, doneChan)
			if err != nil {
				resultChan <- SshResult{svr, err.Error(), false}
			}
		}(svr)
	}

	go func() {
		defer close(doneChan)
		ws.ReadMessage()
		doneChan <- struct{}{}
	}()

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		err := ws.WriteJSON(result)
		if err != nil {
			return
		}
	}
}

func runCommand(svr server.Server, cmd command.Command, sshSession *ssh.Session,
	resultChan chan<- SshResult, doneChan <-chan struct{}) error {

	script := cmd.Script
	if svr.BaseDir != "" {
		script = "cd " + svr.BaseDir + " && " + script
	}

	stdoutPipe, _ := sshSession.StdoutPipe()
	stderrPipe, _ := sshSession.StderrPipe()
	errorChan := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		readFromPipe(io.MultiReader(stdoutPipe, stderrPipe), resultChan, doneChan, errorChan, svr)
	}()

	err := sshSession.Run(script)
	if err != nil {
		errorChan <- struct{}{}
		wg.Wait()
	}
	return err
}

func readFromPipe(pipeReader io.Reader, resultChan chan<- SshResult, doneChan <-chan struct{},
	errorChan <-chan struct{}, svr server.Server) {

	readChan := make(chan SshResult)
	go func() {
		defer close(readChan)
		buffer := make([]byte, 4096)
		for {
			n, err := pipeReader.Read(buffer)
			if err != nil {
				return
			}
			select {
			case <-doneChan:
				return
			case <-errorChan:
				return
			case readChan <- SshResult{svr, string(buffer[:n]), true}:
				log.Println("SENT DATA : ", SshResult{svr, string(buffer[:n]), true})
			}
		}
	}()

	for {
		select {
		case result, ok := (<-readChan):
			if ok {
				log.Println("Result read from pipe: ", result)
				resultChan <- result
			} else {
				return
			}
		case <-doneChan:
			return
		case <-errorChan:
			return
		}
	}
}

func createSession(config *ssh.ClientConfig, svr server.Server) (*ssh.Session, error) {
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

	session, err := client.NewSession()
	if err != nil {
		return nil, errors.New("Failed to create SSH session for server: " + svr.Addr + "\n\n" + err.Error())
	}

	return session, nil
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
