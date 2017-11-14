package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

// Handshake related constants
const versionNumber = "0.0.0.1"
const versionTag = "NodeWars:" + versionNumber

var players = make(map[*websocket.Conn]*Player) // connected players
var broadcast = make(chan Message)
var teams = makeDummyTeams()

var gameMap = NewDefaultMap()

var upgrader = websocket.Upgrader{}

// Message is our basic message struct
type Message struct {
	Type   string `json:"type"`
	Sender string `json:"sender"`
	Data   string `json:"data"`
}

// Ask about reduntant error messaging...
func doHandshake(ws *websocket.Conn) error {

	_, p, err := ws.ReadMessage()
	if err != nil {
		log.Printf("Could not read from socket: %v", err)
		return err
	}

	if string(p) == versionTag {
		message := []byte("Welcome to NodeWars")
		if err := ws.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Println("Error while affirming handshake")
			return err
		}
	} else {
		errorMessage := []byte("Version mismation, Server is running " + versionTag + ", closing connection.")
		if err := ws.WriteMessage(websocket.TextMessage, errorMessage); err != nil {
			log.Println("Error while aborting handshake")
			return err
		}
		return errors.New(string(errorMessage))
	}
	return nil
}

var tInc int

func handleConnections(w http.ResponseWriter, r *http.Request) {

	// Upgrade GET to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Close this socket when we're done
	defer ws.Close()

	// Attempt handshake
	err = doHandshake(ws)
	if err != nil {
		log.Printf("Handshake error: %v", err)
		return
	}

	// Assuming we're all good, register client
	thisPlayer := registerPlayer(ws)
	assignToTeam(thisPlayer, &teams[tInc%2])
	// sendWorldState(thisPlayer)
	tInc++

	// Spin up gorouting to monitor outgoing and send those messages to player.Socket
	go messageDispatcher(thisPlayer)

	// Handle socket stream
	for {
		var msg Message

		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)

			// Remove player from team on socket close
			scrubPlayerSocket(ws)
			break
		}
		messageHandler(&msg, thisPlayer)
		fmt.Printf("message from %v\n", thisPlayer.Name)
	}
}

// Should this do socket scrubbing on error? Is that redundant? TODO
func messageDispatcher(p *Player) {
	for {
		msg := <-p.Outgoing
		if err := p.Socket.WriteJSON(msg); err != nil {
			log.Printf("error dispatching message to %v", p.Name)
			scrubPlayerSocket(p.Socket)
			return
		}
	}
}

func messageHandler(msg *Message, sender *Player) {
	switch msg.Type {
	case "chat":
		// Attach sendersocket's name since its relevant for chats
		msg.Sender = sender.Name
		go sender.Team.broadcast(*msg)
	default:
	}
}

// func sendWorldState(p *Player)

func assignToTeam(p *Player, t *Team) {
	t.addPlayer(p)
	p.Socket.WriteJSON(Message{
		Type:   "teamAssign",
		Sender: "",
		Data:   t.Name,
	})
}

func scrubPlayerSocket(ws *websocket.Conn) {
	ws.Close()
	players[ws].Team.removePlayer(players[ws])
	delete(players, ws)
}

func teamChatHandler() {
	for _, team := range teams {
		go func(t Team) {
			for {
				msg := <-t.Channel
				t.broadcast(msg)
			}
		}(team)
	}
}

func main() {
	// So it doesn't complain about fmt
	fmt.Println("Starting " + versionTag + " server...")

	// Set up log file
	f, err := os.OpenFile("log.txt", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}

	//Close log file when we're done
	defer f.Close()
	//set output of logs to f
	log.SetOutput(f)

	// Start Webserver
	fs := http.FileServer(http.Dir("../public"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", handleConnections)

	// Goroutine for dispatching chat messages
	teamChatHandler()

	log.Println("Starting server on port 8080...")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}

}
