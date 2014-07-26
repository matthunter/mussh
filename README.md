Multi-SSH
=====

Multi-SSH is the open source server capable of executing commands via ssh on a group of servers.

## Features
* Written in Go
* Rest API to get/post/delete servers, groups, and commands
* Websocket to execute commands.  Outputs are sent via the socket as they are available (tail -f works)
* Can specify a tunnel address and base directory for each server
* RethinkDB database 


## Installation
* Download and run RethinkDB
* Setup GOPATH:  https://code.google.com/p/go-wiki/wiki/GOPATH
* Run go get
* Run go install

Client: https://github.com/matthunter/mussh-client
