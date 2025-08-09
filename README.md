mytcpchat - TCP chat server + client with SQLite (GORM)

Structure:
  cmd/server/main.go  - server
  cmd/client/main.go  - client

How to run:
  go run ./cmd/server
  go run ./cmd/client

Features:
 - TCP server on :3000
 - Client can send many messages, exit with "exit"
 - Server stores users and messages in SQLite via GORM
 - Commands:
    /setname <name> <password>   - register name
    /connect <name> <password>   - login
    /list                         - list other users
    /echo <...>                   - echo back text
    /add a b                      - add two integers
    /mul a b                      - multiply two integers
    /bytes <text>                 - server replies with byte length
    /words <text>                 - server replies with word count

Server sends message history to new clients.

Archive includes everything, ready to run.
# TCPchat
# TCPchat
