Multi-SSH
=====

Multi-SSH is the open source server capable of executing commands via ssh on a group of servers.

## Features
* Written in Go
* Rest API to get/post/delete servers, groups, and commands
* Websocket to execute commands.  Outputs are sent via the socket as they are available (tail -f works)
* RethinkDB database 


## Installation
Download and run RethinkDB
Setup $GoPath
Run go get
Run go install

Client: https://github.com/matthunter/mussh-client
