package execution

import (
	"code.google.com/p/go.crypto/ssh"
	"io"
	"mussh/resources/command"
	"mussh/resources/server"
	"strconv"
	"sync"
)

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
			sshErr := sshError{"Failed to create SSH session for server: " + svr.Addr, err}
			out <- SshResult{svr, sshErr.Error(), false}
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

func createClient(config *ssh.ClientConfig, svr server.Server) (client *ssh.Client, err error) {
	if svr.Tunnel != "" {
		tunnelClient, err := ssh.Dial("tcp", svr.Tunnel+":"+strconv.Itoa(svr.Port), config)
		if err != nil {
			return nil, sshError{"Failed to dial tunnel address: " + svr.Tunnel, err}
		}
		tunnelConnection, err := tunnelClient.Dial("tcp", svr.Addr+":"+strconv.Itoa(svr.Port))
		if err != nil {
			return nil, sshError{"Failed to dial server: " + svr.Addr, err}
		}
		c, chans, reqs, err := ssh.NewClientConn(tunnelConnection, svr.Addr, config)
		if err != nil {
			return nil, sshError{"Failed to establish SSH connection with server: " + svr.Addr, err}
		}
		client = ssh.NewClient(c, chans, reqs)
	} else {
		client, err = ssh.Dial("tcp", svr.Addr+":"+strconv.Itoa(svr.Port), config)
		if err != nil {
			return nil, sshError{"Failed to establish SSH connection with server: " + svr.Addr, err}
		}
	}
	return
}
