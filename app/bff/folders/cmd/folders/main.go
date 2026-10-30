package main

import (
	"github.com/teamgram/marmota/pkg/commands"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/server"
)

func main() {
	commands.Run(server.New())
}
