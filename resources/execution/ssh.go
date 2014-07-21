package execution

import (
	"code.google.com/p/go.crypto/ssh"
	"mussh/resources/command"
	"mussh/resources/server"
	"strconv"
)

type pipeWriter struct {
	isErr bool
	svr   server.Server
	out   chan<- SshResult
}

func runCommand(svr server.Server, cmd command.Command, config *ssh.ClientConfig, done <-chan struct{}) <-chan SshResult {
	out := make(chan SshResult)

	go func() {
		defer close(out)

		client, err := createClient(config, svr)
		if err != nil {
			out <- SshResult{svr, err.Error(), false}
			return
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			sshErr := sshError{"Failed to create SSH session for server: " + svr.Addr, err}
			out <- SshResult{svr, sshErr.Error(), false}
			return
		}
		defer session.Close()

		session.Stdout = &pipeWriter{false, svr, out}
		session.Stderr = &pipeWriter{true, svr, out}

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

func (pipe *pipeWriter) Write(p []byte) (int, error) {
	pipe.out <- SshResult{pipe.svr, string(p[:]), !pipe.isErr}
	return len(p), nil
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
